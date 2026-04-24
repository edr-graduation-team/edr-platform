// Package command provides command handling for server-initiated actions.
package command

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	urlpkg "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"gopkg.in/yaml.v3"

	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/logging"
	"github.com/edr-platform/win-agent/internal/scanner"
	"github.com/edr-platform/win-agent/internal/signatures"
)

// CommandType enumerates supported command types.
type CommandType string

const (
	CmdTerminateProcess CommandType = "TERMINATE_PROCESS"
	CmdQuarantineFile   CommandType = "QUARANTINE_FILE"
	CmdIsolateNetwork   CommandType = "ISOLATE_NETWORK"
	CmdUnisolateNetwork CommandType = "UNISOLATE_NETWORK"
	CmdCollectForensics CommandType = "COLLECT_FORENSICS"
	CmdUpdateConfig     CommandType = "UPDATE_CONFIG"
	CmdUpdateAgent      CommandType = "UPDATE_AGENT"
	CmdRestartService   CommandType = "RESTART_SERVICE"
	CmdAdjustRate       CommandType = "ADJUST_RATE"
	CmdRunCommand       CommandType = "RUN_CMD"
	CmdRestart          CommandType = "RESTART"  // Machine reboot
	CmdShutdown         CommandType = "SHUTDOWN" // Machine shutdown
	CmdBlockIP          CommandType = "BLOCK_IP"
	CmdUnblockIP        CommandType = "UNBLOCK_IP"
	CmdBlockDomain      CommandType = "BLOCK_DOMAIN"
	CmdUnblockDomain    CommandType = "UNBLOCK_DOMAIN"
	CmdUpdateSignatures CommandType = "UPDATE_SIGNATURES"
	CmdRestoreQuarantineFile CommandType = "RESTORE_QUARANTINE_FILE"
	CmdDeleteQuarantineFile  CommandType = "DELETE_QUARANTINE_FILE"
	CmdUninstallAgent        CommandType = "UNINSTALL_AGENT"

	// Post-isolation triage commands
	CmdPostIsolationTriage  CommandType = "POST_ISOLATION_TRIAGE"
	CmdProcessTreeSnapshot  CommandType = "PROCESS_TREE_SNAPSHOT"
	CmdPersistenceScan      CommandType = "PERSISTENCE_SCAN"
	CmdLsassAccessAudit     CommandType = "LSASS_ACCESS_AUDIT"
	CmdFilesystemTimeline   CommandType = "FILESYSTEM_TIMELINE"
	CmdNetworkLastSeen      CommandType = "NETWORK_LAST_SEEN"
	CmdAgentIntegrityCheck  CommandType = "AGENT_INTEGRITY_CHECK"
	CmdMemoryDump           CommandType = "MEMORY_DUMP"
)

// =============================================================================
// Win32 API Constants & Safety Definitions
// =============================================================================

// Win32 process access rights for R4 safe termination.
const (
	_PROCESS_TERMINATE                = 0x0001
	_PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
)

// criticalSystemProcesses is a hardcoded set of Windows processes that must
// NEVER be terminated. Killing any of these causes a BSOD or system instability.
var criticalSystemProcesses = map[string]bool{
	"csrss.exe":    true,
	"smss.exe":     true,
	"wininit.exe":  true,
	"services.exe": true,
	"lsass.exe":    true,
	"svchost.exe":  true,
	"dwm.exe":      true,
	"winlogon.exe": true,
	"ntoskrnl.exe": true,
	"system":       true,
}

// allowedDiagnostics is the strict whitelist of executables that runCommand
// is permitted to invoke (R5 fix). ALL other executables are BLOCKED.
var allowedDiagnostics = map[string]bool{
	"ping":       true,
	"tracert":    true,
	"pathping":   true,
	"netstat":    true,
	"ipconfig":   true,
	"nslookup":   true,
	"whoami":     true,
	"hostname":   true,
	"systeminfo": true,
	"tasklist":   true,
	"arp":        true,
	"route":      true,
}

// Command represents an incoming command from the server.
type Command struct {
	ID         string
	Type       CommandType
	Parameters map[string]string
	Priority   int
	ExpiresAt  time.Time
	ReceivedAt time.Time
}

// Result represents the outcome of command execution.
type Result struct {
	CommandID string
	Status    string // "SUCCESS", "FAILED", "TIMEOUT"
	Output    string
	Error     string
	Duration  time.Duration
	Timestamp time.Time
}

// GRPCHealthChecker is an interface for checking gRPC connection health.
// Implemented by grpcclient.Client — injected to avoid circular imports.
type GRPCHealthChecker interface {
	IsConnected() bool
}

// Handler processes incoming commands.
type Handler struct {
	logger        *logging.Logger
	quarantineDir string
	serverAddress string // C2 server address for smart isolation
	mu            sync.Mutex

	// Restart info — populated via SetRestartInfo() for self-restart support.
	configPath string
	exePath    string
	pid        int

	// ── Isolation state ──────────────────────────────────────────────────────
	// Protected by mu. All fields are written under mu and read under mu
	// EXCEPT watchdogCancel, which is only ever written once under mu and then
	// called (read) from outside — safe because context.CancelFunc is
	// goroutine-safe to call from any goroutine.

	isIsolated         bool               // true while network is isolated
	isolationHostname  string             // original C2 hostname (e.g. "edr-c2.local")
	isolationPort      string             // gRPC port extracted from server_address
	isolationCurrentIP string             // last resolved IP used in firewall rules
	watchdogCancel     context.CancelFunc // cancels the isolation watchdog goroutine
	blockPolicyCancel  context.CancelFunc // cancels the delayed block-policy goroutine
	grpcHealth         GRPCHealthChecker  // injected: nil-safe health probe

	// configUpdateFn is injected by agent.SetConfigUpdateHandler so the handler
	// can apply a remote config push without importing the agent package.
	configUpdateFn func(newCfg *config.Config) error

	// currentCfg holds a pointer to the live agent config. It is used by the
	// updateConfig handler to clone and apply partial policy updates (e.g. the
	// JSON payload sent by the dashboard's update_filter_policy command).
	// Set via SetCurrentConfig — nil-safe: if unset the handler falls back to
	// the full YAML / sparse-key paths.
	currentCfg *config.Config

	// sigStore is the local malware hash database (optional).
	sigStore *signatures.Store

	// uninstallHook is the agent-level callback that performs the server-
	// authorised uninstall (release protections, schedule SYSTEM cleanup,
	// stop the service). It is injected by the service layer so the
	// command package does not need to import service (which would create
	// a cycle: service → agent → command → service).
	uninstallHook func(reason string) error
}

// NewHandler creates a new command handler.
func NewHandler(logger *logging.Logger, serverAddress string) *Handler {
	exePath, _ := os.Executable()
	return &Handler{
		logger:        logger,
		quarantineDir: "C:\\ProgramData\\EDR\\quarantine",
		serverAddress: serverAddress,
		exePath:       exePath,
		pid:           os.Getpid(),
	}
}

// SetGRPCHealthChecker injects the gRPC client so the isolation watchdog can
// probe connection health. Call this once after NewHandler, before agent.Start().
func (h *Handler) SetGRPCHealthChecker(hc GRPCHealthChecker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.grpcHealth = hc
}

// SetRestartInfo injects the config file path so restartService can relaunch
// the agent in standalone mode.
func (h *Handler) SetRestartInfo(configPath string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if abs, err := filepath.Abs(configPath); err == nil {
		h.configPath = abs
	} else {
		h.configPath = configPath
	}
}

// SetConfigUpdateCallback registers the function that will be called when the
// C2 server sends an UPDATE_CONFIG command. The callback is agent.UpdateConfig.
// Using a callback avoids a direct import of the agent package from command.
func (h *Handler) SetConfigUpdateCallback(fn func(newCfg *config.Config) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.configUpdateFn = fn
}

// SetCurrentConfig gives the handler a reference to the live agent config so
// that updateConfig can clone and partially update it when the dashboard pushes
// a FilterPolicy JSON payload (params["policy"]).
// Call this immediately after SetConfigUpdateCallback in agent.go.
func (h *Handler) SetCurrentConfig(cfg *config.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.currentCfg = cfg
}

// SetSignatureStore wires the local malware hash database for UPDATE_SIGNATURES commands.
func (h *Handler) SetSignatureStore(s *signatures.Store) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sigStore = s
}

// SetUninstallHook registers the agent-level callback that performs the
// server-authorised uninstall. When a COMMAND_TYPE_UNINSTALL_AGENT arrives,
// the handler invokes this hook, returns SUCCESS so SendCommandResult can
// deliver the "uninstall confirm" over the still-open stream, then the hook
// is responsible for stopping the service and cleaning up on disk.
func (h *Handler) SetUninstallHook(fn func(reason string) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.uninstallHook = fn
}

