// Package event provides event batching and compression.
package event

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/golang/snappy"
	"github.com/google/uuid"

	"github.com/edr-platform/win-agent/internal/logging"
)

// Batch represents a batch of events ready for transmission.
type Batch struct {
	ID          string    `json:"batch_id"`
	AgentID     string    `json:"agent_id"`
	Timestamp   time.Time `json:"timestamp"`
	EventCount  int       `json:"event_count"`
	Compression string    `json:"compression"`
	Payload     []byte    `json:"payload"`
	Checksum    string    `json:"checksum"`
	Events      []*Event  `json:"-"` // Not serialized, kept for reference
}

// Batcher collects events and creates compressed batches.
type Batcher struct {
	mu          sync.Mutex
	events      []*Event
	batchSize   int
	interval    time.Duration
	compression string
	lastFlush   time.Time
	logger      *logging.Logger
}

// NewBatcher creates a new event batcher.
func NewBatcher(batchSize int, interval time.Duration, compression string, logger *logging.Logger) *Batcher {
	if batchSize <= 0 {
		batchSize = 50
	}
	if interval <= 0 {
		interval = time.Second
	}
	if compression == "" {
		compression = "snappy"
	}

	return &Batcher{
		events:      make([]*Event, 0, batchSize),
		batchSize:   batchSize,
		interval:    interval,
		compression: compression,
		lastFlush:   time.Now(),
		logger:      logger,
	}
}

// Add adds an event to the batch. Returns a batch if threshold is reached.
func (b *Batcher) Add(evt *Event) *Batch {
	if evt == nil {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = append(b.events, evt)

	// Check if batch size threshold reached
	if len(b.events) >= b.batchSize {
		return b.createBatch()
	}

	return nil
}

// FlushIfReady returns a batch if interval has elapsed, otherwise nil.
func (b *Batcher) FlushIfReady() *Batch {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.events) == 0 {
		return nil
	}

	if time.Since(b.lastFlush) >= b.interval {
		return b.createBatch()
	}

	return nil
}

// Flush creates a batch from all pending events.
func (b *Batcher) Flush() *Batch {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.events) == 0 {
		return nil
	}

	return b.createBatch()
}

// createBatch creates a compressed batch from current events.
// Must be called with lock held.
func (b *Batcher) createBatch() *Batch {
	if len(b.events) == 0 {
		return nil
	}

	// Copy events
	events := make([]*Event, len(b.events))
	copy(events, b.events)

	// Fill per-event source metadata so downstream (alerts/context) can be
	// attributed to a specific endpoint even when the collector payload doesn't
	// embed host identity.
	fillEventSource(events)

	// Clear buffer
	b.events = b.events[:0]
	b.lastFlush = time.Now()

	// Serialize events to JSON
	jsonData, err := json.Marshal(events)
	if err != nil {
		b.logger.Errorf("Failed to serialize events: %v", err)
		return nil
	}

	// Compress payload
	var payload []byte
	var compressionType string

	switch b.compression {
	case "snappy":
		payload = snappy.Encode(nil, jsonData)
		compressionType = "snappy"
	case "none":
		payload = jsonData
		compressionType = "none"
	default:
		payload = snappy.Encode(nil, jsonData)
		compressionType = "snappy"
	}

	// Calculate checksum
	hash := sha256.Sum256(payload)
	checksum := hex.EncodeToString(hash[:])

	batch := &Batch{
		ID:          uuid.New().String(),
		Timestamp:   time.Now().UTC(),
		EventCount:  len(events),
		Compression: compressionType,
		Payload:     payload,
		Checksum:    checksum,
		Events:      events,
	}

	// Log compression ratio
	if b.logger != nil {
		ratio := float64(len(payload)) / float64(len(jsonData)) * 100
		b.logger.Debugf("Batch created: %d events, %d→%d bytes (%.1f%%)",
			len(events), len(jsonData), len(payload), ratio)
	}

	return batch
}

// fillEventSource populates Event.Source for every event in the batch.
// This prevents downstream gaps where alerts show an empty `source{}` block.
func fillEventSource(events []*Event) {
	hostname := "unknown"
	if h, err := os.Hostname(); err == nil && h != "" {
		hostname = h
	}

	ip := getFirstNonLoopbackIP()
	if ip == "" {
		ip = "unknown"
	}

	// NOTE: os_version in event source is set to "unknown" here since getOSVersion()
	// lives in the grpcclient package. The real os_version is sent via heartbeat
	// and stored persistently in the DB — no need to duplicate the WMI call per-batch.
	osVersion := "unknown"

	agentVersion := "1.0.0"
	osType := runtime.GOOS
	if osType == "" {
		osType = "unknown"
	}

	for _, evt := range events {
		if evt == nil {
			continue
		}
		evt.Source = EventSource{
			Hostname:     hostname,
			IPAddress:    ip,
			OSType:       osType,
			OSVersion:    osVersion,
			AgentVersion: agentVersion,
		}
	}
}

// getFirstNonLoopbackIP returns the first non-loopback IP string found on
// active network interfaces.
func getFirstNonLoopbackIP() string {
	defer func() {
		_ = recover()
	}()

	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range ifaces {
		// Skip down or loopback interfaces.
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}

			if ip == nil {
				continue
			}
			// Skip loopback/link-local.
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			if ip.String() == "" {
				continue
			}
			return ip.String()
		}
	}

	return ""
}

// Count returns the number of pending events.
func (b *Batcher) Count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

// SetBatchSize changes the batch size threshold.
func (b *Batcher) SetBatchSize(size int) {
	if size < 1 {
		size = 1
	}
	if size > 10000 {
		size = 10000
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.batchSize = size
}

// SetInterval changes the flush interval.
func (b *Batcher) SetInterval(interval time.Duration) {
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	if interval > 60*time.Second {
		interval = 60 * time.Second
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.interval = interval
}

// Reconfigure atomically updates batchSize, interval, and compression in a
// single lock acquisition. This is used by agent.UpdateConfig() during a
// hot-reload so all three parameters change simultaneously without a window
// where they are inconsistent.
func (b *Batcher) Reconfigure(batchSize int, interval time.Duration, compression string) {
	if batchSize < 1 {
		batchSize = 1
	}
	if batchSize > 10000 {
		batchSize = 10000
	}
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	if interval > 60*time.Second {
		interval = 60 * time.Second
	}
	if compression == "" {
		compression = "snappy"
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.batchSize = batchSize
	b.interval = interval
	b.compression = compression
}
