package signatures

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/edr-platform/win-agent/internal/logging"
)

// DefaultMalwareBazaarRecentURL is the public MalwareBazaar "recent" CSV export.
const DefaultMalwareBazaarRecentURL = "https://bazaar.abuse.ch/export/csv/recent/"

var allowedSignatureFetchHosts = map[string]struct{}{
	"bazaar.abuse.ch": {},
	"127.0.0.1":       {},
	"localhost":       {},
}

func publicFeedURLAllowed(u *url.URL) bool {
	if u.Scheme != "https" {
		if u.Scheme == "http" {
			h := strings.ToLower(u.Hostname())
			return h == "127.0.0.1" || h == "localhost"
		}
		return false
	}
	_, ok := allowedSignatureFetchHosts[strings.ToLower(u.Hostname())]
	return ok
}

// FetchAndMergeMalwareBazaarCSV downloads a CSV (typically MalwareBazaar recent) and merges into the store.
func FetchAndMergeMalwareBazaarCSV(ctx context.Context, client *http.Client, store *Store, feedURL string, force bool, hashLimit int) (inserted, skipped int, err error) {
	u, err := url.Parse(feedURL)
	if err != nil {
		return 0, 0, err
	}
	if !publicFeedURLAllowed(u) {
		return 0, 0, fmt.Errorf("signatures: URL host not allowed for public feed: %s (allowed: bazaar.abuse.ch or http localhost)", u.Host)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", "EDR-Platform-Agent/1.0 (signature-feed; https://github.com/edr-platform)")
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("signatures: feed HTTP %s", resp.Status)
	}
	body := io.LimitReader(resp.Body, 8<<20)
	return store.MergeFromMalwareBazaarCSV(body, force, hashLimit)
}

// StartPublicFeedAutoFetch periodically merges the public CSV feed until ctx is done.
// First merge runs immediately in the same goroutine (caller should use go StartPublicFeedAutoFetch(...)).
func StartPublicFeedAutoFetch(ctx context.Context, store *Store, logger *logging.Logger, feedURL string, interval time.Duration, hashLimit int, force bool) {
	if store == nil || logger == nil || interval <= 0 {
		return
	}
	if feedURL == "" {
		feedURL = DefaultMalwareBazaarRecentURL
	}
	if hashLimit <= 0 {
		hashLimit = 500
	}
	client := &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	run := func() {
		subCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		ins, skip, err := FetchAndMergeMalwareBazaarCSV(subCtx, client, store, feedURL, force, hashLimit)
		if err != nil {
			logger.Warnf("[sigfeed] public feed merge failed: %v", err)
			return
		}
		n, _ := store.Version()
		logger.Infof("[sigfeed] public feed OK: inserted=%d skipped=%d db_entries=%d url=%s", ins, skip, n, feedURL)
	}
	run()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
