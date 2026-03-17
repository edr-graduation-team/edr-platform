package cache

import "time"

// ProcessLineageEntry holds the contextual snapshot of a single process
// at the moment it was observed. It is stored in Redis (as a Hash) and
// retrieved during risk scoring to reconstruct the process ancestry chain.
//
// Field names use compact snake_case identifiers to minimise Redis memory
// usage per key (Redis stores each Hash field name verbatim).
type ProcessLineageEntry struct {
	// Identity
	AgentID string `redis:"agent_id"` // UUID of the reporting agent
	PID     int64  `redis:"pid"`      // Process ID
	PPID    int64  `redis:"ppid"`     // Parent process ID (0 if unavailable)

	// Process image
	Name        string `redis:"name"`       // e.g. "powershell.exe"
	Executable  string `redis:"executable"` // Full path, e.g. "C:\Windows\System32\..."
	CommandLine string `redis:"cmd_line"`   // Full command-line string (may be truncated to 512 chars)

	// Parent context — populated when the agent sends parent fields directly.
	// These are the quick-path fields; for deeper ancestry use GetLineageChain.
	ParentName       string `redis:"parent_name"`       // e.g. "winword.exe"
	ParentExecutable string `redis:"parent_executable"` // Full path of parent

	// User security context
	UserName       string `redis:"user_name"`   // e.g. "CORP\jsmith"
	UserSID        string `redis:"user_sid"`    // e.g. "S-1-5-18"
	IntegrityLevel string `redis:"integrity"`   // "Low", "Medium", "High", "System"
	IsElevated     bool   `redis:"is_elevated"` // true if process token is elevated

	// Binary trust
	SignatureStatus string `redis:"sig_status"` // "microsoft", "trusted", "unsigned", ""
	HashSHA256      string `redis:"sha256"`     // SHA-256 of the executable (if available)

	// Observation timestamp (Unix seconds — avoids time.Time JSON overhead)
	SeenAt int64 `redis:"seen_at"`
}

// NewProcessLineageEntry constructs an entry from the flat map[string]interface{}
// payload carried inside a domain.LogEvent's RawData.
//
// Field resolution strategy (mirrors FieldMapper.sigmaToAgentData):
//  1. Check the top-level RawData key directly (e.g. RawData["pid"]).
//     This handles events that have already been flattened.
//  2. Fall back to the nested "data" sub-map (e.g. RawData["data"]["pid"]).
//     This handles the Windows Agent's native event format, which wraps all
//     process-specific fields inside a "data": {...} JSON object:
//     { "event_type": "process", "data": { "pid": 1234, "ppid": 5678, ... } }
//
// Without the fallback, every pid/ppid read returns 0, the Redis lineage entry
// is rejected by WriteEntry (pid==0), and GetLineageChain always returns an
// empty chain — producing "No lineage data" in the Context tab.
func NewProcessLineageEntry(agentID string, data map[string]interface{}) *ProcessLineageEntry {
	e := &ProcessLineageEntry{
		AgentID: agentID,
		SeenAt:  time.Now().Unix(),
	}

	// dataMap extracts the nested "data" sub-object once, if present.
	// Returns nil when the event is already flat (no sub-map).
	var dataSub map[string]interface{}
	if sub, ok := data["data"]; ok && sub != nil {
		if m, ok := sub.(map[string]interface{}); ok {
			dataSub = m
		}
	}

	// resolve looks up a key by checking the top-level map first, then the
	// nested data sub-map. This makes the function work for both flat events
	// (unit tests, Sigma test fixtures) and real agent events.
	resolve := func(key string) interface{} {
		if v, ok := data[key]; ok && v != nil {
			return v
		}
		if dataSub != nil {
			if v, ok := dataSub[key]; ok && v != nil {
				return v
			}
		}
		return nil
	}

	// Helper: extract a string from resolved value safely.
	str := func(key string) string {
		v := resolve(key)
		if v == nil {
			return ""
		}
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}

	// Helper: extract an int64 from resolved value safely.
	// Handles uint32 explicitly because Windows API (Toolhelp32/ETW) surfaces
	// ProcessID and ParentProcessID as uint32. Without this case every PPID
	// falls through to 0, silently breaking the lineage chain walk.
	i64 := func(key string) int64 {
		v := resolve(key)
		if v == nil {
			return 0
		}
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case float64:
			return int64(n)
		case uint32: // Windows API: ProcessEntry32.ProcessID / ParentProcessID
			return int64(n)
		case uint64:
			return int64(n)
		case uint:
			return int64(n)
		}
		return 0
	}

	// Helper: extract a bool from resolved value safely.
	boolean := func(key string) bool {
		v := resolve(key)
		if v == nil {
			return false
		}
		if b, ok := v.(bool); ok {
			return b
		}
		return false
	}

	e.PID = i64("pid")
	e.PPID = i64("ppid")
	e.Name = str("name")
	e.Executable = str("executable")

	// Truncate command-line to 512 chars to cap Redis field size
	// while preserving enough context for LOLBin detection.
	cmdLine := str("command_line")
	if len(cmdLine) > 512 {
		cmdLine = cmdLine[:512]
	}
	e.CommandLine = cmdLine

	e.ParentName = str("parent_name")
	e.ParentExecutable = str("parent_executable")
	e.UserName = str("user_name")
	e.UserSID = str("user_sid")
	e.IntegrityLevel = str("integrity_level")
	e.IsElevated = boolean("is_elevated")
	e.SignatureStatus = str("signature_status")
	e.HashSHA256 = str("hash_sha256")

	return e
}

