package domain

import (
	"crypto/md5"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LogEvent represents a normalized security event in ECS (Elastic Common Schema) format.
// It provides efficient field access with caching and automatic category inference.
// Thread-safe for concurrent field access.
type LogEvent struct {
	RawData   map[string]interface{} `json:"raw_data"`
	EventID   *string                `json:"event_id,omitempty"`
	Category  EventCategory          `json:"category"`
	Product   string                 `json:"product"`
	Service   string                 `json:"service,omitempty"`
	Timestamp time.Time              `json:"timestamp"`

	fieldCache map[string]interface{}
	cacheMu    sync.RWMutex
	hash       *string
	hashMu     sync.Mutex
}

// NewLogEvent creates a new LogEvent from raw event data.
// It automatically extracts event_id, infers category, and extracts product/timestamp.
// Returns an error if rawData is nil.
func NewLogEvent(rawData map[string]interface{}) (*LogEvent, error) {
	if rawData == nil {
		return nil, fmt.Errorf("rawData cannot be nil")
	}

	event := &LogEvent{
		RawData:    rawData,
		Category:   EventCategoryUnknown,
		Product:    "windows",
		Timestamp:  time.Now(),
		fieldCache: make(map[string]interface{}),
	}

	event.EventID = event.extractEventID()
	event.Category = event.inferCategory()
	event.Product = event.extractProduct()
	event.Timestamp = event.extractTimestamp()

	return event, nil
}

// GetField retrieves a field value by path with caching.
// Supports both flat ("EventID") and nested ("process.command_line") field paths.
// Thread-safe for concurrent access.
func (e *LogEvent) GetField(fieldPath string) (interface{}, bool) {
	e.cacheMu.RLock()
	if cached, ok := e.fieldCache[fieldPath]; ok {
		e.cacheMu.RUnlock()
		return cached, true
	}
	e.cacheMu.RUnlock()

	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()

	// Double-check after acquiring write lock
	if cached, ok := e.fieldCache[fieldPath]; ok {
		return cached, true
	}

	if val, ok := e.RawData[fieldPath]; ok {
		e.fieldCache[fieldPath] = val
		return val, true
	}

	if val := e.getNested(fieldPath); val != nil {
		e.fieldCache[fieldPath] = val
		return val, true
	}

	e.fieldCache[fieldPath] = nil
	return nil, false
}

// GetStringField retrieves a field value as a string.
// Returns empty string if field is not found or cannot be converted.
func (e *LogEvent) GetStringField(fieldPath string) string {
	val, ok := e.GetField(fieldPath)
	if !ok || val == nil {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

// GetFloat64Field retrieves a field value as float64.
// Returns 0 and false if field is not found or cannot be converted.
func (e *LogEvent) GetFloat64Field(fieldPath string) (float64, bool) {
	val, ok := e.GetField(fieldPath)
	if !ok || val == nil {
		return 0, false
	}

	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// GetInt64Field retrieves a field value as int64.
// Returns 0 and false if field is not found or cannot be converted.
func (e *LogEvent) GetInt64Field(fieldPath string) (int64, bool) {
	val, ok := e.GetField(fieldPath)
	if !ok || val == nil {
		return 0, false
	}

	switch v := val.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	case string:
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i, true
		}
	}
	return 0, false
}

// GetBoolField retrieves a field value as bool.
// Returns false and false if field is not found or cannot be converted.
func (e *LogEvent) GetBoolField(fieldPath string) (bool, bool) {
	val, ok := e.GetField(fieldPath)
	if !ok || val == nil {
		return false, false
	}

	switch v := val.(type) {
	case bool:
		return v, true
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			return b, true
		}
	}
	return false, false
}

// GetFieldWithDefault retrieves a field value or returns the default if not found.
func (e *LogEvent) GetFieldWithDefault(fieldPath string, defaultValue interface{}) interface{} {
	if val, ok := e.GetField(fieldPath); ok && val != nil {
		return val
	}
	return defaultValue
}

// HasField checks if a field exists and has a non-nil value.
func (e *LogEvent) HasField(fieldPath string) bool {
	val, ok := e.GetField(fieldPath)
	return ok && val != nil
}

// ComputeHash generates an MD5 hash for deduplication based on key fields.
// Thread-safe and cached after first computation.
func (e *LogEvent) ComputeHash() string {
	e.hashMu.Lock()
	defer e.hashMu.Unlock()

	if e.hash != nil {
		return *e.hash
	}

	keyFields := []string{
		e.getEventIDString(),
		e.GetStringField("process.name"),
		e.GetStringField("process.command_line"),
		e.GetStringField("CommandLine"),
		e.GetStringField("Image"),
		e.Timestamp.Format("2006-01-02 15:04"),
	}

	hashInput := strings.Join(keyFields, "|")
	hash := fmt.Sprintf("%x", md5.Sum([]byte(hashInput)))
	e.hash = &hash
	return hash
}

// String returns a human-readable string representation of the event.
func (e *LogEvent) String() string {
	eventIDStr := "N/A"
	if e.EventID != nil {
		eventIDStr = *e.EventID
	}
	return fmt.Sprintf("LogEvent{id=%s, category=%s, product=%s, timestamp=%s}",
		eventIDStr, e.Category, e.Product, e.Timestamp.Format(time.RFC3339))
}

// GetCategory returns the event category as a string.
// Implements ports.Event interface.
func (e *LogEvent) GetCategory() string {
	return string(e.Category)
}

// GetProduct returns the event product.
// Implements ports.Event interface.
func (e *LogEvent) GetProduct() string {
	return e.Product
}

func (e *LogEvent) extractEventID() *string {
	paths := []string{
		"event.code",
		"EventID",
		"event_id",
		"winlog.event_id",
		"System.EventID",
		"Event.System.EventID",
	}

	for _, path := range paths {
		if val, ok := e.GetField(path); ok && val != nil {
			str := fmt.Sprintf("%v", val)
			return &str
		}
	}
	return nil
}

func (e *LogEvent) inferCategory() EventCategory {
	// Check agent's event_type field first (our EDR agent sends this on every event)
	if et, ok := e.GetField("event_type"); ok && et != nil {
		switch strings.ToLower(fmt.Sprintf("%v", et)) {
		case "process":
			return EventCategoryProcessCreation
		case "network":
			return EventCategoryNetworkConnection
		case "file":
			return EventCategoryFileEvent
		case "registry":
			return EventCategoryRegistryEvent
		case "dns":
			return EventCategoryDNSQuery
		case "auth":
			return EventCategoryAuthentication
		case "driver":
			return EventCategoryDriverLoad
		case "image_load":
			return EventCategoryImageLoad
		case "pipe":
			return EventCategoryPipeCreated
		case "wmi":
			return EventCategoryWMIEvent
		case "clipboard":
			return EventCategoryFileEvent
		}
	}

	if e.EventID != nil {
		if eventID, err := strconv.Atoi(*e.EventID); err == nil {
			if cat := InferCategoryFromEventID(eventID); cat != EventCategoryUnknown {
				return cat
			}
		}
	}

	if action, ok := e.GetField("event.action"); ok {
		actionStr := strings.ToLower(fmt.Sprintf("%v", action))
		if strings.Contains(actionStr, "start") || strings.Contains(actionStr, "create") || strings.Contains(actionStr, "exec") {
			return EventCategoryProcessCreation
		}
		if strings.Contains(actionStr, "connect") || strings.Contains(actionStr, "network") {
			return EventCategoryNetworkConnection
		}
		if strings.Contains(actionStr, "file") || strings.Contains(actionStr, "write") || strings.Contains(actionStr, "read") {
			return EventCategoryFileEvent
		}
		if strings.Contains(actionStr, "dns") || strings.Contains(actionStr, "query") {
			return EventCategoryDNSQuery
		}
		if strings.Contains(actionStr, "registry") {
			return EventCategoryRegistryEvent
		}
		if strings.Contains(actionStr, "logon") || strings.Contains(actionStr, "auth") || strings.Contains(actionStr, "login") {
			return EventCategoryAuthentication
		}
	}

	if category, ok := e.GetField("event.category"); ok {
		catStr := strings.ToLower(fmt.Sprintf("%v", category))
		if strings.Contains(catStr, "process") {
			return EventCategoryProcessCreation
		}
		if strings.Contains(catStr, "network") {
			return EventCategoryNetworkConnection
		}
		if strings.Contains(catStr, "file") {
			return EventCategoryFileEvent
		}
		if strings.Contains(catStr, "registry") {
			return EventCategoryRegistryEvent
		}
		if strings.Contains(catStr, "authentication") {
			return EventCategoryAuthentication
		}
	}

	if e.HasField("Image") || e.HasField("process.executable") {
		if e.HasField("CommandLine") || e.HasField("process.command_line") {
			return EventCategoryProcessCreation
		}
	}

	if e.HasField("DestinationIp") || e.HasField("destination.ip") {
		return EventCategoryNetworkConnection
	}

	if e.HasField("TargetFilename") || e.HasField("file.path") {
		return EventCategoryFileEvent
	}

	if e.HasField("TargetObject") || e.HasField("registry.path") {
		return EventCategoryRegistryEvent
	}

	if e.HasField("QueryName") || e.HasField("dns.question.name") {
		return EventCategoryDNSQuery
	}

	return EventCategoryUnknown
}

func (e *LogEvent) extractProduct() string {
	paths := []string{
		"source.os_type",
		"agent.type",
		"log.type",
		"winlog.provider_name",
		"event.module",
	}

	for _, path := range paths {
		if val, ok := e.GetField(path); ok && val != nil {
			valStr := strings.ToLower(fmt.Sprintf("%v", val))
			if strings.Contains(valStr, "windows") || strings.Contains(valStr, "sysmon") {
				return "windows"
			}
			if strings.Contains(valStr, "linux") {
				return "linux"
			}
			if strings.Contains(valStr, "macos") || strings.Contains(valStr, "darwin") {
				return "macos"
			}
		}
	}

	if e.EventID != nil {
		if eventID, err := strconv.Atoi(*e.EventID); err == nil {
			if _, ok := EventIDToCategory[eventID]; ok {
				return "windows"
			}
		}
	}

	return "windows"
}

func (e *LogEvent) extractTimestamp() time.Time {
	paths := []string{
		"@timestamp",
		"timestamp",
		"event.created",
		"event.ingested",
		"EventTime",
		"UtcTime",
	}

	for _, path := range paths {
		if val, ok := e.GetField(path); ok && val != nil {
			switch v := val.(type) {
			case time.Time:
				return v
			case string:
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					return t
				}
				if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
					return t
				}
			}
		}
	}

	return time.Now()
}

func (e *LogEvent) getNested(path string) interface{} {
	if !strings.Contains(path, ".") {
		return nil
	}

	parts := strings.Split(path, ".")
	var current interface{} = e.RawData

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}

		if val, exists := m[part]; exists {
			current = val
			continue
		}

		// Try case-insensitive match
		found := false
		for key, val := range m {
			if strings.EqualFold(key, part) {
				current = val
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	return current
}

func (e *LogEvent) getEventIDString() string {
	if e.EventID != nil {
		return *e.EventID
	}
	return ""
}