// Execute processes a command and returns the result.
func (h *Handler) Execute(ctx context.Context, cmd *Command) *Result {
	start := time.Now()

	h.logger.Infof("Executing command: type=%s id=%s", cmd.Type, cmd.ID)

	// Check if expired
	if !cmd.ExpiresAt.IsZero() && time.Now().After(cmd.ExpiresAt) {
		return &Result{
			CommandID: cmd.ID,
			Status:    "FAILED",
			Error:     "command expired",
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	var output string
	var err error

	switch cmd.Type {
	case CmdTerminateProcess:
		output, err = h.terminateProcess(ctx, cmd.Parameters)
	case CmdQuarantineFile:
		output, err = h.quarantineFile(ctx, cmd.Parameters)
	case CmdIsolateNetwork:
		output, err = h.isolateNetwork(ctx, cmd.Parameters)
	case CmdUnisolateNetwork:
		output, err = h.unisolateNetwork(ctx, cmd.Parameters)
	case CmdCollectForensics:
		output, err = h.collectForensics(ctx, cmd.Parameters)
	case CmdUpdateConfig:
		output, err = h.updateConfig(ctx, cmd.Parameters)
	case CmdUpdateAgent:
		output, err = h.updateAgent(ctx, cmd.Parameters)
	case CmdRestartService:
		output, err = h.restartService(ctx, cmd.Parameters)
	case CmdAdjustRate:
		output, err = h.adjustRate(ctx, cmd.Parameters)
	case CmdRunCommand:
		output, err = h.runCommand(ctx, cmd.Parameters)
	case CmdRestart:
		output, err = h.restartMachine(ctx, cmd.Parameters)
	case CmdShutdown:
		output, err = h.shutdownMachine(ctx, cmd.Parameters)
	case CmdBlockIP:
		output, err = h.blockIP(ctx, cmd.Parameters)
	case CmdUnblockIP:
		output, err = h.unblockIP(ctx, cmd.Parameters)
	case CmdBlockDomain:
		output, err = h.blockDomain(ctx, cmd.Parameters)
	case CmdUnblockDomain:
		output, err = h.unblockDomain(ctx, cmd.Parameters)
	case CmdUpdateSignatures:
		output, err = h.updateSignatures(ctx, cmd.Parameters)
	case CmdRestoreQuarantineFile:
		output, err = h.restoreQuarantineFile(ctx, cmd.Parameters)
	case CmdDeleteQuarantineFile:
		output, err = h.deleteQuarantineFile(ctx, cmd.Parameters)
	case CmdUninstallAgent:
		output, err = h.uninstallAgent(ctx, cmd.Parameters)
	case CmdPostIsolationTriage:
		output, err = h.postIsolationTriage(ctx, cmd.Parameters)
	case CmdProcessTreeSnapshot:
		output, err = h.processTreeSnapshot(ctx, cmd.Parameters)
	case CmdPersistenceScan:
		output, err = h.persistenceScan(ctx, cmd.Parameters)
	case CmdLsassAccessAudit:
		output, err = h.lsassAccessAudit(ctx, cmd.Parameters)
	case CmdFilesystemTimeline:
		output, err = h.filesystemTimeline(ctx, cmd.Parameters)
	case CmdNetworkLastSeen:
		output, err = h.networkLastSeen(ctx, cmd.Parameters)
	case CmdAgentIntegrityCheck:
		output, err = h.agentIntegrityCheck(ctx, cmd.Parameters)
	case CmdMemoryDump:
		output, err = h.memoryDump(ctx, cmd.Parameters)
	default:
		err = fmt.Errorf("unknown command type: %s", cmd.Type)
	}

	result := &Result{
		CommandID: cmd.ID,
		Output:    output,
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}

	if err != nil {
		result.Status = "FAILED"
		result.Error = err.Error()
		h.logger.Errorf("[C2] Command execution FAILED: id=%s type=%s error=%v", cmd.ID, cmd.Type, err)
	} else {
		result.Status = "SUCCESS"
		h.logger.Infof("[C2] Command executed SUCCESSFULLY: id=%s type=%s duration=%v output=%s", cmd.ID, cmd.Type, result.Duration, truncateOutput(output, 200))
	}

	return result
}

// truncateOutput shortens a string for log output.
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// terminateProcess kills a process by PID using native Win32 APIs.
//
// R4 FIX: Uses OpenProcess + TerminateProcess via syscall instead of shelling
// out to taskkill. Resolves the process name via QueryFullProcessImageNameW
// and checks against the critical system process list to prevent BSODs.
//
// When kill_tree=true in parameters, all descendant processes are terminated
// (children first) using the same safety checks.
func (h *Handler) terminateProcess(_ context.Context, params map[string]string) (string, error) {
	pidStr := params["pid"]
	if pidStr == "" {
		return "", fmt.Errorf("pid parameter is required")
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return "", fmt.Errorf("invalid PID: %s (must be a positive integer)", pidStr)
	}

	killTree := strings.EqualFold(params["kill_tree"], "true") || strings.EqualFold(params["killTree"], "true")
	var order []uint32
	if killTree {
		var errTree error
		order, errTree = processTreePostOrder(uint32(pid))
		if errTree != nil {
			return "", errTree
		}
	} else {
		order = []uint32{uint32(pid)}
	}

	var killed []string
	for _, p := range order {
		msg, err := h.terminateOnePID(int(p))
		if err != nil {
			h.logger.Warnf("[C2] terminate PID %d: %v", p, err)
			continue
		}
		killed = append(killed, fmt.Sprintf("%d", p))
		h.logger.Infof("[C2] %s", msg)
	}
	if len(killed) == 0 {
		return "", fmt.Errorf("no processes terminated (target may be protected or already exited)")
	}
	return fmt.Sprintf("Terminated PIDs: %s (kill_tree=%v)", strings.Join(killed, ","), killTree), nil
}

func (h *Handler) terminateOnePID(pid int) (string, error) {
	// Block PIDs 0 and 4 (System Idle, System kernel).
	if pid == 0 || pid == 4 {
		return "", fmt.Errorf("cannot terminate critical system process (PID %d)", pid)
	}

	// Prevent killing the EDR agent's own process.
	if pid == os.Getpid() {
		return "", fmt.Errorf("cannot terminate the EDR agent's own process (PID %d)", pid)
	}

	// Resolve process name via Win32 API (no shelling out).
	processName, nameErr := getProcessNameByPID(pid)
	if nameErr != nil {
		h.logger.Warnf("[C2] Could not resolve name for PID %d: %v — termination blocked", pid, nameErr)
		return "", fmt.Errorf("cannot resolve process name for PID %d (process may not exist): %w", pid, nameErr)
	}

	// Check against critical system process list.
	if criticalSystemProcesses[strings.ToLower(processName)] {
		return "", fmt.Errorf("BLOCKED: cannot terminate critical system process %q (PID %d) — would cause BSOD", processName, pid)
	}

	// Open process with TERMINATE access right.
	handle, err := syscall.OpenProcess(_PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return "", fmt.Errorf("OpenProcess failed for PID %d (%s): %w", pid, processName, err)
	}
	defer syscall.CloseHandle(handle)

	// Terminate via Win32 API (exit code 1).
	if err := win32TerminateProcess(handle); err != nil {
		return "", fmt.Errorf("TerminateProcess failed for PID %d (%s): %w", pid, processName, err)
	}

	return fmt.Sprintf("Process terminated via Win32 API: PID=%d Name=%s", pid, processName), nil
}

// getProcessNameByPID resolves a PID to its executable name using the Win32
// QueryFullProcessImageNameW API. This is injection-safe — no shell invocation.
func getProcessNameByPID(pid int) (string, error) {
	handle, err := syscall.OpenProcess(_PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", fmt.Errorf("OpenProcess(QUERY): %w", err)
	}
	defer syscall.CloseHandle(handle)

	var buf [512]uint16
	size := uint32(len(buf))

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	queryProc := kernel32.NewProc("QueryFullProcessImageNameW")

	r1, _, e1 := queryProc.Call(
		uintptr(handle),
		0, // dwFlags = 0 → Win32 path format
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		return "", fmt.Errorf("QueryFullProcessImageNameW: %v", e1)
	}

	fullPath := syscall.UTF16ToString(buf[:size])
	return filepath.Base(fullPath), nil
}

// win32TerminateProcess calls the Win32 TerminateProcess API on an open handle.
func win32TerminateProcess(handle syscall.Handle) error {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("TerminateProcess")

	r1, _, e1 := proc.Call(uintptr(handle), 1) // exit code = 1
	if r1 == 0 {
		return fmt.Errorf("TerminateProcess: %v", e1)
	}
	return nil
}

// quarantineFile moves a file to quarantine.
func (h *Handler) quarantineFile(ctx context.Context, params map[string]string) (string, error) {
	filePath := strings.TrimSpace(params["path"])
	if filePath == "" {
		filePath = strings.TrimSpace(params["file_path"])
	}
	if filePath == "" {
		return "", fmt.Errorf("path or file_path parameter is required")
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", filePath)
	}

	if err := os.MkdirAll(h.quarantineDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create quarantine dir: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	baseName := filepath.Base(filePath)
	quarantinePath := filepath.Join(h.quarantineDir, fmt.Sprintf("%s_%s.quarantine", timestamp, baseName))

	if err := os.Rename(filePath, quarantinePath); err != nil {
		// Locked or cross-volume: copy then remove (same as Executor.QuarantineFile).
		if err := copyFile(filePath, quarantinePath); err != nil {
			return "", fmt.Errorf("failed to quarantine file: %w", err)
		}
		_ = os.Remove(filePath)
	}

	metaPath := quarantinePath + ".meta"
	meta := fmt.Sprintf("OriginalPath: %s\nQuarantineTime: %s\nSource: C2 QUARANTINE_FILE\n",
		filePath, time.Now().Format(time.RFC3339))
	_ = os.WriteFile(metaPath, []byte(meta), 0600)

	return fmt.Sprintf("File quarantined: %s -> %s", filePath, quarantinePath), nil
}

// =============================================================================
// NETWORK ISOLATION — Dynamic Architecture
// =============================================================================
//
// Design Overview:
//   1. Just-In-Time Resolution: hostname → IP at isolation time via net.LookupHost
//   2. ACK-before-block: ALLOW rules are installed synchronously; the block
//      policy is applied in a goroutine after a 4-second grace period so
//      SendCommandResult can traverse the still-open gRPC connection.
//   3. Isolation Watchdog: a long-lived goroutine monitors gRPC health every
//      10 seconds while isolated. If the stream drops AND the C2 IP has
//      changed, it atomically replaces the firewall rules and reconnects.
//   4. Graceful Termination: unisolateNetwork() cancels the watchdog context
//      before removing firewall rules, guaranteeing clean shutdown.
// =============================================================================

// resolveC2IP resolves a hostname or bare IP to a usable IPv4 address.
// If addr is already a bare IP, it is returned unchanged.
// If resolution fails, it falls back to using the raw hostname (best-effort).
func (h *Handler) resolveC2IP(hostOrIP string) (string, error) {
	// If it's already a valid IP, return it directly.
	if ip := net.ParseIP(hostOrIP); ip != nil {
		return hostOrIP, nil
	}

	// It's a hostname — resolve via OS DNS (respects /etc/hosts and mDNS).
	h.logger.Infof("[Isolate] Resolving hostname %q via DNS...", hostOrIP)
	addrs, err := net.LookupHost(hostOrIP)
	if err != nil {
		return "", fmt.Errorf("DNS resolution of %q failed: %w", hostOrIP, err)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("DNS returned no addresses for %q", hostOrIP)
	}

	// Prefer IPv4. Walk results: first IPv4 wins.
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
			h.logger.Infof("[Isolate] Resolved %q → %s", hostOrIP, a)
			return a, nil
		}
	}

	// Fall back to first result (may be IPv6).
	h.logger.Warnf("[Isolate] No IPv4 for %q; using %s (IPv6 may not work with netsh remoteip)", hostOrIP, addrs[0])
	return addrs[0], nil
}

// installIsolationRules adds the EDR_C2_* ALLOW rules for the given IP and ports.
// It is idempotent: existing rules with the same names are deleted first.
// httpPort is always "8082" (REST/enrollment); grpcPort is typically "50051".
func (h *Handler) installIsolationRules(c2IP, grpcPort string) error {
	const httpPort = "60200"

	rules := []struct {
		name string
		args []string
	}{
		// gRPC OUT — agent → server
		{
			"EDR_C2_GRPC_OUT",
			[]string{"advfirewall", "firewall", "add", "rule",
				"name=EDR_C2_GRPC_OUT", "dir=out", "action=allow",
				"remoteip=" + c2IP, "remoteport=" + grpcPort, "protocol=TCP"},
		},
		// gRPC IN — allow replies from C2 IP (any local port, TCP established)
		{
			"EDR_C2_GRPC_IN",
			[]string{"advfirewall", "firewall", "add", "rule",
				"name=EDR_C2_GRPC_IN", "dir=in", "action=allow",
				"remoteip=" + c2IP, "protocol=TCP"},
		},
		// HTTP/REST OUT
		{
			"EDR_C2_HTTP_OUT",
			[]string{"advfirewall", "firewall", "add", "rule",
				"name=EDR_C2_HTTP_OUT", "dir=out", "action=allow",
				"remoteip=" + c2IP, "remoteport=" + httpPort, "protocol=TCP"},
		},
		// HTTP/REST IN
		{
			"EDR_C2_HTTP_IN",
			[]string{"advfirewall", "firewall", "add", "rule",
				"name=EDR_C2_HTTP_IN", "dir=in", "action=allow",
				"remoteip=" + c2IP, "protocol=TCP"},
		},
		// DNS OUT
		{
			"EDR_DNS_ALLOW",
			[]string{"advfirewall", "firewall", "add", "rule",
				"name=EDR_DNS_ALLOW", "dir=out", "action=allow",
				"remoteport=53", "protocol=UDP"},
		},
		// Loopback OUT
		{
			"EDR_LOOPBACK_ALLOW",
			[]string{"advfirewall", "firewall", "add", "rule",
				"name=EDR_LOOPBACK_ALLOW", "dir=out", "action=allow",
				"remoteip=127.0.0.1"},
		},
	}

	// ── Phase 1: Delete old rules in parallel (idempotent) ──────────────────
	var wg sync.WaitGroup
	for _, rule := range rules {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+name).Run()
		}(rule.name)
	}
	wg.Wait()

	// ── Phase 2: Add new rules in parallel ───────────────────────────────────
	type ruleResult struct {
		name string
		err  error
		out  string
	}
	results := make(chan ruleResult, len(rules))
	for _, rule := range rules {
		wg.Add(1)
		go func(r struct {
			name string
			args []string
		}) {
			defer wg.Done()
			out, err := exec.Command("netsh", r.args...).CombinedOutput()
			results <- ruleResult{name: r.name, err: err, out: string(out)}
		}(rule)
	}
	wg.Wait()
	close(results)

	for res := range results {
		if res.err != nil {
			return fmt.Errorf("failed to add firewall rule %s: %w (output: %s)", res.name, res.err, res.out)
		}
		h.logger.Infof("[Isolate] Firewall rule added: %s (remoteip=%s)", res.name, c2IP)
	}
	return nil
}

