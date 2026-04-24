//go:build windows
// +build windows

package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ─────────────────────────────────────────────────────────────────────────────
// postIsolationTriage: composite command that runs all lightweight snapshots
// ─────────────────────────────────────────────────────────────────────────────

func (h *Handler) postIsolationTriage(ctx context.Context, params map[string]string) (string, error) {
	type triageErr struct {
		Step  string `json:"step"`
		Error string `json:"error"`
	}

	// NOTE: The Dashboard's IncidentTab falls back to using the payload of
	// `post_isolation_triage` as a substitute for individual snapshots.
	// Therefore this bundle must expose the SAME top-level keys as the
	// dedicated commands (processes, persistence_items, tcp_conns, etc.).
	bundle := map[string]any{
		"version":      1,
		"collected_at": time.Now().UTC().Format(time.RFC3339),
		"errors":       []triageErr{},
	}

	type subResult struct {
		step string
		raw  string
		err  error
	}

	runSub := func(step string, timeout time.Duration, fn func(context.Context) (string, error), ch chan<- subResult) {
		subCtx := ctx
		var cancel context.CancelFunc
		if timeout > 0 {
			subCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		if cancel != nil {
			defer cancel()
		}
		out, err := fn(subCtx)
		ch <- subResult{step: step, raw: out, err: err}
	}

	// Run lightweight triage subtasks in parallel. Each has its own timeout so
	// a slow component can't block the entire bundle.
	resCh := make(chan subResult, 6)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		runSub("process_tree_snapshot", 20*time.Second, func(c context.Context) (string, error) {
			return h.processTreeSnapshot(c, params)
		}, resCh)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		runSub("persistence_scan", 25*time.Second, func(c context.Context) (string, error) {
			return h.persistenceScan(c, params)
		}, resCh)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		runSub("lsass_access_audit", 20*time.Second, func(c context.Context) (string, error) {
			return h.lsassAccessAudit(c, params)
		}, resCh)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		runSub("network_last_seen", 15*time.Second, func(c context.Context) (string, error) {
			return h.networkLastSeen(c, params)
		}, resCh)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		runSub("filesystem_timeline", 30*time.Second, func(c context.Context) (string, error) {
			return h.filesystemTimeline(c, params)
		}, resCh)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		runSub("agent_integrity_check", 10*time.Second, func(c context.Context) (string, error) {
			return h.agentIntegrityCheck(c, params)
		}, resCh)
	}()

	wg.Wait()
	close(resCh)

	// Merge results into the bundle.
	appendErr := func(step string, err error) {
		if err == nil {
			return
		}
		ee := bundle["errors"].([]triageErr)
		ee = append(ee, triageErr{Step: step, Error: err.Error()})
		bundle["errors"] = ee
	}

	for r := range resCh {
		if r.err != nil {
			appendErr(r.step, r.err)
			continue
		}
		trimmed := strings.TrimSpace(r.raw)
		if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
			appendErr(r.step, fmt.Errorf("unexpected output (not JSON object)"))
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			appendErr(r.step, fmt.Errorf("json parse failed: %w", err))
			continue
		}

		// Preserve full nested payloads for debugging.
		bundle[r.step] = payload

		// Normalize: copy the keys the dashboard expects to the top level.
		switch r.step {
		case "process_tree_snapshot":
			if v, ok := payload["processes"]; ok {
				bundle["processes"] = v
			}
			if v, ok := payload["count"]; ok {
				bundle["process_count"] = v
			}
		case "persistence_scan":
			if v, ok := payload["persistence_items"]; ok {
				bundle["persistence_items"] = v
			}
			if v, ok := payload["count"]; ok {
				bundle["persistence_count"] = v
			}
		case "lsass_access_audit":
			if v, ok := payload["lsass_accesses"]; ok {
				bundle["lsass_accesses"] = v
			}
			if v, ok := payload["count"]; ok {
				bundle["lsass_count"] = v
			}
		case "network_last_seen":
			if v, ok := payload["tcp_conns"]; ok {
				bundle["tcp_conns"] = v
			}
			if v, ok := payload["dns_cache"]; ok {
				bundle["dns_cache"] = v
			}
		case "filesystem_timeline":
			if v, ok := payload["files"]; ok {
				bundle["files"] = v
			}
			if v, ok := payload["count"]; ok {
				bundle["files_count"] = v
			}
		case "agent_integrity_check":
			for _, k := range []string{"exe_path", "exe_sha256", "signature_valid", "etw_healthy"} {
				if v, ok := payload[k]; ok {
					bundle[k] = v
				}
			}
		}
	}

	out, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("marshal triage bundle: %w", err)
	}
	return string(out), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// processTreeSnapshot: parent-child tree + modules + network per PID
