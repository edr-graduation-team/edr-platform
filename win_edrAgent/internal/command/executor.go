// Package command provides detailed command execution actions.
package command

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/edr-platform/win-agent/internal/logging"
)

// Executor provides safe command execution with validation.
type Executor struct {
	logger        *logging.Logger
	quarantineDir string
	forensicsDir  string
	updateDir     string

	// Safety checks
	protectedPIDs  map[string]bool
	protectedPaths []string
}

// NewExecutor creates a new command executor.
func NewExecutor(logger *logging.Logger) *Executor {
	return &Executor{
		logger:        logger,
		quarantineDir: "C:\\ProgramData\\EDR\\quarantine",
		forensicsDir:  "C:\\ProgramData\\EDR\\forensics",
		updateDir:     "C:\\ProgramData\\EDR\\updates",
		protectedPIDs: map[string]bool{
			"0": true, "4": true, // System, System Idle
		},
		protectedPaths: []string{
			"C:\\Windows\\System32\\ntoskrnl.exe",
			"C:\\Windows\\System32\\smss.exe",
			"C:\\Windows\\System32\\csrss.exe",
			"C:\\Windows\\System32\\wininit.exe",
			"C:\\Windows\\System32\\services.exe",
			"C:\\Windows\\System32\\lsass.exe",
			"C:\\Windows\\System32\\svchost.exe",
		},
	}
}

// TerminateProcess kills a process by PID with safety checks.
func (e *Executor) TerminateProcess(ctx context.Context, pid string) (string, error) {
	// Validate PID
	pidInt, err := strconv.Atoi(pid)
	if err != nil || pidInt <= 0 {
		return "", fmt.Errorf("invalid PID: %s", pid)
	}

	// Check protected PIDs
	if e.protectedPIDs[pid] {
		return "", fmt.Errorf("cannot terminate protected system process (PID %s)", pid)
	}

	// Get process name for logging
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		fmt.Sprintf("(Get-Process -Id %s -ErrorAction SilentlyContinue).Name", pid))
	nameOutput, _ := cmd.Output()
	processName := strings.TrimSpace(string(nameOutput))

	// Check if it's a critical process
	if e.isCriticalProcess(processName) {
		return "", fmt.Errorf("cannot terminate critical process: %s", processName)
	}

	// Execute termination
	killCmd := exec.CommandContext(ctx, "taskkill", "/PID", pid, "/F", "/T")
	output, err := killCmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("failed to terminate process: %w", err)
	}

	e.logger.Infof("Process terminated: PID=%s Name=%s", pid, processName)
	return fmt.Sprintf("Process terminated: PID=%s Name=%s", pid, processName), nil
}

// isCriticalProcess checks if a process name is critical.
func (e *Executor) isCriticalProcess(name string) bool {
	criticalProcesses := []string{
		"csrss", "smss", "wininit", "services", "lsass",
		"svchost", "dwm", "winlogon", "System",
	}

	name = strings.ToLower(strings.TrimSuffix(name, ".exe"))
	for _, cp := range criticalProcesses {
		if name == strings.ToLower(cp) {
			return true
		}
	}
	return false
}

// QuarantineFile moves a file to quarantine with metadata.
func (e *Executor) QuarantineFile(ctx context.Context, filePath, reason string) (string, error) {
	// Validate file exists
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", filePath)
	}
	if info.IsDir() {
		return "", fmt.Errorf("cannot quarantine directory: %s", filePath)
	}

	// Check protected paths
	for _, protected := range e.protectedPaths {
		if strings.EqualFold(filePath, protected) {
			return "", fmt.Errorf("cannot quarantine protected system file: %s", filePath)
		}
	}

	// Create quarantine directory
	if err := os.MkdirAll(e.quarantineDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create quarantine dir: %w", err)
	}

	// Generate quarantine filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	baseName := filepath.Base(filePath)
	quarantineName := fmt.Sprintf("%s_%s.quarantine", timestamp, baseName)
	quarantinePath := filepath.Join(e.quarantineDir, quarantineName)

	// Create metadata file
	metadataPath := quarantinePath + ".meta"
	metadata := fmt.Sprintf("OriginalPath: %s\nQuarantineTime: %s\nReason: %s\nSize: %d\n",
		filePath, time.Now().Format(time.RFC3339), reason, info.Size())
	os.WriteFile(metadataPath, []byte(metadata), 0600)

	// Move file to quarantine
	if err := os.Rename(filePath, quarantinePath); err != nil {
		// If rename fails (cross-device), copy and delete
		if err := copyFile(filePath, quarantinePath); err != nil {
			return "", fmt.Errorf("failed to copy file: %w", err)
		}
		os.Remove(filePath)
	}

	e.logger.Infof("File quarantined: %s -> %s", filePath, quarantinePath)
	return fmt.Sprintf("File quarantined: %s", quarantinePath), nil
}