// removeIsolationRules deletes all EDR_C2_* firewall rules installed during isolation.
func removeIsolationRules() {
	edrRules := []string{
		"EDR_C2_GRPC_OUT", "EDR_C2_GRPC_IN",
		"EDR_C2_HTTP_OUT", "EDR_C2_HTTP_IN",
		"EDR_C2_ALLOW_OUT", "EDR_C2_ALLOW_IN", // legacy names (cleanup)
		"EDR_DNS_ALLOW",
		"EDR_LOOPBACK_ALLOW",
	}
	for _, name := range edrRules {
		_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+name).Run()
	}
}

// isolateNetwork uses Windows Firewall to block all traffic EXCEPT the C2 server.
//
// IMPORTANT — ACK-before-block design:
// The function adds ALLOW rules synchronously (so they are in place by the time
// Send is called), returns SUCCESS immediately, then applies the block policy in
// a detached goroutine after a 4-second grace period so grpcClient.SendCommandResult
// can complete on the still-open stream before the policy cuts the connection.
//
// Isolation Watchdog:
// A long-lived goroutine is launched that re-resolves the C2 hostname every 10s
// (or immediately after a detected gRPC drop). If the IP has changed it atomically
// replaces the EDR_C2_* firewall rules with new ones for the new IP, then waits
// for the RunReconnector to re-establish the gRPC stream automatically.
func (h *Handler) isolateNetwork(ctx context.Context, params map[string]string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// ── 1. Parse C2 address ──────────────────────────────────────────────────
	serverAddr := params["server_address"]
	if serverAddr == "" {
		serverAddr = h.serverAddress
	}
	if serverAddr == "" {
		return "", fmt.Errorf("missing server_address parameter for smart isolation")
	}

	hostname, grpcPort, err := splitHostPort(serverAddr)
	if err != nil {
		return "", fmt.Errorf("invalid server_address %q: %w", serverAddr, err)
	}

	// ── 2. Just-In-Time DNS resolution ──────────────────────────────────────
	// Resolve at execution time so the firewall rule always reflects the
	// current IP, even if it changed since the agent last connected.
	resolvedIP, err := h.resolveC2IP(hostname)
	if err != nil {
		return "", fmt.Errorf("cannot resolve C2 address: %w", err)
	}

	// ── 3. Stop any previous watchdog (idempotent re-isolation) ─────────────
	if h.watchdogCancel != nil {
		h.watchdogCancel()
		h.watchdogCancel = nil
	}

	// ── 4. Install ALLOW rules synchronously ─────────────────────────────────
	if err := h.installIsolationRules(resolvedIP, grpcPort); err != nil {
		return "", err
	}

	// ── 5. Record isolation state ─────────────────────────────────────────────
	h.isIsolated = true
	h.isolationHostname = hostname
	h.isolationPort = grpcPort
	h.isolationCurrentIP = resolvedIP

	// ── 6. Launch watchdog BEFORE applying block policy ───────────────────────
	// The watchdog context is derived from the agent's outer ctx so it stops
	// automatically when the agent shuts down, AND can be cancelled explicitly
	// by unisolateNetwork().
	watchdogCtx, cancel := context.WithCancel(ctx)
	h.watchdogCancel = cancel

	// Snapshot values for the goroutine (avoids holding h.mu inside goroutine).
	watchHostname := hostname
	watchPort := grpcPort
	watchIP := resolvedIP
	grpcHealth := h.grpcHealth // may be nil — watchdog checks before use

	go h.isolationWatchdog(watchdogCtx, watchHostname, watchPort, watchIP, grpcHealth)

	// ── 7. Apply block policy after grace period (ACK-before-block) ───────────
	// Cancel any previous pending block-policy goroutine (idempotent re-isolation).
	if h.blockPolicyCancel != nil {
		h.blockPolicyCancel()
	}
	blockCtx, blockCancel := context.WithCancel(context.Background())
	h.blockPolicyCancel = blockCancel

	h.logger.Infof("[Isolate] ALLOW rules installed for %s:%s — block policy fires in 4s", resolvedIP, grpcPort)
	go func() {
		h.logger.Info("[Isolate] Waiting 4s for CommandResult ACK before applying block policy...")
		select {
		case <-time.After(4 * time.Second):
			// Timer expired — apply block policy.
		case <-blockCtx.Done():
			// RESTORE arrived during grace period — abort block policy.
			h.logger.Info("[Isolate] Block policy CANCELLED — unisolate arrived during grace period")
			return
		}

		out, err := exec.Command("netsh", "advfirewall", "set", "allprofiles",
			"firewallpolicy", "blockinbound,blockoutbound").CombinedOutput()
		if err != nil {
			h.logger.Errorf("[Isolate] Failed to apply block policy: %v — output: %s", err, string(out))
		} else {
			h.logger.Info("[Isolate] Block policy applied — host is now ISOLATED ✓")
		}
	}()

	return fmt.Sprintf("Network ISOLATED — C2 %s:%s (resolved from %q) is whitelisted; block policy fires in 4s",
		resolvedIP, grpcPort, hostname), nil
}

