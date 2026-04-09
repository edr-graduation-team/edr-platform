// Package enrollment provides embedded CA certificate support for secure zero-touch provisioning.
package enrollment

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "embed"
)

// EmbeddedCACert holds the CA certificate PEM data, injected at build time
// via go:embed. When building through the dashboard, the build process copies
// the real ca.crt into this package directory before compilation.
// If the placeholder is empty (local dev builds), the agent falls back to
// the legacy HTTP fetch.
//
//go:embed ca-chain.crt
var EmbeddedCACert []byte

// HasEmbeddedCA reports whether a valid CA certificate was embedded at build time.
func HasEmbeddedCA() bool {
	return len(EmbeddedCACert) > 0 &&
		strings.Contains(string(EmbeddedCACert), "-----BEGIN CERTIFICATE-----")
}

// WriteEmbeddedCA writes the embedded CA certificate to caPath on disk.
// If caPath already exists, the file is overwritten to ensure the latest
// embedded cert is always used. Directory creation is handled automatically.
func WriteEmbeddedCA(caPath string) error {
	if !HasEmbeddedCA() {
		return fmt.Errorf("no valid CA certificate embedded in this build")
	}

	// Ensure directory exists
	dir := filepath.Dir(caPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create cert directory %s: %w", dir, err)
	}

	// Write with restrictive permissions (SYSTEM/admin only)
	if err := os.WriteFile(caPath, EmbeddedCACert, 0600); err != nil {
		return fmt.Errorf("failed to write embedded CA certificate to %s: %w", caPath, err)
	}

	return nil
}
