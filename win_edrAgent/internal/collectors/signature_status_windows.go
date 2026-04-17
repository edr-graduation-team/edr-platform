//go:build windows
// +build windows

package collectors

import "strings"

// SignatureTrustClass maps the main process image path to coarse trust labels
// consumed by the Sigma engine RiskScorer (computeFPDiscount / computeFPRisk).
//
// Contract (lowercase strings, see sigma_engine_go risk_scorer.go):
//   - "microsoft" — OS / Microsoft publisher context (enables Microsoft FP discount)
//   - "trusted"   — PE has embedded Authenticode directory (heuristic: signed binary)
//   - "unsigned"  — No embedded signature directory
//   - ""          — Path empty or unreadable
//
// This uses the same lightweight PE Security Directory probe as image-load
// hashing (isFileSigned). It is not a full WinVerifyTrust chain; callers
// should treat issuer as best-effort empty unless extended later.
func SignatureTrustClass(imagePath string) (signatureStatus, signatureIssuer string) {
	if strings.TrimSpace(imagePath) == "" {
		return "", ""
	}
	if !isFileSigned(imagePath) {
		return "unsigned", ""
	}
	lower := strings.ToLower(strings.ReplaceAll(imagePath, "/", `\`))
	if strings.Contains(lower, `\windows\`) ||
		strings.Contains(lower, `\program files\windowsapps\`) ||
		strings.Contains(lower, `\program files\microsoft`) ||
		strings.Contains(lower, `\program files (x86)\microsoft`) {
		return "microsoft", ""
	}
	return "trusted", ""
}
