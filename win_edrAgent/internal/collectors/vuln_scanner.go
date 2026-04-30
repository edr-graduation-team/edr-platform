//go:build windows
// +build windows

package collectors

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

type VulnerabilityScannerCollector struct {
	cfg      config.CollectorConfig
	eventChan chan<- *event.Event
	logger   *logging.Logger
}

func NewVulnerabilityScannerCollector(cfg config.CollectorConfig, eventChan chan<- *event.Event, logger *logging.Logger) *VulnerabilityScannerCollector {
	return &VulnerabilityScannerCollector{
		cfg:       cfg,
		eventChan: eventChan,
		logger:    logger,
	}
}

func (c *VulnerabilityScannerCollector) Start(ctx context.Context) {
	if !c.cfg.VulnScanEnabled {
		return
	}
	c.runScan(ctx)
	ticker := time.NewTicker(c.cfg.VulnScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.runScan(ctx)
		}
	}
}

func (c *VulnerabilityScannerCollector) runScan(ctx context.Context) {
	cmdName, args := c.buildCommand()
	if cmdName == "" {
		c.logger.Warn("[VulnScan] scanner command not configured")
		return
	}
	scanCtx, cancel := context.WithTimeout(ctx, c.cfg.VulnScanTimeout)
	defer cancel()
	cmd := exec.CommandContext(scanCtx, cmdName, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if c.shouldAutoInstallTrivy(cmdName, err) {
			if bin, ierr := c.ensureTrivyInstalled(scanCtx); ierr == nil && strings.TrimSpace(bin) != "" {
				c.logger.Infof("[VulnScan] trivy installed at %s; retrying scan", bin)
				cmdName = bin
				cmd = exec.CommandContext(scanCtx, cmdName, args...)
				out, err = cmd.CombinedOutput()
			} else if ierr != nil {
				c.logger.Warnf("[VulnScan] auto-install trivy failed: %v", ierr)
			}
		}
	}
	if err != nil && strings.EqualFold(strings.TrimSpace(c.cfg.VulnScannerType), "trivy") {
		// First run commonly fails because DB/cache is not initialized yet.
		if ierr := c.ensureTrivyDB(scanCtx, cmdName); ierr == nil {
			c.logger.Info("[VulnScan] trivy DB/cache initialized; retrying scan")
			cmd = exec.CommandContext(scanCtx, cmdName, args...)
			out, err = cmd.CombinedOutput()
		} else {
			c.logger.Warnf("[VulnScan] trivy DB init failed: %v", ierr)
		}
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 600 {
			msg = msg[:600] + "..."
		}
		if msg == "" {
			c.logger.Warnf("[VulnScan] scanner execution failed (%s): %v", c.cfg.VulnScannerType, err)
		} else {
			c.logger.Warnf("[VulnScan] scanner execution failed (%s): %v | output=%s", c.cfg.VulnScannerType, err, msg)
		}
		return
	}
	var findings []vulnFinding
	switch strings.ToLower(strings.TrimSpace(c.cfg.VulnScannerType)) {
	case "trivy":
		findings, err = parseTrivyFindings(out)
	case "grype":
		findings, err = parseGrypeFindings(out)
	default:
		err = fmt.Errorf("unsupported scanner type %q", c.cfg.VulnScannerType)
	}
	if err != nil {
		c.logger.Warnf("[VulnScan] parse failed: %v", err)
		return
	}
	emitted := 0
	for _, f := range findings {
		if strings.TrimSpace(f.CVE) == "" {
			continue
		}
		evt := event.NewEvent(event.EventTypeVulnerability, toSeverity(f.Severity), map[string]interface{}{
			"cve":               strings.ToUpper(strings.TrimSpace(f.CVE)),
			"title":             strings.TrimSpace(f.Title),
			"description":       strings.TrimSpace(f.Description),
			"severity":          strings.ToLower(strings.TrimSpace(f.Severity)),
			"cvss":              f.CVSS,
			"source":            strings.ToLower(strings.TrimSpace(c.cfg.VulnScannerType)),
			"package_name":      strings.TrimSpace(f.PackageName),
			"installed_version": strings.TrimSpace(f.InstalledVersion),
			"fixed_version":     strings.TrimSpace(f.FixedVersion),
			"reference_url":     strings.TrimSpace(f.ReferenceURL),
			"published_at":      strings.TrimSpace(f.PublishedAt),
		})
		select {
		case c.eventChan <- evt:
			emitted++
		default:
			c.logger.Warn("[VulnScan] event channel full, dropping finding")
		}
	}
	c.logger.Infof("[VulnScan] scan complete: scanner=%s findings=%d emitted=%d", c.cfg.VulnScannerType, len(findings), emitted)
}