// ─────────────────────────────────────────────────────────────────────────────

type processInfo struct {
	PID        uint32   `json:"pid"`
	PPID       uint32   `json:"ppid"`
	Name       string   `json:"name"`
	Path       string   `json:"path,omitempty"`
	SHA256     string   `json:"sha256,omitempty"`
	Signed     bool     `json:"signed"`
	Modules    []string `json:"modules,omitempty"`
	NetConns   []string `json:"net_conns,omitempty"`
}

func (h *Handler) processTreeSnapshot(ctx context.Context, _ map[string]string) (string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS|windows.TH32CS_SNAPMODULE32, 0)
	if err != nil {
		return "", fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer windows.CloseHandle(snap)

	var processes []processInfo
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := windows.Process32First(snap, &entry); err != nil {
		return "", fmt.Errorf("Process32First: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		name := windows.UTF16ToString(entry.ExeFile[:])
		info := processInfo{
			PID:  entry.ProcessID,
			PPID: entry.ParentProcessID,
			Name: name,
		}

		// Retrieve executable path via OpenProcess
		hProc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if err == nil {
			var pathBuf [windows.MAX_PATH]uint16
			size := uint32(windows.MAX_PATH)
			if windows.QueryFullProcessImageName(hProc, 0, &pathBuf[0], &size) == nil {
				info.Path = windows.UTF16ToString(pathBuf[:size])
				// Hash the binary
				if h2, e2 := hashFile(info.Path); e2 == nil {
					info.SHA256 = h2
				}
				// Check Authenticode signature
				info.Signed = checkAuthenticode(info.Path)
			}
			windows.CloseHandle(hProc)
		}

		processes = append(processes, info)

		if windows.Process32Next(snap, &entry) != nil {
			break
		}
	}

	result := map[string]interface{}{
		"version":     1,
		"captured_at": time.Now().UTC().Format(time.RFC3339),
		"processes":   processes,
		"count":       len(processes),
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// persistenceScan: registry run keys, scheduled tasks, services, startup dirs
// ─────────────────────────────────────────────────────────────────────────────

type persistenceItem struct {
	Type     string `json:"type"`
	Location string `json:"location"`
	Value    string `json:"value"`
	SHA256   string `json:"sha256,omitempty"`
}

func (h *Handler) persistenceScan(_ context.Context, _ map[string]string) (string, error) {
	var items []persistenceItem

	// Run/RunOnce keys (HKLM + HKCU)
	runKeys := []string{
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`,
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`,
		`HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`,
		`HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`,
		`HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`,
	}
	for _, key := range runKeys {
		items = append(items, queryRegRunKey(key)...)
	}

	// Startup folders
	startupDirs := []string{
		filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Startup`),
		`C:\ProgramData\Microsoft\Windows\Start Menu\Programs\Startup`,
	}
	for _, dir := range startupDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			fullPath := filepath.Join(dir, e.Name())
			item := persistenceItem{
				Type:     "startup_folder",
				Location: dir,
				Value:    fullPath,
			}
			if h2, err := hashFile(fullPath); err == nil {
				item.SHA256 = h2
			}
			items = append(items, item)
		}
	}

	// Scheduled tasks (via schtasks /query)
	if out, err := exec.Command("schtasks", "/query", "/fo", "CSV", "/nh").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.SplitN(line, `","`, 3)
			if len(fields) >= 1 {
				taskName := strings.Trim(fields[0], `"`)
				items = append(items, persistenceItem{
					Type:     "scheduled_task",
					Location: "Task Scheduler",
					Value:    taskName,
				})
			}
		}
	}

	// Services (via sc query type= all)
	if out, err := exec.Command("sc", "query", "type=", "all", "state=", "all").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "SERVICE_NAME:") {
				svcName := strings.TrimSpace(strings.TrimPrefix(line, "SERVICE_NAME:"))
				items = append(items, persistenceItem{
					Type:     "service",
					Location: "SCM",
					Value:    svcName,
				})
			}
		}
	}

	result := map[string]interface{}{
		"version":          1,
		"captured_at":      time.Now().UTC().Format(time.RFC3339),
		"persistence_items": items,
		"count":            len(items),
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// queryRegRunKey reads a registry run key and returns persistence items.
func queryRegRunKey(key string) []persistenceItem {
	var items []persistenceItem
	// Use reg query command (simpler than CGO registry calls)
	out, err := exec.Command("reg", "query", key).Output()
	if err != nil {
		return items
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "HKEY") || strings.HasPrefix(line, "!") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			name := parts[0]
			value := strings.Join(parts[2:], " ")
			items = append(items, persistenceItem{
				Type:     "run_key",
				Location: key,
				Value:    name + " = " + value,
			})
		}
	}
	return items
}