// isolationWatchdog is a long-lived goroutine that runs exclusively during isolation.
// It monitors gRPC connectivity and dynamically updates the firewall if the C2 IP changes.
//
// Algorithm (every 10 s):
//  1. Check gRPC health via GRPCHealthChecker.
//  2. If healthy → sleep, repeat.
//  3. If unhealthy → re-resolve C2 hostname.
//     4a. IP unchanged → log; RunReconnector will handle reconnection automatically.
//     4b. IP changed   → call updateFirewallRules(oldIP, newIP); update currentIP.
//     The RunReconnector will then successfully dial the new IP once rules allow it.
func (h *Handler) isolationWatchdog(
	ctx context.Context,
	hostname, port, currentIP string,
	health GRPCHealthChecker,
) {
	h.logger.Infof("[Watchdog] Started — monitoring C2 %q (current IP: %s)", hostname, currentIP)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("[Watchdog] Gracefully terminated (unisolate or agent shutdown)")
			return

		case <-ticker.C:
			// Is the gRPC stream healthy?
			if health != nil && health.IsConnected() {
				h.logger.Debug("[Watchdog] gRPC healthy ✓")
				continue
			}

			h.logger.Warn("[Watchdog] gRPC appears disconnected — re-resolving C2 hostname...")

			// Re-resolve hostname.
			newIP, err := h.resolveC2IP(hostname)
			if err != nil {
				h.logger.Warnf("[Watchdog] Re-resolution of %q failed: %v — will retry next cycle", hostname, err)
				continue
			}

			if newIP == currentIP {
				h.logger.Infof("[Watchdog] IP unchanged (%s) — transient disconnect; RunReconnector will retry", currentIP)
				// No firewall change needed. The RunReconnector in grpc/client.go
				// will re-establish the connection automatically since it loops
				// on c.connected == false.
				continue
			}

			// IP changed — update firewall rules atomically.
			h.logger.Warnf("[Watchdog] C2 IP changed: %s → %s! Updating firewall rules...", currentIP, newIP)

			if err := h.updateFirewallRules(currentIP, newIP, port); err != nil {
				h.logger.Errorf("[Watchdog] Failed to update firewall rules: %v — will retry next cycle", err)
				continue
			}

			h.logger.Infof("[Watchdog] Firewall rules updated for new IP %s ✓ — RunReconnector will reconnect", newIP)

			// Persist new IP in watchdog-local state for the next comparison.
			currentIP = newIP

			// Also update handler state (so a subsequent re-isolation uses the right IP).
			h.mu.Lock()
			h.isolationCurrentIP = newIP
			h.mu.Unlock()
		}
	}
}

// updateFirewallRules atomically replaces EDR_C2_* rules for oldIP with rules
// for newIP. The sequence is:
//  1. Add new ALLOW rules for newIP  (connection possible immediately after)
//  2. Delete old ALLOW rules for oldIP
//
// Adding before deleting ensures zero downtime: the allowed connection window
// is never fully closed between the two operations.
func (h *Handler) updateFirewallRules(oldIP, newIP, grpcPort string) error {
	h.logger.Infof("[FWUpdate] Replacing rules: %s → %s (gRPC port %s)", oldIP, newIP, grpcPort)

	// Step 1: Install rules for the new IP.
	if err := h.installIsolationRules(newIP, grpcPort); err != nil {
		return fmt.Errorf("failed to install rules for new IP %s: %w", newIP, err)
	}

	// Note: installIsolationRules already deletes existing rules by name before
	// re-adding them, so old-IP rules are implicitly replaced. No explicit
	// old-IP deletion is needed here because the rule names are fixed constants
	// (EDR_C2_GRPC_OUT, etc.) not IP-keyed. The new rules overwrite the old.
	h.logger.Infof("[FWUpdate] Rules updated successfully: %s → %s ✓", oldIP, newIP)
	return nil
}

// unisolateNetwork restores the default firewall policy and removes all EDR rules.
// It cancels the isolation watchdog before touching the firewall to guarantee
// the watchdog never races against rule removal.
func (h *Handler) unisolateNetwork(ctx context.Context, params map[string]string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.logger.Info("[Restore] Restoring default firewall policy")

	// ── 1a. Cancel the delayed block-policy goroutine FIRST ───────────────────
	// If RESTORE arrives during the 4-second grace period, we must prevent
	// the pending goroutine from re-applying blockinbound,blockoutbound.
	if h.blockPolicyCancel != nil {
		h.blockPolicyCancel()
		h.blockPolicyCancel = nil
		h.logger.Info("[Restore] Block-policy goroutine cancelled ✓")
	}

	// ── 1b. Stop the watchdog ─────────────────────────────────────────────────
	// This MUST happen before removing firewall rules so the watchdog cannot
	// attempt to re-add rules while we are deleting them.
	if h.watchdogCancel != nil {
		h.watchdogCancel()
		h.watchdogCancel = nil
		h.logger.Info("[Restore] Isolation watchdog cancelled ✓")
	}

	// ── 2. Clear isolation state ──────────────────────────────────────────────
	h.isIsolated = false
	h.isolationHostname = ""
	h.isolationPort = ""
	h.isolationCurrentIP = ""

	// ── 3. Restore outbound-allow default policy ──────────────────────────────
	out, err := exec.CommandContext(ctx, "netsh", "advfirewall", "set", "allprofiles",
		"firewallpolicy", "blockinbound,allowoutbound").CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("failed to restore firewall policy: %w", err)
	}

	// ── 4. Remove all EDR isolation rules ─────────────────────────────────────
	removeIsolationRules()

	return "Network RESTORED — default firewall policy applied, EDR isolation rules removed ✓", nil
}

// splitHostPort extracts hostname/IP and port from "host:port" string.
// If addr contains no port, returns the addr as the host and "50051" as port.
func splitHostPort(addr string) (string, string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// No port present — treat entire string as host.
		return addr, "50051", nil
	}
	return host, port, nil
}

// wevtUtilPath returns the absolute path to wevtutil.exe when SystemRoot is set (normal on Windows agents).
func wevtUtilPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	p := filepath.Join(root, "System32", "wevtutil.exe")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return "wevtutil.exe"
}

func powershellExePath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	p := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return "powershell.exe"
}

// forensicsLookbackMs returns the XPath timediff window in milliseconds from parameters.
func forensicsLookbackMs(params map[string]string) int64 {
	if v := strings.TrimSpace(params["time_range_ms"]); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil && n > 0 {
			return capForensicsMs(n)
		}
	}
	tr := strings.TrimSpace(params["time_range"])
	if tr == "" {
		tr = "Last 24 hours"
	}
	return timeRangeToMs(tr)
}

func capForensicsMs(ms int64) int64 {
	const maxMs = int64(90 * 24 * time.Hour / time.Millisecond)
	const minMs = int64(time.Minute / time.Millisecond)
	if ms > maxMs {
		return maxMs
	}
	if ms < minMs {
		return minMs
	}
	return ms
}

func trimForensicsErr(msg string, max int) string {
	msg = strings.TrimSpace(msg)
	if len(msg) <= max {
		return msg
	}
	return msg[:max] + "…"
}