// IsolateNetwork disables all network adapters.
func (e *Executor) IsolateNetwork(ctx context.Context, whitelistIPs []string) (string, error) {
	// Store current adapter states for recovery
	stateFile := filepath.Join(e.quarantineDir, "network_state.json")

	// Get current adapter states
	listCmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		"Get-NetAdapter | Select-Object Name,Status | ConvertTo-Json")
	stateOutput, err := listCmd.Output()
	if err == nil {
		os.MkdirAll(filepath.Dir(stateFile), 0700)
		os.WriteFile(stateFile, stateOutput, 0600)
	}

	// Configure Windows Firewall to block outbound except whitelist
	if len(whitelistIPs) > 0 {
		// Create whitelist rule
		ips := strings.Join(whitelistIPs, ",")
		fwCmd := exec.CommandContext(ctx, "netsh", "advfirewall", "firewall", "add", "rule",
			"name=EDR_Isolation_Whitelist", "dir=out", "action=allow", "remoteip="+ips)
		fwCmd.Run()
	}

	// Block all outbound traffic
	blockCmd := exec.CommandContext(ctx, "netsh", "advfirewall", "set", "allprofiles",
		"firewallpolicy", "blockinbound,blockoutbound")
	output, err := blockCmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("failed to set firewall policy: %w", err)
	}

	e.logger.Warn("Network isolated - firewall blocking all traffic")
	return "Network isolated - all outbound traffic blocked", nil
}

// UnisolateNetwork restores network connectivity.
func (e *Executor) UnisolateNetwork(ctx context.Context) (string, error) {
	// Remove isolation rule
	exec.CommandContext(ctx, "netsh", "advfirewall", "firewall", "delete", "rule",
		"name=EDR_Isolation_Whitelist").Run()

	// Restore default policy
	restoreCmd := exec.CommandContext(ctx, "netsh", "advfirewall", "set", "allprofiles",
		"firewallpolicy", "blockinbound,allowoutbound")
	output, err := restoreCmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("failed to restore firewall policy: %w", err)
	}

	e.logger.Info("Network isolation removed - connectivity restored")
	return "Network connectivity restored", nil
}

// CollectForensics gathers evidence files and creates a zip archive.
func (e *Executor) CollectForensics(ctx context.Context, paths []string, outputName string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("no paths specified for collection")
	}

	// Create forensics directory
	if err := os.MkdirAll(e.forensicsDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create forensics dir: %w", err)
	}

	// Generate output filename
	timestamp := time.Now().Format("20060102_150405")
	if outputName == "" {
		outputName = "forensics"
	}
	zipPath := filepath.Join(e.forensicsDir, fmt.Sprintf("%s_%s.zip", outputName, timestamp))

	// Create zip file
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add files to zip
	collected := 0
	for _, path := range paths {
		filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			// Create zip entry
			relPath, _ := filepath.Rel(filepath.Dir(path), filePath)
			writer, err := zipWriter.Create(relPath)
			if err != nil {
				return nil
			}

			// Copy file content
			file, err := os.Open(filePath)
			if err != nil {
				return nil
			}
			defer file.Close()

			io.Copy(writer, file)
			collected++
			return nil
		})
	}

	e.logger.Infof("Forensics collected: %d files -> %s", collected, zipPath)
	return fmt.Sprintf("Collected %d files to %s", collected, zipPath), nil
}

// DownloadUpdate downloads a new agent version.
func (e *Executor) DownloadUpdate(ctx context.Context, url, expectedChecksum string) (string, error) {
	// Validate URL
	if !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("update URL must use HTTPS")
	}

	// Create update directory
	if err := os.MkdirAll(e.updateDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create update dir: %w", err)
	}

	// Download file
	updatePath := filepath.Join(e.updateDir, "agent_update.exe")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status: %d", resp.StatusCode)
	}

	out, err := os.Create(updatePath)
	if err != nil {
		return "", fmt.Errorf("failed to create update file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write update file: %w", err)
	}

	// TODO: Verify checksum before applying

	e.logger.Info("Update downloaded successfully")
	return fmt.Sprintf("Update downloaded to %s", updatePath), nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