// ─────────────────────────────────────────────────────────────────────────────
// lsassAccessAudit: Security event log 4656/4663 filtered to lsass.exe
// ─────────────────────────────────────────────────────────────────────────────

type lsassAccessEvent struct {
	TimeCreated string `json:"time_created"`
	EventID     string `json:"event_id"`
	ActorPID    string `json:"actor_pid"`
	ActorPath   string `json:"actor_path,omitempty"`
	AccessMask  string `json:"access_mask,omitempty"`
	Message     string `json:"message,omitempty"`
}

func (h *Handler) lsassAccessAudit(_ context.Context, params map[string]string) (string, error) {
	hoursBack := 24
	if v, ok := params["hours_back"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 168 {
			hoursBack = n
		}
	}

	// Query Security log for events targeting LSASS within a strict time window.
	// Use timediff(@SystemTime) in milliseconds to avoid locale/date parsing.
	msWindow := int64(hoursBack) * 3600 * 1000
	xpath := fmt.Sprintf(
		`*[System[(EventID=4656 or EventID=4663) and TimeCreated[timediff(@SystemTime) <= %d]] and EventData[Data[@Name='ObjectName'] and (contains(Data[@Name='ObjectName'],'lsass') or contains(Data[@Name='ObjectName'],'LSASS'))]]`,
		msWindow,
	)

	out, err := exec.Command("wevtutil", "qe", "Security", "/q:"+xpath, "/f:text", "/c:200").Output()

	var events []lsassAccessEvent
	if err == nil {
		// Parse plain text output from wevtutil
		lines := strings.Split(string(out), "\n")
		var current lsassAccessEvent
		inEvent := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Event[") {
				if inEvent && current.EventID != "" {
					events = append(events, current)
				}
				current = lsassAccessEvent{}
				inEvent = true
			} else if strings.HasPrefix(line, "Date:") {
				current.TimeCreated = strings.TrimSpace(strings.TrimPrefix(line, "Date:"))
			} else if strings.HasPrefix(line, "Event ID:") {
				current.EventID = strings.TrimSpace(strings.TrimPrefix(line, "Event ID:"))
			} else if strings.Contains(line, "Process ID:") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					current.ActorPID = strings.TrimSpace(parts[1])
				}
			} else if strings.Contains(line, "Access Mask:") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					current.AccessMask = strings.TrimSpace(parts[1])
				}
			}
		}
		if inEvent && current.EventID != "" {
			events = append(events, current)
		}
	}

	result := map[string]interface{}{
		"version":         1,
		"captured_at":     time.Now().UTC().Format(time.RFC3339),
		"hours_back":      hoursBack,
		"lsass_accesses":  events,
		"count":           len(events),
	}
	outJSON, _ := json.Marshal(result)
	return string(outJSON), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// filesystemTimeline: files modified within ±window_hours of now