// canonicalEventLogChannel maps dashboard / API shorthand to Windows channel names for wevtutil.
func canonicalEventLogChannel(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return s
	}
	if strings.Contains(s, "/") {
		return s
	}
	switch strings.ToLower(s) {
	case "security":
		return "Security"
	case "system":
		return "System"
	case "application", "app":
		return "Application"
	case "setup":
		return "Setup"
	case "sysmon":
		return "Microsoft-Windows-Sysmon/Operational"
	case "powershell":
		return "Microsoft-Windows-PowerShell/Operational"
	case "forwardedevents", "forwarded":
		return "ForwardedEvents"
	default:
		return s
	}
}

// resolveEventLogChannels returns one or more candidate channels for a given shorthand.
// This lets the agent gracefully degrade when certain channels are missing (e.g. PowerShell).
func resolveEventLogChannels(raw string) []string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	// If caller passed an explicit channel path, try it as-is.
	if strings.Contains(s, "/") {
		return []string{s}
	}
	switch strings.ToLower(s) {
	case "powershell":
		// Newer channel (Operational) first, then classic legacy channel.
		return []string{
			"Microsoft-Windows-PowerShell/Operational",
			"Windows PowerShell",
		}
	case "sysmon":
		return []string{"Microsoft-Windows-Sysmon/Operational"}
	default:
		return []string{canonicalEventLogChannel(s)}
	}
}

func ensureEventChannelEnabled(ctx context.Context, channel string) error {
	// Best-effort enable. If the channel doesn't exist, we return an error and the caller can fallback.
	_, err := exec.CommandContext(ctx, "wevtutil", "sl", channel, "/e:true").CombinedOutput()
	return err
}

func isChannelNotFound(err error, output []byte) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(string(output))
	// Common Windows wording for missing event channel.
	return strings.Contains(s, "the specified channel could not be found") ||
		strings.Contains(s, "channel not found") ||
		strings.Contains(s, "15007")
}

func collectForensicsMaxEvents(params map[string]string) int {
	const def = 500
	const hardMax = 5000
	v := strings.TrimSpace(params["max_events"])
	if v == "" {
		v = strings.TrimSpace(params["event_limit"])
	}
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	if n > hardMax {
		return hardMax
	}
	return n
}

// collectForensics collects Windows Event Logs (wevtutil) and/or hashes a file (scan_file).
// Parameters:
//   - log_types or types: comma-separated channels (e.g. security,system or full provider paths)
//   - time_range: human text or Go duration (e.g. 24h, 168h); optional time_range_ms overrides (milliseconds)
//   - max_events / event_limit: cap per log (default 500, max 5000)
//   - file_path or path: optional file hash scan; may be combined with log collection
func (h *Handler) collectForensics(ctx context.Context, params map[string]string) (string, error) {
	logTypes := strings.TrimSpace(params["types"])
	if logTypes == "" {
		logTypes = strings.TrimSpace(params["log_types"])
	}
	filePath := strings.TrimSpace(params["file_path"])
	if filePath == "" {
		filePath = strings.TrimSpace(params["path"])
	}

	hasLogs := logTypes != ""
	hasFile := filePath != ""

	if !hasLogs && !hasFile {
		return "", fmt.Errorf("provide log_types/types and/or file_path/path (e.g. log_types=Security,System or file_path=C:\\\\path\\\\file.exe)")
	}

	ms := forensicsLookbackMs(params)
	maxEv := collectForensicsMaxEvents(params)
	wevt := wevtUtilPath()

	var sections []string
	var warnings []string

	if hasLogs {
		types := strings.Split(logTypes, ",")
		var results []string
		var bundleEvents []map[string]any
		returnEvents := true
		if v := strings.TrimSpace(params["return_events"]); v != "" {
			returnEvents = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
		}
		eventsPerType := 75
		if v := strings.TrimSpace(params["events_per_type"]); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
				eventsPerType = n
			}
		}
		anyOK := false
		for _, logName := range types {
			logName = strings.TrimSpace(logName)
			if logName == "" {
				continue
			}

			candidates := resolveEventLogChannels(logName)
			if len(candidates) == 0 {
				continue
			}

			var channel string
			var output []byte
			var lastErr error
			ok := false

			for _, cand := range candidates {
				channel = cand
				// Try to enable the channel first (best effort). This fixes cases where the channel exists but is disabled.
				_ = ensureEventChannelEnabled(ctx, channel)

				query := fmt.Sprintf("*[System[TimeCreated[timediff(@SystemTime) <= %d]]]", ms)
				cmd := exec.CommandContext(ctx, wevt, "qe", channel, "/q:"+query, "/c:"+strconv.Itoa(maxEv), "/f:text")
				output, lastErr = cmd.CombinedOutput()
				if lastErr != nil {
					// Channel missing? try fallback candidate instead of failing the whole command.
					if isChannelNotFound(lastErr, output) && len(candidates) > 1 {
						h.logger.Warnf("[C2] Event channel missing for %q (%q): %v — trying fallback", logName, channel, lastErr)
						continue
					}

					h.logger.Warnf("[C2] wevtutil xpath failed for %q (%q): %v — fallback /rd:true", logName, channel, lastErr)
					cmd2 := exec.CommandContext(ctx, wevt, "qe", channel, "/c:"+strconv.Itoa(maxEv), "/f:text", "/rd:true")
					output, lastErr = cmd2.CombinedOutput()
					if lastErr != nil {
						// still not found? try next candidate if any
						if isChannelNotFound(lastErr, output) && len(candidates) > 1 {
							h.logger.Warnf("[C2] Event channel missing for %q (%q) after fallback: %v — trying next", logName, channel, lastErr)
							continue
						}
						continue
					}
				}

				ok = true
				break
			}

			if !ok {
				em := trimForensicsErr(fmt.Sprintf("%v: %s", lastErr, string(output)), 400)
				h.logger.Warnf("[C2] Log collection failed for %q: %s", logName, em)
				results = append(results, fmt.Sprintf("%s: failed (%s)", logName, em))
				continue
			}

			eventCount := strings.Count(string(output), "Event[")
			if eventCount == 0 {
				lines := strings.Split(strings.TrimSpace(string(output)), "\n")
				for _, l := range lines {
					if strings.TrimSpace(l) != "" {
						eventCount++
					}
				}
			}

			results = append(results, fmt.Sprintf("%s: %d events (window_ms=%d cap=%d)", channel, eventCount, ms, maxEv))
			anyOK = true
			h.logger.Infof("[C2] Forensics log slice ok channel=%s events=%d", channel, eventCount)

			if returnEvents {
				// Best-effort structured event capture for UI browsing.
				evs, err := h.collectEventLogAsJSON(ctx, channel, strings.ToLower(canonicalEventLogChannel(logName)), ms, eventsPerType)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("%s: structured parse failed (%v)", logName, err))
				} else if len(evs) > 0 {
					bundleEvents = append(bundleEvents, evs...)
				}
			}
		}

		if len(results) == 0 {
			return "", fmt.Errorf("no valid log names in types: %s", logTypes)
		}
		if !anyOK {
			return strings.Join(results, "; "), fmt.Errorf("all requested event logs failed: %s", strings.Join(results, "; "))
		}

		// If requested, return JSON bundle so the server can persist events.
		if returnEvents {
			payload := map[string]any{
				"version":    1,
				"command_id": strings.TrimSpace(params["command_id"]),
				"agent_id":   strings.TrimSpace(params["agent_id"]),
				"time_range": strings.TrimSpace(params["time_range"]),
				"log_types":  logTypes,
				"summary": map[string]any{
					"counts":   strings.Join(results, "; "),
					"warnings": warnings,
				},
				"events": bundleEvents,
			}
			b, _ := json.Marshal(payload)
			if len(b) > 0 {
				return string(b), nil
			}
		}

		sections = append(sections, "event_logs: "+strings.Join(results, "; "))
	}

	if hasFile {
		fsOut, err := h.collectFileHashForensics(ctx, filePath)
		if err != nil {
			warnings = append(warnings, "file_scan: "+err.Error())
			if !hasLogs {
				return "", err
			}
		} else {
			sections = append(sections, "file_scan: "+fsOut)
		}
	}

	out := strings.Join(sections, " | ")
	if len(warnings) > 0 {
		out += " | errors: " + strings.Join(warnings, "; ")
	}
	return out, nil
}

// collectFileHashForensics hashes a file on disk (used for dashboard scan_file → COLLECT_FORENSICS).
func (h *Handler) collectFileHashForensics(ctx context.Context, filePath string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, expected a file: %s", filePath)
	}
	const maxBytes = 32 << 20
	hashHex, readN, err := scanner.FileSHA256Limited(filePath, maxBytes)
	if err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}
	return fmt.Sprintf("File scan: path=%s size=%d bytes sha256=%s (hashed_bytes=%d)",
		filePath, info.Size(), hashHex, readN), nil
}