func (c *VulnerabilityScannerCollector) buildCommand() (string, []string) {
	st := strings.ToLower(strings.TrimSpace(c.cfg.VulnScannerType))
	bin := strings.TrimSpace(c.cfg.VulnScannerPath)
	if bin == "" && st == "trivy" {
		managed := trivyExePath()
		if fileExists(managed) {
			bin = managed
		}
	}
	if bin == "" {
		bin = st
	}
	if len(c.cfg.VulnScanArgs) > 0 {
		return bin, c.cfg.VulnScanArgs
	}
	// Safe defaults: scan filesystem root and emit JSON.
	if st == "grype" {
		// Focus on installed application directories by default (not full disk).
		return bin, []string{"dir:C:\\Program Files", "dir:C:\\Program Files (x86)", "-o", "json"}
	}
	return bin, []string{
		"fs", "C:\\Program Files", "C:\\Program Files (x86)",
		"--format", "json",
		"--quiet",
		"--cache-dir", trivyCacheDir(),
		// Windows hosts often contain locked system files that cause Trivy fs scans
		// to exit with status=1. Skip them so scanning remains resilient.
		"--skip-files", `C:\DumpStack.log.tmp`,
		"--skip-files", `C:\pagefile.sys`,
		"--skip-files", `C:\hiberfil.sys`,
		"--skip-files", `C:\swapfile.sys`,
	}
}

type vulnFinding struct {
	CVE              string
	Title            string
	Description      string
	Severity         string
	CVSS             float64
	PackageName      string
	InstalledVersion string
	FixedVersion     string
	ReferenceURL     string
	PublishedAt      string
}

type trivyReport struct {
	Results []struct {
		Vulnerabilities []struct {
			ID               string `json:"VulnerabilityID"`
			Title            string `json:"Title"`
			Description      string `json:"Description"`
			Severity         string `json:"Severity"`
			PkgName          string `json:"PkgName"`
			InstalledVersion string `json:"InstalledVersion"`
			FixedVersion     string `json:"FixedVersion"`
			PublishedDate    string `json:"PublishedDate"`
			References       []string `json:"References"`
			CVSS             map[string]struct {
				V3Score float64 `json:"V3Score"`
			} `json:"CVSS"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

func parseTrivyFindings(raw []byte) ([]vulnFinding, error) {
	var r trivyReport
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	out := make([]vulnFinding, 0, 256)
	for _, res := range r.Results {
		for _, v := range res.Vulnerabilities {
			max := 0.0
			for _, c := range v.CVSS {
				if c.V3Score > max {
					max = c.V3Score
				}
			}
			ref := ""
			if len(v.References) > 0 {
				ref = v.References[0]
			}
			out = append(out, vulnFinding{
				CVE:              v.ID,
				Title:            v.Title,
				Description:      v.Description,
				Severity:         v.Severity,
				CVSS:             max,
				PackageName:      v.PkgName,
				InstalledVersion: v.InstalledVersion,
				FixedVersion:     v.FixedVersion,
				ReferenceURL:     ref,
				PublishedAt:      v.PublishedDate,
			})
		}
	}
	return out, nil
}

type grypeReport struct {
	Matches []struct {
		Vulnerability struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Severity    string `json:"severity"`
			DataSource  string `json:"dataSource"`
			CVSS        []struct {
				Metrics struct {
					BaseScore float64 `json:"baseScore"`
				} `json:"metrics"`
			} `json:"cvss"`
		} `json:"vulnerability"`
		Artifact struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"artifact"`
		Fix struct {
			Versions []string `json:"versions"`
		} `json:"fix"`
	} `json:"matches"`
}

func parseGrypeFindings(raw []byte) ([]vulnFinding, error) {
	var r grypeReport
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	out := make([]vulnFinding, 0, len(r.Matches))
	for _, m := range r.Matches {
		max := 0.0
		for _, c := range m.Vulnerability.CVSS {
			if c.Metrics.BaseScore > max {
				max = c.Metrics.BaseScore
			}
		}
		fixed := ""
		if len(m.Fix.Versions) > 0 {
			fixed = m.Fix.Versions[0]
		}
		out = append(out, vulnFinding{
			CVE:              m.Vulnerability.ID,
			Title:            m.Vulnerability.ID,
			Description:      m.Vulnerability.Description,
			Severity:         m.Vulnerability.Severity,
			CVSS:             max,
			PackageName:      m.Artifact.Name,
			InstalledVersion: m.Artifact.Version,
			FixedVersion:     fixed,
			ReferenceURL:     m.Vulnerability.DataSource,
		})
	}
	return out, nil
}

func toSeverity(s string) event.Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return event.SeverityCritical
	case "high":
		return event.SeverityHigh
	case "medium":
		return event.SeverityMedium
	case "low":
		return event.SeverityLow
	default:
		return event.SeverityMedium
	}
}

