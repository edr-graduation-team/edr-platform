package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/repository"
)

// CISA Known Exploited Vulnerabilities catalog (public, no auth).
const cisaKEVURL = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"

// KEVSyncService periodically fetches the CISA KEV catalog and propagates flags onto findings.
type KEVSyncService struct {
	repo     repository.VulnerabilityRepository
	logger   *logrus.Logger
	client   *http.Client
	interval time.Duration
	url      string
}

// NewKEVSyncService creates a service with sensible defaults (24h interval).
func NewKEVSyncService(repo repository.VulnerabilityRepository, logger *logrus.Logger) *KEVSyncService {
	return &KEVSyncService{
		repo:     repo,
		logger:   logger,
		client:   &http.Client{Timeout: 60 * time.Second},
		interval: 24 * time.Hour,
		url:      cisaKEVURL,
	}
}

// kevFeed mirrors the CISA JSON shape we care about.
type kevFeed struct {
	Title           string         `json:"title"`
	CatalogVersion  string         `json:"catalogVersion"`
	DateReleased    string         `json:"dateReleased"`
	Count           int            `json:"count"`
	Vulnerabilities []kevFeedEntry `json:"vulnerabilities"`
}

type kevFeedEntry struct {
	CVEID                      string `json:"cveID"`
	VendorProject              string `json:"vendorProject"`
	Product                    string `json:"product"`
	VulnerabilityName          string `json:"vulnerabilityName"`
	DateAdded                  string `json:"dateAdded"`
	ShortDescription           string `json:"shortDescription"`
	RequiredAction             string `json:"requiredAction"`
	DueDate                    string `json:"dueDate"`
	KnownRansomwareCampaignUse string `json:"knownRansomwareCampaignUse"`
	Notes                      string `json:"notes"`
}

func parseKEVDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// SyncOnce performs a single fetch + apply cycle.
func (s *KEVSyncService) SyncOnce(ctx context.Context) (catalogSize int, findingsUpdated int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "edr-platform/kev-sync")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch KEV: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, 0, fmt.Errorf("KEV fetch HTTP %d: %s", resp.StatusCode, string(body))
	}

	var feed kevFeed
	if err := json.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return 0, 0, fmt.Errorf("decode KEV JSON: %w", err)
	}

	kevMap := make(map[string]repository.KEVEntry, len(feed.Vulnerabilities))
	for _, v := range feed.Vulnerabilities {
		cve := strings.ToUpper(strings.TrimSpace(v.CVEID))
		if cve == "" {
			continue
		}
		kevMap[cve] = repository.KEVEntry{
			CVE:               cve,
			VendorProject:     v.VendorProject,
			Product:           v.Product,
			VulnerabilityName: v.VulnerabilityName,
			DateAdded:         parseKEVDate(v.DateAdded),
			ShortDescription:  v.ShortDescription,
			RequiredAction:    v.RequiredAction,
			DueDate:           parseKEVDate(v.DueDate),
			KnownRansomware:   v.KnownRansomwareCampaignUse,
			Notes:             v.Notes,
		}
	}

	updated, err := s.repo.MarkKEVForCVEs(ctx, kevMap)
	if err != nil {
		return len(kevMap), 0, fmt.Errorf("apply KEV: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"catalog_size":     len(kevMap),
		"findings_updated": updated,
		"catalog_version":  feed.CatalogVersion,
	}).Info("CISA KEV catalog synchronized")

	return len(kevMap), updated, nil
}

// Run starts the periodic sync loop. Cancel ctx to stop.
func (s *KEVSyncService) Run(ctx context.Context) {
	// Run once at startup (after a short delay to let DB migrations settle).
	go func() {
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			return
		}
		if _, _, err := s.SyncOnce(ctx); err != nil {
			s.logger.WithError(err).Warn("Initial KEV sync failed; will retry on schedule")
		}
	}()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, _, err := s.SyncOnce(ctx); err != nil {
				s.logger.WithError(err).Warn("Scheduled KEV sync failed")
			}
		}
	}
}