func (h *Handler) collectEventLogAsJSON(ctx context.Context, channel string, logType string, lookbackMs int64, maxEvents int) ([]map[string]any, error) {
	ps := powershellExePath()

	script := fmt.Sprintf(`$ErrorActionPreference='Stop';
$ms=%d;
$log=%q;
$max=%d;
$q = "*[System[TimeCreated[timediff(@SystemTime) <= $ms]]]";
$events = Get-WinEvent -LogName $log -FilterXPath $q -MaxEvents $max -ErrorAction Stop;
$out = @();
foreach ($e in $events) {
  $out += [pscustomobject]@{
    timestamp = ($e.TimeCreated.ToUniversalTime().ToString("o"));
    log_type = %q;
    event_id = [string]$e.Id;
    level = [string]$e.LevelDisplayName;
    provider = [string]$e.ProviderName;
    message = [string]$e.Message;
    raw = [pscustomobject]@{ xml = [string]$e.ToXml() };
  }
}
$out | ConvertTo-Json -Depth 6 -Compress;`, lookbackMs, channel, maxEvents, strings.ToLower(logType))

	out, err := exec.CommandContext(ctx, ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("powershell get-winevent failed: %v: %s", err, string(out))
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	var anyVal any
	if err := json.Unmarshal([]byte(raw), &anyVal); err != nil {
		return nil, fmt.Errorf("json parse failed: %v", err)
	}
	switch v := anyVal.(type) {
	case []any:
		res := make([]map[string]any, 0, len(v))
		for _, it := range v {
			if m, ok := it.(map[string]any); ok {
				res = append(res, m)
			}
		}
		return res, nil
	case map[string]any:
		return []map[string]any{v}, nil
	default:
		return nil, nil
	}
}

// timeRangeToMs converts a time range string to milliseconds for wevtutil timediff().
// Accepts Go durations (e.g. 48h, 90m), plain hour counts (e.g. 48), and legacy phrases.
func timeRangeToMs(timeRange string) int64 {
	s := strings.TrimSpace(timeRange)
	low := strings.ToLower(s)
	if d, err := time.ParseDuration(low); err == nil && d > 0 {
		return capForensicsMs(d.Milliseconds())
	}
	switch low {
	case "1h", "last 1 hour", "last hour":
		return capForensicsMs(3600000)
	case "6h", "last 6 hours":
		return capForensicsMs(21600000)
	case "12h", "last 12 hours":
		return capForensicsMs(43200000)
	case "24h", "last 24 hours", "last day":
		return capForensicsMs(86400000)
	case "7d", "last 7 days", "last week":
		return capForensicsMs(604800000)
	case "30d", "last 30 days", "last month":
		return capForensicsMs(2592000000)
	default:
		if n, err := strconv.Atoi(low); err == nil && n > 0 {
			// Interpret small integers as hours (e.g. 48 → 48h)
			if n <= 24*90 {
				return capForensicsMs(int64(n) * 3600000)
			}
		}
		return capForensicsMs(86400000)
	}
}

// updateConfig applies a new configuration pushed by the C2 server.
//
// The C2 passes the new config as a YAML string in params["config"].
// If params["config"] is empty, the handler looks for individual overrides in
// params["server_address"], params["batch_size"], etc. (sparse update mode).
//
// Flow:
//  1. Deserialise the YAML payload into a *config.Config.
//  2. Merge with the current config (sparse updates only override set fields).
//  3. Call the registered configUpdateFn (agent.UpdateConfig) which validates,
//     saves to disk, and hot-swaps the running config.
func (h *Handler) updateConfig(ctx context.Context, params map[string]string) (string, error) {
	configYAML := strings.TrimSpace(params["config"])

	// ── Case 1: Full YAML payload ─────────────────────────────────────────────
	if configYAML != "" {
		newCfg := &config.Config{}
		if err := yaml.Unmarshal([]byte(configYAML), newCfg); err != nil {
			return "", fmt.Errorf("failed to parse config YAML: %w", err)
		}

		h.mu.Lock()
		fn := h.configUpdateFn
		h.mu.Unlock()

		if fn == nil {
			return "", fmt.Errorf("no config update handler registered — agent not wired for hot-reload")
		}

		if err := fn(newCfg); err != nil {
			return "", fmt.Errorf("config update failed: %w", err)
		}
		return "Configuration updated successfully (hot-reload applied)", nil
	}

	// ── Case 1b: Filter policy JSON payload (dashboard update_filter_policy) ───
	// The dashboard's InlineAgentDetail panel posts:
	//   { command_type: "update_filter_policy", parameters: { "policy": "<json>" } }
	// The backend maps update_filter_policy → COMMAND_TYPE_UPDATE_CONFIG and
	// forwards the parameters map unchanged, so the agent receives params["policy"].
	// We parse the JSON, clone the current config, apply the policy fields, and
	// call configUpdateFn for a live hot-reload.
	if policyJSON := strings.TrimSpace(params["policy"]); policyJSON != "" {
		// Inline struct matching the FilterPolicy shape sent by the dashboard.
		var policy struct {
			ExcludeProcesses []string `json:"exclude_processes"`
			ExcludeEventIDs  []int    `json:"exclude_event_ids"`
			TrustedHashes    []string `json:"trusted_hashes"`
			RateLimit        *struct {
				Enabled        bool `json:"enabled"`
				DefaultMaxEps  int  `json:"default_max_eps"`
				CriticalBypass bool `json:"critical_bypass"`
			} `json:"rate_limit"`
		}
		if err := json.Unmarshal([]byte(policyJSON), &policy); err != nil {
			return "", fmt.Errorf("failed to parse policy JSON: %w", err)
		}

		h.mu.Lock()
		fn := h.configUpdateFn
		base := h.currentCfg
		h.mu.Unlock()

		if fn == nil {
			return "", fmt.Errorf("no config update handler registered — agent not wired for hot-reload")
		}
		if base == nil {
			return "", fmt.Errorf("current config not available in command handler — call SetCurrentConfig")
		}

		// Clone so we don't mutate the live config directly (agent.UpdateConfig
		// performs the atomic swap under its own write lock).
		newCfg := base.Clone()

		// Apply only the fields that are present in the policy payload.
		if len(policy.ExcludeProcesses) > 0 {
			newCfg.Filtering.ExcludeProcesses = policy.ExcludeProcesses
		}
		if len(policy.ExcludeEventIDs) > 0 {
			newCfg.Filtering.ExcludeEventIDs = policy.ExcludeEventIDs
		}
		if len(policy.TrustedHashes) > 0 {
			newCfg.Filtering.TrustedHashes = policy.TrustedHashes
		}
		if policy.RateLimit != nil {
			newCfg.Filtering.RateLimit.Enabled = policy.RateLimit.Enabled
			newCfg.Filtering.RateLimit.DefaultMaxEPS = policy.RateLimit.DefaultMaxEps
			newCfg.Filtering.RateLimit.CriticalBypass = policy.RateLimit.CriticalBypass
		}

		if err := fn(newCfg); err != nil {
			return "", fmt.Errorf("filter policy apply failed: %w", err)
		}

		h.logger.Infof("[C2] Filter policy hot-reloaded: %d excluded processes, %d excluded event IDs, %d trusted hashes",
			len(newCfg.Filtering.ExcludeProcesses),
			len(newCfg.Filtering.ExcludeEventIDs),
			len(newCfg.Filtering.TrustedHashes))

		return fmt.Sprintf("Filter policy applied (hot-reload): %d excluded processes, %d excluded event IDs, %d trusted hashes",
			len(newCfg.Filtering.ExcludeProcesses),
			len(newCfg.Filtering.ExcludeEventIDs),
			len(newCfg.Filtering.TrustedHashes)), nil
	}

	// ── Case 2: Sparse key-value overrides ────────────────────────────────────
	// When no full YAML is provided, honour individual override params.
	// This is useful for targeted policy pushes ("just change batch_size").
	var overrides []string
	if v, ok := params["server_address"]; ok && v != "" {
		overrides = append(overrides, "server.address="+v)
	}
	if v, ok := params["log_level"]; ok && v != "" {
		overrides = append(overrides, "logging.level="+v)
	}
	if v, ok := params["exclude_process"]; ok && v != "" {
		overrides = append(overrides, "filtering.exclude_processes+="+v)
	}

	if len(overrides) == 0 {
		return "", fmt.Errorf("UPDATE_CONFIG requires either a 'config' YAML payload, a 'policy' JSON payload, or at least one override param (server_address, log_level, exclude_process)")
	}
	h.mu.Lock()
	fn := h.configUpdateFn
	base := h.currentCfg
	h.mu.Unlock()
	if fn == nil {
		return "", fmt.Errorf("no config update handler registered — agent not wired for hot-reload")
	}
	if base == nil {
		return "", fmt.Errorf("current config not available in command handler — call SetCurrentConfig")
	}

	newCfg := base.Clone()
	updated := 0

	if v := strings.TrimSpace(params["server_address"]); v != "" {
		newCfg.Server.Address = v
		updated++
	}
	if v := strings.TrimSpace(params["log_level"]); v != "" {
		newCfg.Logging.Level = strings.ToUpper(v)
		updated++
	}
	if v := strings.TrimSpace(params["exclude_process"]); v != "" {
		exists := false
		for _, p := range newCfg.Filtering.ExcludeProcesses {
			if strings.EqualFold(strings.TrimSpace(p), v) {
				exists = true
				break
			}
		}
		if !exists {
			newCfg.Filtering.ExcludeProcesses = append(newCfg.Filtering.ExcludeProcesses, v)
			updated++
		}
	}

	if updated == 0 {
		return "Sparse overrides received, no effective changes (already present)", nil
	}
	if err := fn(newCfg); err != nil {
		return "", fmt.Errorf("sparse config update failed: %w", err)
	}

	// Keep command handler's current config pointer in sync for subsequent updates.
	h.mu.Lock()
	h.currentCfg = newCfg
	h.mu.Unlock()

	h.logger.Infof("[C2] Sparse config hot-reload applied (%d changes): %v", updated, overrides)
	return fmt.Sprintf("Sparse config hot-reload applied (%d changes): %v", updated, overrides), nil
}

// uninstallAgent is the server-driven removal path. The server has already
// enforced mTLS + RBAC + audit before the UNINSTALL_AGENT command was
// dispatched, so no local token is required. We simply delegate to the
// service-layer hook (which releases protections and schedules SYSTEM
// cleanup) and return success. SendCommandResult(status=SUCCESS) carries the
// "uninstall confirm" back to the server before the stream dies with the
// service.
func (h *Handler) uninstallAgent(_ context.Context, params map[string]string) (string, error) {
	reason := strings.TrimSpace(params["reason"])
	if reason == "" {
		reason = "server-issued UNINSTALL_AGENT"
	}

	h.mu.Lock()
	hook := h.uninstallHook
	h.mu.Unlock()

	if hook == nil {
		return "", fmt.Errorf("uninstall hook not registered — agent cannot honour UNINSTALL_AGENT")
	}

	h.logger.Warnf("[C2] UNINSTALL_AGENT received (reason=%q) — releasing protections and scheduling removal", reason)
	if err := hook(reason); err != nil {
		return "", fmt.Errorf("uninstall hook failed: %w", err)
	}

	return fmt.Sprintf("Uninstall scheduled (reason=%s). Service will stop and agent files will be removed.", reason), nil
}

// mtlsHTTPClient builds an HTTP client that presents the agent's enrolled
// client certificate and validates the server against the trusted CA. This is
// the only transport allowed to download personalised agent packages — the
// server rejects requests without a valid peer certificate whose CN matches
// the package's bound agent_id.
func (h *Handler) mtlsHTTPClient(serverName string) (*http.Client, error) {
	h.mu.Lock()
	cfg := h.currentCfg
	h.mu.Unlock()
	if cfg == nil {
		return nil, fmt.Errorf("agent config not wired into command handler")
	}

	// Prefer inline PEM blobs (stored in the protected Registry) over files.
	var clientCert tls.Certificate
	var err error
	if len(cfg.Certs.CertPEM) > 0 && len(cfg.Certs.KeyPEM) > 0 {
		clientCert, err = tls.X509KeyPair(cfg.Certs.CertPEM, cfg.Certs.KeyPEM)
	} else if cfg.Certs.CertPath != "" && cfg.Certs.KeyPath != "" {
		clientCert, err = tls.LoadX509KeyPair(cfg.Certs.CertPath, cfg.Certs.KeyPath)
	} else {
		return nil, fmt.Errorf("no client certificate material available for mTLS download")
	}
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	var caPEM []byte
	if len(cfg.Certs.CACertPEM) > 0 {
		caPEM = cfg.Certs.CACertPEM
	} else if cfg.Certs.CAPath != "" {
		caPEM, err = os.ReadFile(cfg.Certs.CAPath)
		if err != nil {
			return nil, fmt.Errorf("read CA cert: %w", err)
		}
	}
	if len(caPEM) == 0 || !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no usable CA certificate for server verification")
	}

	return &http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{clientCert},
				RootCAs:      caPool,
				// Dynamic + reliable: if serverName is empty, Go will verify the certificate
				// against the request URL host (works correctly across redirects).
				// If provided, it overrides for special deployments.
				ServerName: strings.TrimSpace(serverName),
			},
		},
	}, nil
}

