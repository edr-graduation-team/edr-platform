package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/repository"
)

// VulnScannerIngestService parses Trivy/Grype reports into normalized findings.
type VulnScannerIngestService struct {
	logger *logrus.Logger
}

func NewVulnScannerIngestService(logger *logrus.Logger) *VulnScannerIngestService {
	return &VulnScannerIngestService{logger: logger}
}

func (s *VulnScannerIngestService) Parse(scannerType string, agentID uuid.UUID, raw json.RawMessage) ([]repository.VulnerabilityFindingInput, error) {
	switch strings.ToLower(strings.TrimSpace(scannerType)) {
	case "trivy":
		return s.parseTrivy(agentID, raw)
	case "grype":
		return s.parseGrype(agentID, raw)
	default:
		return nil, fmt.Errorf("unsupported scanner_type %q (supported: trivy, grype)", scannerType)
	}
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
			References       []string `json:"References"`
			PublishedDate    string `json:"PublishedDate"`
			CVSS             map[string]struct {
				V3Score float64 `json:"V3Score"`
			} `json:"CVSS"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

func (s *VulnScannerIngestService) parseTrivy(agentID uuid.UUID, raw json.RawMessage) ([]repository.VulnerabilityFindingInput, error) {
	var report trivyReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, fmt.Errorf("invalid trivy report JSON: %w", err)
	}
	out := make([]repository.VulnerabilityFindingInput, 0, 256)
	for _, res := range report.Results {
		for _, v := range res.Vulnerabilities {
			if strings.TrimSpace(v.ID) == "" {
				continue
			}
			var cvss *float64
			max := 0.0
			for _, c := range v.CVSS {
				if c.V3Score > max {
					max = c.V3Score
				}
			}
			if max > 0 {
				cvss = &max
			}
			var publishedAt *time.Time
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v.PublishedDate)); err == nil {
				publishedAt = &t
			}
			ref := ""
			if len(v.References) > 0 {
				ref = strings.TrimSpace(v.References[0])
			}
			out = append(out, repository.VulnerabilityFindingInput{
				AgentID:          agentID,
				CVE:              strings.ToUpper(strings.TrimSpace(v.ID)),
				Title:            strings.TrimSpace(v.Title),
				Description:      strings.TrimSpace(v.Description),
				Severity:         strings.ToLower(strings.TrimSpace(v.Severity)),
				CVSS:             cvss,
				Source:           "trivy",
				PackageName:      strings.TrimSpace(v.PkgName),
				InstalledVersion: strings.TrimSpace(v.InstalledVersion),
				FixedVersion:     strings.TrimSpace(v.FixedVersion),
				ReferenceURL:     ref,
				PublishedAt:      publishedAt,
			})
		}
	}
	s.logger.WithFields(logrus.Fields{
		"agent_id": agentID,
		"scanner":  "trivy",
		"parsed":   len(out),
	}).Info("Parsed scanner report")
	return out, nil
}

type grypeReport struct {
	Matches []struct {
		Vulnerability struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Severity    string `json:"severity"`
			CVSS        []struct {
				Metrics struct {
					BaseScore float64 `json:"baseScore"`
				} `json:"metrics"`
			} `json:"cvss"`
			DataSource string `json:"dataSource"`
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

func (s *VulnScannerIngestService) parseGrype(agentID uuid.UUID, raw json.RawMessage) ([]repository.VulnerabilityFindingInput, error) {
	var report grypeReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, fmt.Errorf("invalid grype report JSON: %w", err)
	}
	out := make([]repository.VulnerabilityFindingInput, 0, len(report.Matches))
	for _, m := range report.Matches {
		cve := strings.ToUpper(strings.TrimSpace(m.Vulnerability.ID))
		if cve == "" {
			continue
		}
		var cvss *float64
		max := 0.0
		for _, c := range m.Vulnerability.CVSS {
			if c.Metrics.BaseScore > max {
				max = c.Metrics.BaseScore
			}
		}
		if max > 0 {
			cvss = &max
		}
		fixed := ""
		if len(m.Fix.Versions) > 0 {
			fixed = strings.TrimSpace(m.Fix.Versions[0])
		}
		out = append(out, repository.VulnerabilityFindingInput{
			AgentID:          agentID,
			CVE:              cve,
			Title:            cve,
			Description:      strings.TrimSpace(m.Vulnerability.Description),
			Severity:         strings.ToLower(strings.TrimSpace(m.Vulnerability.Severity)),
			CVSS:             cvss,
			Source:           "grype",
			PackageName:      strings.TrimSpace(m.Artifact.Name),
			InstalledVersion: strings.TrimSpace(m.Artifact.Version),
			FixedVersion:     fixed,
			ReferenceURL:     strings.TrimSpace(m.Vulnerability.DataSource),
		})
	}
	s.logger.WithFields(logrus.Fields{
		"agent_id": agentID,
		"scanner":  "grype",
		"parsed":   len(out),
	}).Info("Parsed scanner report")
	return out, nil
}

