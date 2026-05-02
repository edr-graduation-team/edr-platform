package service

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// MalwareBazaarRecentCSVURL is the public MalwareBazaar "recent" CSV export.
const MalwareBazaarRecentCSVURL = "https://bazaar.abuse.ch/export/csv/recent/"

// SignatureSyncService fetches MalwareBazaar CSV periodically and stores new hashes in PostgreSQL.
type SignatureSyncService struct {
	repo     repository.MalwareHashRepository
	logger   *logrus.Logger
	client   *http.Client
	interval time.Duration
	url      string
	triggerCh chan struct{}
}

// NewSignatureSyncService creates a sync service with a 6-hour default interval.
func NewSignatureSyncService(repo repository.MalwareHashRepository, logger *logrus.Logger) *SignatureSyncService {
	return &SignatureSyncService{
		repo:      repo,
		logger:    logger,
		client:    &http.Client{Timeout: 3 * time.Minute},
		interval:  6 * time.Hour,
		url:       MalwareBazaarRecentCSVURL,
		triggerCh: make(chan struct{}, 1),
	}
}

// TriggerNow queues an immediate sync (non-blocking; dropped if one is already queued).
func (s *SignatureSyncService) TriggerNow() {
	select {
	case s.triggerCh <- struct{}{}:
	default:
	}
}

// SyncOnce downloads the CSV and inserts new hashes. Returns inserted count.
func (s *SignatureSyncService) SyncOnce(ctx context.Context) (inserted int64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "edr-platform/signature-sync")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch MalwareBazaar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var reader io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Type"), "gzip") ||
		strings.HasSuffix(s.url, ".gz") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return 0, fmt.Errorf("gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	hashes, err := parseCSV(reader)
	if err != nil {
		return 0, fmt.Errorf("parse CSV: %w", err)
	}
	if len(hashes) == 0 {
		return 0, nil
	}

	inserted, err = s.repo.InsertMany(ctx, hashes)
	if err != nil {
		return 0, fmt.Errorf("insert hashes: %w", err)
	}

	total, _ := s.repo.Count(ctx)
	maxVer, _ := s.repo.GetMaxVersion(ctx)
	s.logger.WithFields(logrus.Fields{
		"inserted":    inserted,
		"parsed":      len(hashes),
		"db_total":    total,
		"max_version": maxVer,
	}).Info("[sigfeed] MalwareBazaar sync complete")

	return inserted, nil
}

// Run starts the periodic sync loop. Cancel ctx to stop.
func (s *SignatureSyncService) Run(ctx context.Context) {
	go func() {
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			return
		}
		if _, err := s.SyncOnce(ctx); err != nil {
			s.logger.WithError(err).Warn("[sigfeed] Initial MalwareBazaar sync failed; will retry on schedule")
		}
	}()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.triggerCh:
			if _, err := s.SyncOnce(ctx); err != nil {
				s.logger.WithError(err).Warn("[sigfeed] Manual trigger sync failed")
			}
		case <-ticker.C:
			if _, err := s.SyncOnce(ctx); err != nil {
				s.logger.WithError(err).Warn("[sigfeed] Scheduled MalwareBazaar sync failed")
			}
		}
	}
}

// parseCSV extracts SHA-256 hashes + metadata from a MalwareBazaar CSV.
// Lines starting with '#' are comments. The expected column order is:
//
//	first_seen,sha256_hash,md5_hash,sha1_hash,reporter,file_name,file_type_guess,
//	mime_type,signature,clamav,vtpercent,imphash,ssdeep,tlsh
func parseCSV(r io.Reader) ([]*models.MalwareHash, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	const sha256Len = 64

	var out []*models.MalwareHash
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, ",")
		if len(cols) < 2 {
			continue
		}
		sha := strings.ToLower(strings.Trim(strings.TrimSpace(cols[1]), `"`))
		if len(sha) != sha256Len {
			continue
		}

		h := &models.MalwareHash{
			SHA256: sha,
			Source: "malwarebazaar",
		}
		if len(cols) > 6 {
			h.Family = strings.Trim(strings.TrimSpace(cols[6]), `"`)
		}
		if len(cols) > 5 {
			h.Name = strings.Trim(strings.TrimSpace(cols[5]), `"`)
		}
		out = append(out, h)
	}
	return out, sc.Err()
}
