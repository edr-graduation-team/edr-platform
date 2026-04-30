//go:build windows
// +build windows

package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	out, err := cmd.Output()
	if err != nil {
		c.logger.Warnf("[VulnScan] scanner execution failed (%s): %v", c.cfg.VulnScannerType, err)
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
	if bin == "" {
		bin = st
	}
	if len(c.cfg.VulnScanArgs) > 0 {
		return bin, c.cfg.VulnScanArgs
	}
	// Safe defaults: scan filesystem root and emit JSON.
	if st == "grype" {
		return bin, []string{"dir:C:\\", "-o", "json"}
	}
	return bin, []string{"fs", "C:\\", "--format", "json", "--quiet"}
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

