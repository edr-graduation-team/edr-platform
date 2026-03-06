// Package enrollment provides CA certificate auto-bootstrap for zero-touch provisioning.
package enrollment

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edr-platform/win-agent/internal/logging"
)

const (
	caFetchTimeout    = 15 * time.Second
	caFetchMaxRetries = 3
	caFetchRetryDelay = 2 * time.Second
	caHTTPPort        = "8090"
	caEndpoint        = "/api/v1/agent/ca"
)

// FetchCACertificate downloads the CA certificate from the Connection Manager's
// REST API and saves it to caPath. This enables zero-touch provisioning — agents
// can bootstrap TLS trust without manually pre-distributing the CA file.
//
// serverAddr is the gRPC address (e.g. "192.168.129.1:50051"); the host is
// extracted and the HTTP port (8090) is used for the REST API call.
//
// If caPath already exists, this function is a no-op.
func FetchCACertificate(serverAddr, caPath string, logger *logging.Logger) error {
	// Skip if CA cert already exists
	if _, err := os.Stat(caPath); err == nil {
		logger.Debugf("CA certificate already exists at %s, skipping fetch", caPath)
		return nil
	}

	// Extract host from gRPC address (strip port if present)
	host := serverAddr
	if h, _, err := net.SplitHostPort(serverAddr); err == nil {
		host = h
	}

	url := fmt.Sprintf("http://%s:%s%s", host, caHTTPPort, caEndpoint)
	logger.Infof("Fetching CA certificate from %s", url)

	client := &http.Client{Timeout: caFetchTimeout}

	var lastErr error
	for attempt := 1; attempt <= caFetchMaxRetries; attempt++ {
		if attempt > 1 {
			logger.Infof("Retry %d/%d for CA certificate fetch...", attempt, caFetchMaxRetries)
			time.Sleep(caFetchRetryDelay * time.Duration(attempt-1))
		}

		resp, err := client.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			logger.Warnf("CA fetch attempt %d failed: %v", attempt, err)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			logger.Warnf("CA fetch attempt %d: HTTP %d", attempt, resp.StatusCode)
			continue
		}

		if readErr != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", readErr)
			logger.Warnf("CA fetch attempt %d: read error: %v", attempt, readErr)
			continue
		}

		// Basic PEM validation
		if !strings.Contains(string(body), "-----BEGIN CERTIFICATE-----") {
			lastErr = fmt.Errorf("response is not a valid PEM certificate")
			logger.Warn("CA fetch: response does not contain PEM certificate data")
			continue
		}

		// Ensure directory exists
		dir := filepath.Dir(caPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create cert directory %s: %w", dir, err)
		}

		// Save with restrictive permissions
		if err := os.WriteFile(caPath, body, 0600); err != nil {
			return fmt.Errorf("failed to write CA certificate to %s: %w", caPath, err)
		}

		logger.Infof("CA certificate saved to %s (%d bytes)", caPath, len(body))
		return nil
	}

	return fmt.Errorf("failed to fetch CA certificate after %d attempts: %w", caFetchMaxRetries, lastErr)
}
