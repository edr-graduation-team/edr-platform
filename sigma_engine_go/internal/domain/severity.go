package domain

import (
	"fmt"
	"strings"
)

// Severity represents the severity level of an alert or rule.
type Severity int

const (
	SeverityInformational Severity = iota + 1
	SeverityLow
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

// String returns the lowercase string representation of the severity.
func (s Severity) String() string {
	switch s {
	case SeverityInformational:
		return "informational"
	case SeverityLow:
		return "low"
	case SeverityMedium:
		return "medium"
	case SeverityHigh:
		return "high"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// FromString parses a severity string and returns the corresponding Severity.
// Returns an error if the string is not recognized.
func SeverityFromString(level string) (Severity, error) {
	normalized := strings.ToLower(strings.TrimSpace(level))
	switch normalized {
	case "informational", "info":
		return SeverityInformational, nil
	case "low":
		return SeverityLow, nil
	case "medium":
		return SeverityMedium, nil
	case "high":
		return SeverityHigh, nil
	case "critical":
		return SeverityCritical, nil
	default:
		return SeverityMedium, fmt.Errorf("unknown severity level: %q", level)
	}
}

// SeverityFromStringSafe parses a severity string and returns the corresponding Severity.
// Returns a default value (SeverityMedium) if parsing fails.
func SeverityFromStringSafe(level string) Severity {
	sev, err := SeverityFromString(level)
	if err != nil {
		return SeverityMedium
	}
	return sev
}

// Compare returns:
//   - -1 if s < other
//   - 0 if s == other
//   - 1 if s > other
func (s Severity) Compare(other Severity) int {
	if s < other {
		return -1
	}
	if s > other {
		return 1
	}
	return 0
}