func (c *VulnerabilityScannerCollector) shouldAutoInstallTrivy(cmdName string, err error) bool {
	if strings.ToLower(strings.TrimSpace(c.cfg.VulnScannerType)) != "trivy" {
		return false
	}
	if strings.TrimSpace(c.cfg.VulnScannerPath) != "" {
		// Operator explicitly pinned a binary path; do not override.
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "executable file not found") || strings.Contains(strings.ToLower(cmdName), "trivy")
}

func trivyToolDir() string {
	return `C:\ProgramData\EDR\tools\trivy`
}

func trivyExePath() string {
	return filepath.Join(trivyToolDir(), "trivy.exe")
}

func trivyCacheDir() string {
	return filepath.Join(trivyToolDir(), "cache")
}

func (c *VulnerabilityScannerCollector) ensureTrivyInstalled(ctx context.Context) (string, error) {
	if fileExists(trivyExePath()) {
		return trivyExePath(), nil
	}
	if err := os.MkdirAll(trivyToolDir(), 0755); err != nil {
		return "", fmt.Errorf("create trivy dir: %w", err)
	}
	zipURL, err := fetchLatestTrivyWindowsZipURL(ctx)
	if err != nil {
		return "", err
	}
	tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("trivy-%d.zip", time.Now().UnixNano()))
	defer os.Remove(tmpZip)
	if err := downloadToFile(ctx, zipURL, tmpZip, 120<<20); err != nil {
		return "", fmt.Errorf("download trivy zip: %w", err)
	}
	if err := extractZipFile(tmpZip, "trivy.exe", trivyExePath()); err != nil {
		return "", fmt.Errorf("extract trivy.exe: %w", err)
	}
	_ = os.MkdirAll(trivyCacheDir(), 0755)
	return trivyExePath(), nil
}

func (c *VulnerabilityScannerCollector) ensureTrivyDB(ctx context.Context, trivyBin string) error {
	if strings.TrimSpace(trivyBin) == "" {
		trivyBin = trivyExePath()
	}
	if err := os.MkdirAll(trivyCacheDir(), 0755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, trivyBin, "image", "--download-db-only", "--cache-dir", trivyCacheDir(), "--quiet")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 500 {
			msg = msg[:500] + "..."
		}
		if msg != "" {
			return fmt.Errorf("%v: %s", err, msg)
		}
		return err
	}
	return nil
}

func fetchLatestTrivyWindowsZipURL(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/aquasecurity/trivy/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github api status %d", resp.StatusCode)
	}
	var body struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&body); err != nil {
		return "", err
	}
	for _, a := range body.Assets {
		n := strings.ToLower(strings.TrimSpace(a.Name))
		if strings.Contains(n, "windows-64bit.zip") {
			return strings.TrimSpace(a.BrowserDownloadURL), nil
		}
	}
	return "", fmt.Errorf("windows-64bit trivy asset not found in latest release")
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

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