// ─────────────────────────────────────────────────────────────────────────────

type timelineFile struct {
	Path    string `json:"path"`
	MTime   string `json:"mtime"`
	Size    int64  `json:"size_bytes"`
	SHA256  string `json:"sha256,omitempty"`
}

func (h *Handler) filesystemTimeline(_ context.Context, params map[string]string) (string, error) {
	windowHours := 6
	if v, ok := params["window_hours"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 72 {
			windowHours = n
		}
	}

	since := time.Now().Add(-time.Duration(windowHours) * time.Hour)

	scanDirs := []string{
		os.Getenv("TEMP"),
		os.Getenv("TMP"),
		filepath.Join(os.Getenv("APPDATA"), "Local", "Temp"),
		`C:\ProgramData`,
	}

	// Add user profile directories
	if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
		scanDirs = append(scanDirs,
			filepath.Join(userProfile, "Downloads"),
			filepath.Join(userProfile, "Desktop"),
			filepath.Join(userProfile, "Documents"),
		)
	}

	var files []timelineFile
	maxFiles := 500

	for _, dir := range scanDirs {
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || len(files) >= maxFiles {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if info.ModTime().After(since) {
				tf := timelineFile{
					Path:  path,
					MTime: info.ModTime().UTC().Format(time.RFC3339),
					Size:  info.Size(),
				}
				// Only hash small files to avoid perf impact
				if info.Size() < 50*1024*1024 {
					if h2, err := hashFile(path); err == nil {
						tf.SHA256 = h2
					}
				}
				files = append(files, tf)
			}
			return nil
		})
	}

	result := map[string]interface{}{
		"version":      1,
		"captured_at":  time.Now().UTC().Format(time.RFC3339),
		"window_hours": windowHours,
		"since":        since.UTC().Format(time.RFC3339),
		"files":        files,
		"count":        len(files),
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// networkLastSeen: last TCP connections + DNS queries
// ─────────────────────────────────────────────────────────────────────────────

func (h *Handler) networkLastSeen(_ context.Context, _ map[string]string) (string, error) {
	type connEntry struct {
		Proto       string `json:"proto"`
		LocalAddr   string `json:"local_addr"`
		RemoteAddr  string `json:"remote_addr"`
		State       string `json:"state"`
		PID         string `json:"pid,omitempty"`
		ProcessName string `json:"process_name,omitempty"`
	}

	var conns []connEntry

	// netstat -ano for active connections
	if out, err := exec.Command("netstat", "-ano").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			fields := strings.Fields(line)
			if len(fields) >= 5 && (fields[0] == "TCP" || fields[0] == "UDP") {
				c := connEntry{
					Proto:      fields[0],
					LocalAddr:  fields[1],
					RemoteAddr: fields[2],
				}
				if fields[0] == "TCP" && len(fields) >= 5 {
					c.State = fields[3]
					c.PID = fields[4]
				} else if fields[0] == "UDP" && len(fields) >= 4 {
					c.PID = fields[3]
				}
				conns = append(conns, c)
			}
		}
	}

	// Limit to 200 entries
	if len(conns) > 200 {
		conns = conns[:200]
	}

	// Recent DNS via ipconfig /displaydns (cached DNS)
	type dnsEntry struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Answer string `json:"answer,omitempty"`
	}
	var dnsEntries []dnsEntry

	if out, err := exec.Command("ipconfig", "/displaydns").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		var current dnsEntry
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Record Name") {
				if current.Name != "" {
					dnsEntries = append(dnsEntries, current)
				}
				parts := strings.SplitN(line, ":", 2)
				current = dnsEntry{Name: strings.TrimSpace(parts[len(parts)-1])}
			} else if strings.HasPrefix(line, "Record Type") {
				parts := strings.SplitN(line, ":", 2)
				current.Type = strings.TrimSpace(parts[len(parts)-1])
			} else if strings.HasPrefix(line, "A (Host) Record") || strings.HasPrefix(line, "CNAME Record") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					current.Answer = strings.TrimSpace(parts[1])
				}
			}
		}
		if current.Name != "" {
			dnsEntries = append(dnsEntries, current)
		}
	}
	if len(dnsEntries) > 200 {
		dnsEntries = dnsEntries[:200]
	}

	result := map[string]interface{}{
		"version":     1,
		"captured_at": time.Now().UTC().Format(time.RFC3339),
		"tcp_conns":   conns,
		"dns_cache":   dnsEntries,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// agentIntegrityCheck: verify agent binary signature + ETW health
// ─────────────────────────────────────────────────────────────────────────────

func (h *Handler) agentIntegrityCheck(_ context.Context, _ map[string]string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		exePath = ""
	}

	var binarySHA256 string
	if exePath != "" {
		binarySHA256, _ = hashFile(exePath)
	}

	signed := false
	if exePath != "" {
		signed = checkAuthenticode(exePath)
	}

	// Check if ETW tracing sessions are alive (simple heuristic: logman query)
	// Sessions created by the agent:
	//  - Kernel session name defaults to "EDRKernelTrace" in collectors/etw.go
	//  - Process access session defaults to "EDRProcAccessTrace" in collectors/process_access.go
	etwHealthy := false
	for _, sessionName := range []string{"EDRKernelTrace", "EDRProcAccessTrace"} {
		if out, err := exec.Command("logman", "query", sessionName).Output(); err == nil {
			if strings.Contains(strings.ToLower(string(out)), "running") {
				etwHealthy = true
				break
			}
		}
	}

	result := map[string]interface{}{
		"version":        1,
		"checked_at":     time.Now().UTC().Format(time.RFC3339),
		"exe_path":       exePath,
		"exe_sha256":     binarySHA256,
		"signature_valid": signed,
		"etw_healthy":    etwHealthy,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// hashFile computes the SHA-256 of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 64*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checkAuthenticode checks if a file has a valid Authenticode signature
// using sigcheck fallback: returns true if WinVerifyTrust succeeds.
func checkAuthenticode(path string) bool {
	// Use certutil or powershell as a lightweight check
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`(Get-AuthenticodeSignature "%s").Status`, path),
	).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "Valid"
}

