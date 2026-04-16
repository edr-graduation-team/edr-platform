// Package collectors — File I/O event handler for the ETW kernel tracer.
//
// Handles FileIo events delivered real-time from the Windows Kernel
// ETW session (EVENT_TRACE_FLAG_FILE_IO_INIT). Every event contains the
// exact PID that initiated the I/O, enabling process-to-file attribution.
//
// MITRE coverage: T1005 (Data from Local System), T1565 (Data Manipulation),
// T1486 (Data Encrypted for Impact / Ransomware).
//
//go:build windows
// +build windows

package collectors

import (
	"path/filepath"
	"strings"

	"github.com/edr-platform/win-agent/internal/event"
)

// FileIo opcode constants from the Windows Kernel Trace.
const (
	fileIoCreate = 64
	fileIoWrite  = 68
	fileIoDelete = 70
	fileIoRename = 71
)

// handleFileIo processes a single file I/O event from the kernel ETW callback.
// This method runs in its own goroutine (spawned by goFileIoEvent) so it is
// safe to call enrichment APIs that may block briefly.
func (c *ETWCollector) handleFileIo(pid uint32, opcode uint8, filePath string) {
	// --- Noise filter: skip high-volume, low-value file paths ---
	lower := strings.ToLower(filePath)
	if isNoisyFilePath(lower) {
		return
	}

	// Map kernel opcode → human-readable action + severity.
	var action string
	var severity event.Severity

	switch opcode {
	case fileIoCreate:
		action = "created"
		severity = event.SeverityLow
	case fileIoWrite:
		action = "modified"
		severity = event.SeverityLow
	case fileIoDelete:
		action = "deleted"
		severity = event.SeverityMedium
	case fileIoRename:
		action = "renamed"
		severity = event.SeverityLow
	default:
		return
	}

	// Enrich with the process that performed the I/O.
	procPath := getImagePath(pid)
	procName := baseName(procPath)
	if procName == "" {
		procName = "unknown"
	}
	// Drop the agent's own file I/O (queue/log writes) to avoid self-generated
	// noise and false-positive drift in downstream Sigma matching.
	if isSelfOrChildProcess(strings.ToLower(procName), "") {
		return
	}

	name := filepath.Base(filePath)
	dir := filepath.Dir(filePath)
	ext := filepath.Ext(name)

	// For production context-aware scoring, file events must carry enough
	// endpoint/process context (user + command-line) to satisfy the required
	// `context_quality_score` inputs.
	sid, user, elevated, integrity := getPrivileges(pid)
	cmdLine := getCmdLine(pid)

	evt := event.NewEvent(event.EventTypeFile, severity, map[string]interface{}{
		"action":          action,
		"path":            filePath,
		"name":            name,
		"directory":       dir,
		"extension":       ext,
		"pid":             pid,
		"process_name":    procName,
		"process_path":    procPath,
		"user_name":       user,
		"user_sid":        sid,
		"command_line":    cmdLine,
		"is_elevated":     elevated,
		"integrity_level": integrity,
	})

	// Apply configurable filter.
	if c.filter != nil && c.filter.ShouldFilter(evt) {
		return
	}

	c.send(evt)
	c.fileEvents.Add(1)
}

// isNoisyFilePath filters out high-volume, low-value file I/O noise.
// This is critical for performance — kernel file I/O generates thousands
// of events per second from OS services, antivirus, indexing, etc.
//
// These are hard-coded because they represent immutable OS behavior —
// no real-world attack depends on writing to these paths/extensions.
// The configurable ExcludePaths list in FilterConfig handles user-defined
// exclusions and is checked after event creation in the filter pipeline.
func isNoisyFilePath(lower string) bool {
	// Skip common temp/cache/OS-internal file extensions.
	noisySuffixes := []string{
		".tmp", ".log", ".etl", ".blf", ".regtrans-ms",
		"~rf", ".pf", "thumbs.db", "desktop.ini",
		// Windows Event Log / diagnostics
		".evtx", ".pma", ".sdi",
		// ESE / transaction journaling (used by Search, BITS, etc.)
		".jrs", ".chk",
		// WMI / COM metadata
		".mof",
		// Catalog files (driver signing verification)
		".cat",
		// Side-by-side assembly manifests
		".manifest",
	}
	for _, s := range noisySuffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}

	// Skip high-noise OS directories.
	noisyDirs := []string{
		`\windows\softwaredistribution`,
		`\windows\temp`,
		`\windows\prefetch`,
		`\windows\servicing`,
		`\appdata\local\temp`,
		`\appdata\local\microsoft\windows\inetcache`,
		`\windows\logs\cbs`,
		`\programdata\microsoft\windows\wer`,
		`\$extend`,
		`\system volume information`,
		// .NET / Assembly (Global Assembly Cache)
		`\windows\assembly`,
		`\windows\winsxs`,
		`\windows\microsoft.net`,
		// Installer cache
		`\windows\installer`,
		// Application Compatibility (shim database)
		`\windows\appcompat`,
		// Windows Defender real-time scan artifacts
		`\programdata\microsoft\windows defender`,
		// Office telemetry
		`\appdata\local\microsoft\office`,
		// UWP app containers (extremely noisy on Win10/11)
		`\appdata\local\packages`,
		// Font cache
		`\windows\fonts`,
		// Windows Search index
		`\programdata\microsoft\search`,
		// Agent internals (self-generated I/O, no attacker signal)
		`\programdata\edr\queue`,
		`\programdata\edr\logs`,
		`\programdata\edr\quarantine`,
	}
	for _, d := range noisyDirs {
		if strings.Contains(lower, d) {
			return true
		}
	}

	// Skip kernel/driver device paths (not real filesystem paths).
	if strings.HasPrefix(lower, `\device\`) && !strings.Contains(lower, `\harddiskvolume`) {
		return true
	}

	return false
}
