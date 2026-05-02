package signatures

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/edr-platform/win-agent/internal/logging"
)

// serverFeedLine is the JSON shape returned by the server NDJSON endpoint.
type serverFeedLine struct {
	SHA256   string `json:"sha256"`
	Name     string `json:"name"`
	Family   string `json:"family"`
	Severity string `json:"severity"`
	Source   string `json:"source"`
	Version  int64  `json:"version"`
}

// FetchAndMergeServerNDJSON pulls the delta NDJSON feed from the server and
// merges new hashes into the local store. Returns inserted, skipped counts and
// the highest version seen in the response.
func FetchAndMergeServerNDJSON(ctx context.Context, client *http.Client, store *Store, feedURL string, sinceVersion int64) (inserted, skipped int, maxVersion int64, err error) {
	u, err := url.Parse(feedURL)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("server_sync: parse url: %w", err)
	}

	q := u.Query()
	q.Set("since_version", strconv.FormatInt(sinceVersion, 10))
	q.Set("limit", "50000")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("server_sync: build request: %w", err)
	}
	req.Header.Set("User-Agent", "EDR-Platform-Agent/1.0 (server-sig-sync)")

	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("server_sync: HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, 0, 0, fmt.Errorf("server_sync: HTTP %d: %s", resp.StatusCode, string(body))
	}

	dec := json.NewDecoder(io.LimitReader(resp.Body, 128<<20))
	maxVersion = sinceVersion

	// Buffer all lines into a single NDJSON payload (stripped of the version field)
	// so we can call MergeFromNDJSON once (one bulk bbolt transaction).
	var buf []byte
	for dec.More() {
		var line serverFeedLine
		if decErr := dec.Decode(&line); decErr != nil {
			break
		}
		if len(line.SHA256) != 64 {
			continue
		}
		if line.Version > maxVersion {
			maxVersion = line.Version
		}
		row, _ := json.Marshal(map[string]string{
			"sha256":   line.SHA256,
			"name":     line.Name,
			"family":   line.Family,
			"severity": line.Severity,
			"source":   line.Source,
		})
		buf = append(buf, row...)
		buf = append(buf, '\n')
	}

	if len(buf) > 0 {
		inserted, skipped, err = store.MergeFromNDJSON(&bytesReader{data: buf}, false)
	}
	return inserted, skipped, maxVersion, err
}

// StartServerFeedAutoFetch periodically checks whether the server has newer hashes
// and downloads only the delta. Runs until ctx is cancelled.
// Call as: go StartServerFeedAutoFetch(ctx, store, logger, feedURL, interval)
func StartServerFeedAutoFetch(ctx context.Context, store *Store, logger *logging.Logger, feedURL string, interval time.Duration) {
	if store == nil || logger == nil || interval <= 0 || feedURL == "" {
		return
	}

	client := &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}

	// Derive the version-check URL from the feed URL (replace feed.ndjson → version).
	versionURL := deriveVersionURL(feedURL)

	run := func() {
		localVer, err := store.GetServerVersion()
		if err != nil {
			logger.Warnf("[sigfeed-server] GetServerVersion: %v", err)
			return
		}

		// Quick HEAD check: is there anything new?
		if versionURL != "" {
			remoteVer, err := fetchRemoteVersion(ctx, client, versionURL)
			if err != nil {
				logger.Warnf("[sigfeed-server] version check failed: %v", err)
			} else if remoteVer <= localVer {
				logger.Debugf("[sigfeed-server] up-to-date (local=%d remote=%d)", localVer, remoteVer)
				return
			}
		}

		subCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		ins, skip, maxVer, err := FetchAndMergeServerNDJSON(subCtx, client, store, feedURL, localVer)
		if err != nil {
			logger.Warnf("[sigfeed-server] merge failed: %v", err)
			return
		}
		if maxVer > localVer {
			if setErr := store.SetServerVersion(maxVer); setErr != nil {
				logger.Warnf("[sigfeed-server] SetServerVersion: %v", setErr)
			}
		}
		n, _ := store.Version()
		logger.Infof("[sigfeed-server] OK: inserted=%d skipped=%d new_server_version=%d db_entries=%d", ins, skip, maxVer, n)
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

// fetchRemoteVersion calls GET <versionURL> and returns the max_version field.
func fetchRemoteVersion(ctx context.Context, client *http.Client, versionURL string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var body struct {
		MaxVersion int64 `json:"max_version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err != nil {
		return 0, err
	}
	return body.MaxVersion, nil
}

// deriveVersionURL converts ".../feed.ndjson" → ".../version".
func deriveVersionURL(feedURL string) string {
	u, err := url.Parse(feedURL)
	if err != nil {
		return ""
	}
	p := u.Path
	const suffix = "feed.ndjson" // len == 11
	if len(p) > len(suffix) && p[len(p)-len(suffix):] == suffix {
		u.Path = p[:len(p)-len(suffix)] + "version"
		u.RawQuery = ""
		return u.String()
	}
	return ""
}

// newSingleLineReader builds a minimal NDJSON reader for one hash record.
func newSingleLineReader(sha256, name, family, severity, source string) io.Reader {
	line, _ := json.Marshal(map[string]string{
		"sha256":   sha256,
		"name":     name,
		"family":   family,
		"severity": severity,
		"source":   source,
	})
	return newBytesReader(append(line, '\n'))
}

// newBytesReader wraps a byte slice in an io.Reader.
type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(b []byte) io.Reader {
	return &bytesReader{data: b}
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