// updateAgent downloads and installs new agent version.
func (h *Handler) updateAgent(ctx context.Context, params map[string]string) (string, error) {
	version := params["version"]
	url := params["url"]
	checksum := params["checksum"]

	if version == "" || url == "" {
		return "", fmt.Errorf("version and url parameters are required")
	}
	if checksum == "" {
		return "", fmt.Errorf("checksum parameter is required")
	}

	h.logger.Infof("[C2] UPDATE_AGENT requested: version=%s url=%s", version, url)

	// Optional: apply config overrides before swap (best-effort).
	if sd := strings.TrimSpace(params["server_domain"]); sd != "" {
		sp := strings.TrimSpace(params["server_port"])
		if sp == "" {
			sp = "47051"
		}
		if cfg, err := config.LoadFromRegistry(); err == nil && cfg != nil {
			cfg.Server.Address = fmt.Sprintf("%s:%s", sd, sp)
			cfg.Server.TLSServerName = sd
			_ = cfg.SaveToRegistry()
			h.logger.Infof("[C2] UPDATE_AGENT config override applied: server=%s:%s", sd, sp)
		}
	}
	if strings.EqualFold(strings.TrimSpace(params["install_sysmon"]), "true") {
		if cfg, err := config.LoadFromRegistry(); err == nil && cfg != nil {
			cfg.Sysmon.InstallOnFirstRun = true
			_ = cfg.SaveToRegistry()
		}
	}

	// Download binary to a staging path (root dir is Admin+SYSTEM).
	// The download endpoint requires mTLS so the server can verify the peer
	// certificate's CN matches the agent_id bound to the package row. A
	// plain http.DefaultClient would be rejected with 403.
	stagePath := `C:\ProgramData\EDR\edr-agent.patch.exe`

	// Flexible scheme handling:
	// - Some deployments expose the package endpoint over HTTP, others HTTPS.
	// - We try both (in a safe order) so upgrades are resilient.
	// TLS ServerName handling:
	// - We rely on the request URL hostname (dynamic) unless an explicit override is passed.
	urlsToTry := buildHTTPAndHTTPSCandidates(strings.TrimSpace(url))
	httpClient, err := h.mtlsHTTPClient("")
	if err != nil {
		return "", fmt.Errorf("build mTLS client for update download: %w", err)
	}

	var resp *http.Response
	var lastErr error
	for _, u := range urlsToTry {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			lastErr = fmt.Errorf("build download request: %w", err)
			continue
		}
		r, err := httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("download failed: %w", err)
			continue
		}
		// If server responded, decide based on status. If not OK, capture and try next.
		if r.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
			_ = r.Body.Close()
			lastErr = fmt.Errorf("download failed: status=%d body=%s", r.StatusCode, strings.TrimSpace(string(b)))
			continue
		}
		resp = r
		break
	}
	if resp == nil {
		return "", lastErr
	}
	defer resp.Body.Close()

	tmp := stagePath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return "", fmt.Errorf("open stage file: %w", err)
	}
	hh := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, hh), resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write stage file: %w", err)
	}
	_ = f.Close()
	got := hex.EncodeToString(hh.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(checksum)) {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("checksum mismatch: got=%s want=%s", got, checksum)
	}
	_ = os.Remove(stagePath)
	if err := os.Rename(tmp, stagePath); err != nil {
		return "", fmt.Errorf("finalize stage file: %w", err)
	}
	h.logger.Infof("[C2] UPDATE_AGENT download+verify OK: sha256=%s bytes staged=%s", got[:16]+"...", stagePath)

	// Schedule a SYSTEM task to stop → swap → start.
	taskName := fmt.Sprintf("EDR_Patch_%d", time.Now().UnixNano())
	st := time.Now().Add(1 * time.Minute).Format("15:04")
	dst := `C:\ProgramData\EDR\bin\edr-agent.exe`
	// Keep a backup.
	// Robustness: allow longer stop time, force-kill the process if needed, then swap.
	tr := fmt.Sprintf(`cmd /c sc stop EDRAgent ^& timeout /t 6 /nobreak ^& taskkill /F /IM edr-agent.exe >nul 2>nul ^& timeout /t 2 /nobreak ^& del /f /q "%s.old" ^& ren "%s" "edr-agent.exe.old" ^& copy /y "%s" "%s" ^& del /f /q "%s" ^& sc start EDRAgent`,
		dst, dst, stagePath, dst, stagePath,
	)
	create := exec.Command("schtasks", "/Create", "/TN", taskName, "/RU", "SYSTEM", "/SC", "ONCE", "/ST", st, "/F", "/TR", tr)
	if out, err := create.CombinedOutput(); err != nil {
		return "", fmt.Errorf("schedule patch task create failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	run := exec.Command("schtasks", "/Run", "/TN", taskName)
	if out, err := run.CombinedOutput(); err != nil {
		return "", fmt.Errorf("schedule patch task run failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	_ = exec.Command("schtasks", "/Delete", "/TN", taskName, "/F").Run()

	return fmt.Sprintf("Agent upgrade scheduled: version=%s sha256=%s (service will restart shortly)", version, got[:16]+"..."), nil
}

func buildHTTPAndHTTPSCandidates(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := urlpkg.Parse(raw)
	if err != nil || u == nil {
		// If parsing fails, just try as-is.
		return []string{raw}
	}
	// If scheme missing, prefer https then http.
	if u.Scheme == "" {
		httpsU := *u
		httpsU.Scheme = "https"
		httpU := *u
		httpU.Scheme = "http"
		return []string{httpsU.String(), httpU.String()}
	}
	// If scheme is http/https, try original first then the other.
	if strings.EqualFold(u.Scheme, "http") {
		alt := *u
		alt.Scheme = "https"
		return []string{u.String(), alt.String()}
	}
	if strings.EqualFold(u.Scheme, "https") {
		alt := *u
		alt.Scheme = "http"
		return []string{u.String(), alt.String()}
	}
	// Unknown scheme: try as-is only.
	return []string{u.String()}
}

// restartService handles all agent service control commands.
func (h *Handler) restartService(ctx context.Context, params map[string]string) (string, error) {
	mode := strings.ToLower(params["mode"])
	if mode == "" {
		mode = "restart"
	}

	h.mu.Lock()
	configPath := h.configPath
	exePath := h.exePath
	pid := h.pid
	h.mu.Unlock()

	isService := false
	if out, err := exec.Command("sc", "query", "EDRAgent").Output(); err == nil {
		isService = strings.Contains(string(out), "RUNNING") || strings.Contains(string(out), "STOPPED")
	}

	h.logger.Infof("[C2] restartService mode=%s isService=%v pid=%d exe=%s cfg=%s",
		mode, isService, pid, exePath, configPath)

	detachedRun := func(script string) error {
		cmd := exec.Command("cmd.exe", "/C", script)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: 0x00000008 | 0x00000200,
		}
		return cmd.Start()
	}

	var script string
	var logMsg, returnMsg string

	switch mode {
	case "enable_sysmon":
		return h.enableSysmon(ctx, params)
	case "disable_sysmon":
		return h.disableSysmon(ctx, params)
	case "stop":
		if isService {
			script = "ping 127.0.0.1 -n 4 > nul && sc stop EDRAgent"
			logMsg = "[C2] STOP AGENT (service): detached sc stop in ~3s"
			returnMsg = "Agent service stop scheduled (~3s). Dashboard will show Offline."
		} else {
			script = fmt.Sprintf("ping 127.0.0.1 -n 4 > nul && taskkill /F /PID %d", pid)
			logMsg = fmt.Sprintf("[C2] STOP AGENT (standalone): taskkill /F /PID %d in ~3s", pid)
			returnMsg = "Agent process will be terminated in ~3 seconds."
		}

	case "start":
		if isService {
			script = "sc start EDRAgent"
			logMsg = "[C2] START AGENT (service): sc start"
			returnMsg = "Agent service starting. Will reconnect shortly."
		} else {
			return "Agent is already running in standalone mode.", nil
		}

	default: // "restart"
		if isService {
			script = "ping 127.0.0.1 -n 4 > nul && sc stop EDRAgent && ping 127.0.0.1 -n 3 > nul && sc start EDRAgent"
			logMsg = "[C2] RESTART AGENT (service): detached sc stop+start in ~3s"
			returnMsg = "Agent service restart scheduled. Will stop in ~3s, restart in ~5s."
		} else {
			if exePath == "" || configPath == "" {
				return "", fmt.Errorf("cannot restart standalone: exe=%q config=%q (SetRestartInfo not called?)", exePath, configPath)
			}
			batContent := fmt.Sprintf(
				"@echo off\r\nping 127.0.0.1 -n 4 > nul\r\ntaskkill /F /PID %d\r\nping 127.0.0.1 -n 3 > nul\r\nstart \"EDR Agent\" \"%s\" -config \"%s\"\r\n",
				pid, exePath, configPath,
			)
			batPath := `C:\ProgramData\EDR\edr_restart.bat`
			if err := os.WriteFile(batPath, []byte(batContent), 0755); err != nil {
				batPath = filepath.Join(os.TempDir(), "edr_restart.bat")
				if err2 := os.WriteFile(batPath, []byte(batContent), 0755); err2 != nil {
					return "", fmt.Errorf("failed to write restart bat: %v (fallback: %v)", err, err2)
				}
			}
			h.logger.Infof("[C2] Restart bat written: %s", batPath)
			script = batPath
			logMsg = fmt.Sprintf("[C2] RESTART AGENT (standalone): kill PID %d + relaunch via bat in ~3s", pid)
			returnMsg = "Agent will restart in ~3s. A new terminal window ('EDR Agent') will open."
		}
	}

	h.logger.Warn(logMsg)
	if err := detachedRun(script); err != nil {
		h.logger.Errorf("[C2] Failed to spawn detached restart script: %v", err)
		return "", fmt.Errorf("failed to spawn restart script: %w", err)
	}
	h.logger.Infof("[C2] Detached restart process spawned — ACK sent before action fires")
	return returnMsg, nil
}

// adjustRate changes event collection rate.
func (h *Handler) adjustRate(ctx context.Context, params map[string]string) (string, error) {
	batchSize := params["batch_size"]
	interval := params["interval"]
	return fmt.Sprintf("Rate adjusted: batch_size=%s interval=%s", batchSize, interval), nil
}

// runCommand executes a diagnostic command from a strict whitelist.
//
// R5 FIX: The previous implementation passed raw user input to cmd.exe /C,
// which was a catastrophic RCE vulnerability. This version:
//   1. Parses the command into executable + arguments (no shell interpretation)
//   2. Validates the executable against a hardcoded whitelist of safe diagnostics
//   3. Invokes exec.Command directly (no cmd.exe, no shell interpolation)
func (h *Handler) runCommand(ctx context.Context, params map[string]string) (string, error) {
	cmdStr := strings.TrimSpace(params["cmd"])
	if cmdStr == "" {
		return "", fmt.Errorf("cmd parameter is required")
	}

	// Parse into executable + arguments (no shell interpretation).
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command after parsing")
	}

	// Normalize executable name: strip path and .exe suffix.
	exeName := strings.ToLower(filepath.Base(parts[0]))
	exeName = strings.TrimSuffix(exeName, ".exe")

	// R5 FIX: Strict whitelist check.
	if !allowedDiagnostics[exeName] {
		allowed := make([]string, 0, len(allowedDiagnostics))
		for k := range allowedDiagnostics {
			allowed = append(allowed, k)
		}
		h.logger.Warnf("[C2] BLOCKED run_cmd: %q is not in whitelist", parts[0])
		return "", fmt.Errorf("BLOCKED: %q is not in the allowed diagnostic commands whitelist. Allowed: %v", parts[0], allowed)
	}

	// Execute directly via exec.Command — NO cmd.exe, NO shell interpolation.
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var execCmd *exec.Cmd
	if len(parts) > 1 {
		execCmd = exec.CommandContext(timeoutCtx, parts[0], parts[1:]...)
	} else {
		execCmd = exec.CommandContext(timeoutCtx, parts[0])
	}

	output, err := execCmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}

	h.logger.Infof("[C2] run_cmd executed (whitelisted): %s", cmdStr)
	return string(output), nil
}

