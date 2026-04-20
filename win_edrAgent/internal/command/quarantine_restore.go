package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readOriginalPathFromQuarantineMeta(metaPath string) (string, error) {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return "", fmt.Errorf("read meta %s: %w", metaPath, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "originalpath:") {
			_, rest, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			p := strings.TrimSpace(rest)
			if p != "" {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("OriginalPath not found in %s", metaPath)
}

func (h *Handler) restoreQuarantineFile(ctx context.Context, params map[string]string) (string, error) {
	_ = ctx
	qp := strings.TrimSpace(params["quarantine_path"])
	if qp == "" {
		return "", fmt.Errorf("quarantine_path is required")
	}
	orig := strings.TrimSpace(params["original_path"])
	if orig == "" {
		var err error
		orig, err = readOriginalPathFromQuarantineMeta(qp + ".meta")
		if err != nil {
			return "", err
		}
	}
	if orig == "" {
		return "", fmt.Errorf("could not resolve original path")
	}
	if err := os.MkdirAll(filepath.Dir(orig), 0755); err != nil {
		return "", fmt.Errorf("create parent dir: %w", err)
	}
	if _, err := os.Stat(orig); err == nil {
		return "", fmt.Errorf("refusing to overwrite existing file at %s", orig)
	}
	if err := os.Rename(qp, orig); err != nil {
		if err := copyFile(qp, orig); err != nil {
			return "", fmt.Errorf("restore copy failed: %w", err)
		}
		_ = os.Remove(qp)
	}
	_ = os.Remove(qp + ".meta")
	return fmt.Sprintf("Restored quarantine file to %s", orig), nil
}

func (h *Handler) deleteQuarantineFile(ctx context.Context, params map[string]string) (string, error) {
	_ = ctx
	qp := strings.TrimSpace(params["quarantine_path"])
	if qp == "" {
		return "", fmt.Errorf("quarantine_path is required")
	}
	_ = os.Remove(qp + ".meta")
	if err := os.Remove(qp); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove quarantine file: %w", err)
	}
	return fmt.Sprintf("Deleted quarantine object %s", qp), nil
}
