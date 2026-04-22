package command

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	sysmonChannel = "Microsoft-Windows-Sysmon/Operational"
	sysmonZipURL  = "https://download.sysinternals.com/files/Sysmon.zip"
)

func sysmonToolDir() string {
	return `C:\ProgramData\EDR\tools\sysmon`
}

func sysmonExePath() string {
	return filepath.Join(sysmonToolDir(), "Sysmon64.exe")
}

func sysmonConfigPath() string {
	return filepath.Join(sysmonToolDir(), "sysmonconfig.xml")
}

func (h *Handler) enableSysmon(ctx context.Context, params map[string]string) (string, error) {
	if err := os.MkdirAll(sysmonToolDir(), 0755); err != nil {
		return "", fmt.Errorf("create sysmon dir: %w", err)
	}

	// Optional: download config XML (dashboard can pass a URL)
	if u := strings.TrimSpace(params["config_url"]); u != "" {
		if err := downloadToFile(ctx, u, sysmonConfigPath(), 5<<20); err != nil {
			return "", fmt.Errorf("download sysmon config: %w", err)
		}
	}

	installedBefore := isSysmonServiceInstalled(ctx)

	if !fileExists(sysmonExePath()) {
		if err := downloadAndExtractSysmon(ctx, sysmonExePath()); err != nil {
			return "", err
		}
	}

	// Install if missing; if present, we still ensure channel is enabled.
	installArgs := []string{"-accepteula", "-i"}
	if fileExists(sysmonConfigPath()) {
		installArgs = append(installArgs, sysmonConfigPath())
	}
	// Sysmon install is idempotent; if it's already installed this updates config when provided.
	out, err := execCombined(ctx, sysmonExePath(), installArgs...)
	if err != nil {
		return "", fmt.Errorf("sysmon install/config failed: %v: %s", err, trim(out, 400))
	}

	if err := setEventChannelEnabled(ctx, sysmonChannel, true); err != nil {
		return "", err
	}

	after := "already installed"
	if !installedBefore {
		after = "installed"
	}

	msg := fmt.Sprintf("Sysmon %s and channel enabled (%s).", after, sysmonChannel)
	if fileExists(sysmonConfigPath()) {
		sum, _ := sha256File(sysmonConfigPath())
		msg += fmt.Sprintf(" Config applied (sha256=%s).", sum)
	}
	return msg, nil
}

func (h *Handler) disableSysmon(ctx context.Context, _ map[string]string) (string, error) {
	_ = setEventChannelEnabled(ctx, sysmonChannel, false)

	// Uninstall if binary exists; if service exists but binary missing, this will be best-effort only.
	if fileExists(sysmonExePath()) {
		out, err := execCombined(ctx, sysmonExePath(), "-accepteula", "-u")
		if err != nil {
			return "", fmt.Errorf("sysmon uninstall failed: %v: %s", err, trim(out, 400))
		}
		return "Sysmon uninstalled and channel disabled.", nil
	}
	if isSysmonServiceInstalled(ctx) {
		return "Sysmon appears installed, but Sysmon64.exe is missing on disk; channel disabled only.", nil
	}
	return "Sysmon not installed; channel disabled (no-op).", nil
}

func isSysmonServiceInstalled(ctx context.Context) bool {
	// Sysmon service name is typically Sysmon64 (64-bit).
	cmd := exec.CommandContext(ctx, "sc", "query", "Sysmon64")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	s := strings.ToUpper(string(out))
	return strings.Contains(s, "SERVICE_NAME") && (strings.Contains(s, "RUNNING") || strings.Contains(s, "STOPPED"))
}

func setEventChannelEnabled(ctx context.Context, channel string, enabled bool) error {
	flag := "/e:false"
	if enabled {
		flag = "/e:true"
	}
	out, err := execCombined(ctx, "wevtutil", "sl", channel, flag)
	if err != nil {
		return fmt.Errorf("wevtutil set channel %q %v failed: %v: %s", channel, enabled, err, trim(out, 400))
	}
	return nil
}

func downloadAndExtractSysmon(ctx context.Context, targetExe string) error {
	tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("sysmon-%d.zip", time.Now().UnixNano()))
	defer os.Remove(tmpZip)

	if err := downloadToFile(ctx, sysmonZipURL, tmpZip, 40<<20); err != nil {
		return fmt.Errorf("download sysmon zip: %w", err)
	}
	if err := extractZipFile(tmpZip, "Sysmon64.exe", targetExe); err != nil {
		// Fallback: some zips may contain Sysmon.exe + Sysmon64.exe; we only need Sysmon64.exe.
		return fmt.Errorf("extract Sysmon64.exe: %w", err)
	}
	return nil
}

func extractZipFile(zipPath, wantName, outPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if !strings.EqualFold(filepath.Base(f.Name), wantName) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		w, err := os.Create(outPath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			w.Close()
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("file %q not found in zip", wantName)
}

func downloadToFile(ctx context.Context, url, path string, maxBytes int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	body := io.LimitReader(resp.Body, maxBytes)
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func execCombined(ctx context.Context, exe string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func sha256File(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

