// Package main provides a CLI utility for decrypting EDR Agent log files.
//
// The agent encrypts its log entries using AES-256-GCM, with the encryption
// key protected by Windows DPAPI (CryptProtectData with CRYPTPROTECT_LOCAL_MACHINE).
//
// This means:
//   - Only a process running on the SAME MACHINE can decrypt the logs.
//   - The process must run as LocalSystem or an Administrator (DPAPI LOCAL_MACHINE scope).
//   - The decryption key file is at C:\ProgramData\EDR\security\log.key
//
// Usage:
//
//	edr-log-reader.exe                                   # decrypt default log
//	edr-log-reader.exe -log C:\ProgramData\EDR\logs\agent.log
//	edr-log-reader.exe -log C:\ProgramData\EDR\logs\agent.log -key C:\ProgramData\EDR\security\log.key
//	edr-log-reader.exe -log C:\ProgramData\EDR\logs\agent.log -out decrypted.log
//	edr-log-reader.exe -tail 50                          # show last 50 lines
//
//go:build windows
// +build windows

package main

import (
	"bufio"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/edr-platform/win-agent/internal/logging"
	"github.com/edr-platform/win-agent/internal/security"
)

const (
	defaultLogPath = `C:\ProgramData\EDR\logs\agent.log`
	defaultKeyPath = `C:\ProgramData\EDR\security\log.key`
)

func main() {
	logPath := flag.String("log", defaultLogPath, "Path to the encrypted log file")
	keyPath := flag.String("key", defaultKeyPath, "Path to the DPAPI-protected AES key file")
	outPath := flag.String("out", "", "Output file for decrypted logs (default: stdout)")
	tailN := flag.Int("tail", 0, "Show only the last N lines (0 = all)")
	flag.Parse()

	// Initialize the encryptor (loads key from DPAPI store)
	logger := logging.NewLogger(logging.Config{Level: "ERROR"})
	defer logger.Close()

	enc, err := security.NewEncryptor(*keyPath, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot load encryption key from %s\n", *keyPath)
		fmt.Fprintf(os.Stderr, "       %v\n\n", err)
		fmt.Fprintf(os.Stderr, "Possible causes:\n")
		fmt.Fprintf(os.Stderr, "  1. You are not running on the same machine as the agent.\n")
		fmt.Fprintf(os.Stderr, "  2. You are not running as Administrator or SYSTEM.\n")
		fmt.Fprintf(os.Stderr, "  3. The key file does not exist (agent never started?).\n")
		fmt.Fprintf(os.Stderr, "\nTip: Run this tool as Administrator on the agent machine.\n")
		os.Exit(1)
	}

	// Open the log file
	f, err := os.Open(*logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot open log file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(f)
	// Increase buffer size for long encrypted lines
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading log file: %v\n", err)
		os.Exit(1)
	}

	// Apply tail filter
	if *tailN > 0 && *tailN < len(lines) {
		lines = lines[len(lines)-*tailN:]
	}

	// Determine output destination
	var out *os.File
	if *outPath != "" {
		out, err = os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer out.Close()
	} else {
		out = os.Stdout
	}

	// Decrypt each line
	decrypted := 0
	plaintext := 0
	errors := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to decode as base64 (encrypted line)
		ciphertext, b64Err := base64.StdEncoding.DecodeString(line)
		if b64Err == nil && len(ciphertext) > 12 {
			// Looks like an encrypted line — try to decrypt
			plain, decErr := enc.Decrypt(ciphertext)
			if decErr == nil {
				fmt.Fprint(out, string(plain))
				decrypted++
				continue
			}
		}

		// Not encrypted or decryption failed — print as-is (plaintext line)
		fmt.Fprintln(out, line)
		plaintext++
	}

	// Print summary to stderr (so it doesn't mix with decrypted output)
	if *outPath != "" {
		fmt.Fprintf(os.Stderr, "Decrypted %d lines, %d plaintext, %d errors → %s\n",
			decrypted, plaintext, errors, *outPath)
	}
}
