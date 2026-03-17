// Package installer provides unit tests for the installer logic.
// Tests run entirely in temp directories and do NOT require Administrator privileges
// or a real Windows Service Control Manager. SCM-dependent paths are tested separately.
//
//go:build windows
// +build windows

package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edr-platform/win-agent/internal/config"
	"gopkg.in/yaml.v3"
)

// =============================================================================
// PatchHostsFile tests (uses temp files, no admin required)
// =============================================================================

func patchHostsFileFromPath(path, serverIP, serverDomain string) error {
	// Temporarily redirect the package-level hostsFile constant is not possible
	// in Go, so we duplicate the logic here pointing at our temp path.
	// This tests the algorithm independently.
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		stripped := strings.TrimSpace(line)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		fields := strings.Fields(stripped)
		if len(fields) >= 2 && fields[0] == serverIP {
			for _, f := range fields[1:] {
				if strings.EqualFold(f, serverDomain) {
					return nil // idempotent — already present
				}
			}
		}
	}

	content := string(data)
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content += "\n"
	}
	content += serverIP + "\t" + serverDomain + "\t" + hostsComment + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}

func TestPatchHostsFile_AppendNew(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "hosts")
	initial := "127.0.0.1\tlocalhost\n"
	if err := os.WriteFile(tmpFile, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	if err := patchHostsFileFromPath(tmpFile, "192.168.1.10", "edr.internal"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(tmpFile)
	content := string(data)

	if !strings.Contains(content, "192.168.1.10") {
		t.Error("expected IP to be appended")
	}
	if !strings.Contains(content, "edr.internal") {
		t.Error("expected domain to be appended")
	}
	if !strings.Contains(content, hostsComment) {
		t.Error("expected EDR comment marker")
	}
}

func TestPatchHostsFile_Idempotent(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "hosts")
	existing := "127.0.0.1\tlocalhost\n192.168.1.10\tedr.internal\t# EDR C2\n"
	if err := os.WriteFile(tmpFile, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := patchHostsFileFromPath(tmpFile, "192.168.1.10", "edr.internal"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(tmpFile)
	// Count occurrences: should still be exactly 1.
	count := strings.Count(string(data), "192.168.1.10\tedr.internal")
	if count != 1 {
		t.Errorf("expected exactly 1 entry, got %d", count)
	}
}

func TestPatchHostsFile_CommentedEntryIsNotIdempotent(t *testing.T) {
	// A commented-out entry should NOT prevent a new active entry from being added.
	tmpFile := filepath.Join(t.TempDir(), "hosts")
	existing := "127.0.0.1\tlocalhost\n# 192.168.1.10\tedr.internal\n"
	if err := os.WriteFile(tmpFile, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := patchHostsFileFromPath(tmpFile, "192.168.1.10", "edr.internal"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(tmpFile)
	// Should now have an uncommented entry.
	lines := strings.Split(string(data), "\n")
	found := false
	for _, l := range lines {
		stripped := strings.TrimSpace(l)
		if !strings.HasPrefix(stripped, "#") && strings.Contains(stripped, "192.168.1.10") && strings.Contains(stripped, "edr.internal") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected an active (non-comment) hosts entry to be added")
	}
}

func TestPatchHostsFile_NoTrailingNewlineSafe(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "hosts")
	// File with no trailing newline.
	if err := os.WriteFile(tmpFile, []byte("127.0.0.1\tlocalhost"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := patchHostsFileFromPath(tmpFile, "10.0.0.1", "c2.edr"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(tmpFile)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "10.0.0.1") {
		t.Errorf("last line should contain the new entry, got: %q", last)
	}
}

// =============================================================================
// GenerateConfig tests (uses temp directory, no admin required)
// =============================================================================

func TestGenerateConfig_FieldsCorrect(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	opts := Options{
		ServerDomain: "edr.internal",
		ServerPort:   "50051",
		Token:        "test-token-abc123",
		ConfigPath:   cfgPath,
	}

	if err := GenerateConfig(opts); err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}

	// Load the generated file and verify fields.
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse generated config: %v", err)
	}

	if cfg.Server.Address != "edr.internal:50051" {
		t.Errorf("expected server address 'edr.internal:50051', got %q", cfg.Server.Address)
	}
	if cfg.Certs.BootstrapToken != "test-token-abc123" {
		t.Errorf("expected bootstrap token 'test-token-abc123', got %q", cfg.Certs.BootstrapToken)
	}
	if len(cfg.Agent.ID) != 36 {
		t.Errorf("expected UUID (36 chars), got %q (len=%d)", cfg.Agent.ID, len(cfg.Agent.ID))
	}
}

func TestGenerateConfig_DefaultPortFallback(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	opts := Options{
		ServerDomain: "c2.company.com",
		ServerPort:   "", // empty — should default to 50051
		Token:        "tok",
		ConfigPath:   cfgPath,
	}

	if err := GenerateConfig(opts); err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	var cfg config.Config
	yaml.Unmarshal(data, &cfg)

	if !strings.HasSuffix(cfg.Server.Address, ":50051") {
		t.Errorf("expected default port 50051, got %q", cfg.Server.Address)
	}
}

func TestGenerateConfig_UniqueIDPerCall(t *testing.T) {
	tmpDir := t.TempDir()
	opts := Options{
		ServerDomain: "test.edr",
		ServerPort:   "50051",
		Token:        "tok",
	}

	ids := make(map[string]bool)
	for i := 0; i < 5; i++ {
		opts.ConfigPath = filepath.Join(tmpDir, "config"+string(rune('0'+i))+".yaml")
		if err := GenerateConfig(opts); err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		data, _ := os.ReadFile(opts.ConfigPath)
		var cfg config.Config
		yaml.Unmarshal(data, &cfg)
		if ids[cfg.Agent.ID] {
			t.Errorf("duplicate agent ID generated: %s", cfg.Agent.ID)
		}
		ids[cfg.Agent.ID] = true
	}
}

func TestGenerateConfig_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "deep", "nested", "config")
	cfgPath := filepath.Join(nested, "config.yaml")

	opts := Options{
		ServerDomain: "test.edr",
		ServerPort:   "50051",
		Token:        "tok",
		ConfigPath:   cfgPath,
	}

	if err := GenerateConfig(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Error("config file was not created in nested directory")
	}
}
