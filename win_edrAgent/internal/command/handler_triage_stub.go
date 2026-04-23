//go:build !windows
// +build !windows

package command

import (
	"context"
	"fmt"
)

func (h *Handler) postIsolationTriage(_ context.Context, _ map[string]string) (string, error) {
	return "", fmt.Errorf("post_isolation_triage is only supported on Windows")
}
func (h *Handler) processTreeSnapshot(_ context.Context, _ map[string]string) (string, error) {
	return "", fmt.Errorf("process_tree_snapshot is only supported on Windows")
}
func (h *Handler) persistenceScan(_ context.Context, _ map[string]string) (string, error) {
	return "", fmt.Errorf("persistence_scan is only supported on Windows")
}
func (h *Handler) lsassAccessAudit(_ context.Context, _ map[string]string) (string, error) {
	return "", fmt.Errorf("lsass_access_audit is only supported on Windows")
}
func (h *Handler) filesystemTimeline(_ context.Context, _ map[string]string) (string, error) {
	return "", fmt.Errorf("filesystem_timeline is only supported on Windows")
}
func (h *Handler) networkLastSeen(_ context.Context, _ map[string]string) (string, error) {
	return "", fmt.Errorf("network_last_seen is only supported on Windows")
}
func (h *Handler) agentIntegrityCheck(_ context.Context, _ map[string]string) (string, error) {
	return "", fmt.Errorf("agent_integrity_check is only supported on Windows")
}
func (h *Handler) memoryDump(_ context.Context, _ map[string]string) (string, error) {
	return "", fmt.Errorf("memory_dump is only supported on Windows")
}
