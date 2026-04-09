// Package main provides the Agent Builder HTTP service.
//
// This is a lightweight, single-purpose Go HTTP server that runs inside a
// Docker container with the Go toolchain available. It accepts build requests
// from the Connection Manager and cross-compiles the EDR agent for Windows
// with embedded CA certificate and configuration injected via ldflags.
//
// Architecture:
//
//	Dashboard → Connection Manager (REST API) → Agent Builder (this service)
//	                                                   ↓
//	                                           go build -ldflags ...
//	                                                   ↓
//	                                           edr-agent.exe (binary download)
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BuildRequest is the JSON body accepted by POST /build.
type BuildRequest struct {
	ServerIP     string `json:"server_ip"`
	ServerDomain string `json:"server_domain"`
	ServerPort   string `json:"server_port"`
	Token        string `json:"token"`
	SkipConfig   bool   `json:"skip_config"`
	CACertPEM    string `json:"ca_cert_pem"` // PEM-encoded CA certificate to embed
}

func main() {
	port := os.Getenv("BUILDER_PORT")
	if port == "" {
		port = "8090"
	}

	agentSrc := os.Getenv("AGENT_SRC_DIR")
	if agentSrc == "" {
		agentSrc = "/app/agent-src"
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	// Build endpoint
	mux.HandleFunc("/build", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req BuildRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body: " + err.Error()})
			return
		}

		log.Printf("[BUILD] Starting build: skip_config=%v, server=%s:%s",
			req.SkipConfig, req.ServerDomain, req.ServerPort)

		// ── Write CA certificate for go:embed ───────────────────────────────
		embedTarget := filepath.Join(agentSrc, "internal", "enrollment", "ca-chain.crt")
		if req.CACertPEM != "" {
			if err := os.WriteFile(embedTarget, []byte(req.CACertPEM), 0644); err != nil {
				log.Printf("[BUILD] Failed to write CA cert: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": "Failed to write CA certificate for embedding: " + err.Error(),
				})
				return
			}
			defer func() {
				// Restore empty placeholder after build
				_ = os.WriteFile(embedTarget, []byte(" "), 0644)
			}()
		}

		// ── Build ldflags ───────────────────────────────────────────────────
		ldflags := []string{
			"-w", "-s",
			fmt.Sprintf("-X main.Version=%s", "dashboard-build"),
			fmt.Sprintf("-X main.BuildTime=%s", time.Now().UTC().Format(time.RFC3339)),
		}

		if req.Token != "" {
			// SECURITY: Two embedded values, neither is plaintext:
			//
			// 1. EmbeddedTokenHash (SHA-256) — for uninstall verification.
			//    Irreversible; useless to an attacker even if extracted.
			//
			// 2. EmbeddedTokenObf (XOR-obfuscated hex) — for zero-touch enrollment.
			//    The token is XOR'd with a compile-time key so `strings binary`
			//    cannot reveal it. Decoded at runtime ONLY for the single CSR call,
			//    then zeroed from memory. This raises the bar significantly above
			//    plaintext embedding (the standard for commercial EDR agents).
			tokenHash := sha256Hex(req.Token)
			tokenObf := xorObfuscate(req.Token)
			ldflags = append(ldflags, fmt.Sprintf("-X main.EmbeddedTokenHash=%s", tokenHash))
			ldflags = append(ldflags, fmt.Sprintf("-X main.EmbeddedTokenObf=%s", tokenObf))
		}

		if !req.SkipConfig {
			if req.ServerIP != "" {
				ldflags = append(ldflags, fmt.Sprintf("-X main.EmbeddedServerIP=%s", req.ServerIP))
			}
			if req.ServerDomain != "" {
				ldflags = append(ldflags, fmt.Sprintf("-X main.EmbeddedServerDomain=%s", req.ServerDomain))
			}
			if req.ServerPort != "" {
				ldflags = append(ldflags, fmt.Sprintf("-X main.EmbeddedServerPort=%s", req.ServerPort))
			}
		}

		// ── Create temp output ──────────────────────────────────────────────
		tmpDir, err := os.MkdirTemp("", "edr-build-*")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Failed to create temp directory: " + err.Error(),
			})
			return
		}
		defer os.RemoveAll(tmpDir)

		outputPath := filepath.Join(tmpDir, "edr-agent.exe")

		// ── Cross-compile ───────────────────────────────────────────────────
		cmd := exec.Command("go", "build",
			"-ldflags", strings.Join(ldflags, " "),
			"-o", outputPath,
			"./cmd/agent",
		)
		cmd.Dir = agentSrc
		cmd.Env = append(os.Environ(),
			"GOOS=windows",
			"GOARCH=amd64",
			"CGO_ENABLED=1",                          // Required: ETW collector uses C (cgo)
			"CC=x86_64-w64-mingw32-gcc",              // MinGW cross-compiler for Windows
			"GOPROXY=https://goproxy.io,direct",
		)

		buildStart := time.Now()
		buildOutput, err := cmd.CombinedOutput()
		buildDuration := time.Since(buildStart)

		if err != nil {
			log.Printf("[BUILD] FAILED in %s: %v\nOutput: %s", buildDuration, err, string(buildOutput))
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error":        "Build failed",
				"build_output": string(buildOutput),
				"duration":     buildDuration.String(),
			})
			return
		}

		log.Printf("[BUILD] SUCCESS in %s, output: %s", buildDuration, outputPath)

		// ── Read binary + compute hash ──────────────────────────────────────
		binaryData, err := os.ReadFile(outputPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Failed to read built binary: " + err.Error(),
			})
			return
		}

		hash := sha256.Sum256(binaryData)
		hashStr := fmt.Sprintf("%x", hash)

		log.Printf("[BUILD] Binary size: %d bytes, SHA256: %s", len(binaryData), hashStr[:16]+"...")

		// ── Return binary as download ───────────────────────────────────────
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="edr-agent.exe"`)
		w.Header().Set("X-Agent-SHA256", hashStr)
		w.Header().Set("X-Build-Duration", buildDuration.String())
		w.WriteHeader(http.StatusOK)
		w.Write(binaryData)
	})

	log.Printf("Agent Builder listening on :%s (agent source: %s)", port, agentSrc)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Failed to start builder: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// sha256Hex returns the lowercase hex-encoded SHA-256 hash of s.
// Used to compute the token hash before embedding it into the agent binary.
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// xorObfuscate XOR-encrypts the plaintext with a fixed key and returns it as hex.
// This prevents `strings binary` from revealing the enrollment token.
// The same key is compiled into the agent for decoding at runtime.
//
// NOTE: This is NOT cryptographic encryption — it is obfuscation to raise the
// bar against casual extraction. The real security comes from:
//   - DACL protection on the agent process/service (SYSTEM-only access)
//   - The token being a one-time enrollment secret (consumed on first use)
//   - The uninstall path using SHA-256 hash (irreversible)
func xorObfuscate(plaintext string) string {
	// 32-byte XOR key — compiled into both builder and agent.
	key := []byte("EDR-Agent-XOR-Key-2026!@#$%^&*()")
	data := []byte(plaintext)
	for i := range data {
		data[i] ^= key[i%len(key)]
	}
	return hex.EncodeToString(data)
}