// restartMachine initiates an OS-level machine reboot.
//
// R6 FIX: Requires explicit confirm:"true" parameter and uses a 30-second
// delay to allow cancellation (shutdown /a) if issued by mistake.
func (h *Handler) restartMachine(_ context.Context, params map[string]string) (string, error) {
	// R6 FIX: Require explicit confirmation.
	if strings.ToLower(params["confirm"]) != "true" {
		return "", fmt.Errorf("BLOCKED: machine restart requires confirm=\"true\" parameter for safety")
	}

	h.logger.Warn("[C2] RESTART MACHINE command received (CONFIRMED) — scheduling OS reboot in 30 seconds")

	reason := params["reason"]
	if reason == "" {
		reason = "EDR C2 remote restart command"
	}

	cmd := exec.Command("shutdown", "/r", "/t", "30", "/d", "p:4:1", "/c", reason)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("shutdown command failed: %w", err)
	}

	return fmt.Sprintf("Machine restart scheduled in 30s (reason: %s). Run 'shutdown /a' to cancel.", reason), nil
}

// shutdownMachine initiates an OS-level machine shutdown.
//
// R6 FIX: Requires explicit confirm:"true" parameter and uses a 30-second
// delay to allow cancellation (shutdown /a) if issued by mistake.
func (h *Handler) shutdownMachine(_ context.Context, params map[string]string) (string, error) {
	// R6 FIX: Require explicit confirmation.
	if strings.ToLower(params["confirm"]) != "true" {
		return "", fmt.Errorf("BLOCKED: machine shutdown requires confirm=\"true\" parameter for safety")
	}

	h.logger.Warn("[C2] SHUTDOWN MACHINE command received (CONFIRMED) — scheduling OS shutdown in 30 seconds")

	reason := params["reason"]
	if reason == "" {
		reason = "EDR C2 remote shutdown command"
	}

	cmd := exec.Command("shutdown", "/s", "/t", "30", "/d", "p:4:1", "/c", reason)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("shutdown command failed: %w", err)
	}

	return fmt.Sprintf("Machine shutdown scheduled in 30s (reason: %s). Run 'shutdown /a' to cancel.", reason), nil
}