// ─────────────────────────────────────────────────────────────────────────────
// memoryDump: acquire full RAM via WinPMEM (analyst-approved action)
// ─────────────────────────────────────────────────────────────────────────────

func (h *Handler) memoryDump(ctx context.Context, params map[string]string) (string, error) {
	// Locate winpmem binary next to the agent
	exeDir := filepath.Dir(func() string { p, _ := os.Executable(); return p }())
	winpmem := filepath.Join(exeDir, "winpmem.exe")

	if _, err := os.Stat(winpmem); err != nil {
		return "", fmt.Errorf("winpmem.exe not found at %s — memory dump unavailable", winpmem)
	}

	outputDir := params["output_dir"]
	if outputDir == "" {
		outputDir = filepath.Join(exeDir, "dumps")
	}
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return "", fmt.Errorf("create dump dir: %w", err)
	}

	dumpPath := filepath.Join(outputDir, fmt.Sprintf("mem_%s.dmp", time.Now().Format("20060102_150405")))

	cmd := exec.CommandContext(ctx, winpmem, dumpPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("winpmem failed: %w — output: %s", err, string(out))
	}

	// Hash the dump for integrity
	dumpHash, _ := hashFile(dumpPath)

	info, _ := os.Stat(dumpPath)
	var dumpSize int64
	if info != nil {
		dumpSize = info.Size()
	}

	result := map[string]interface{}{
		"version":    1,
		"dumped_at":  time.Now().UTC().Format(time.RFC3339),
		"path":       dumpPath,
		"size_bytes": dumpSize,
		"sha256":     dumpHash,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}
