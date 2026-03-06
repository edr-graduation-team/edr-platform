package utils

import (
	"path/filepath"
	"regexp"
	"strings"
)

// NormalizeWindowsPath normalizes a Windows path.
// Converts forward slashes to backslashes and handles UNC paths.
func NormalizeWindowsPath(path string) string {
	if path == "" {
		return path
	}

	// Handle UNC paths (\\server\share)
	if strings.HasPrefix(path, "\\\\") {
		return strings.ReplaceAll(path, "/", "\\")
	}

	// Normalize separators
	normalized := strings.ReplaceAll(path, "/", "\\")

	// Remove duplicate backslashes (except at start for UNC)
	if strings.HasPrefix(normalized, "\\\\") {
		rest := normalized[2:]
		rest = regexp.MustCompile(`\\+`).ReplaceAllString(rest, "\\")
		normalized = "\\\\" + rest
	} else {
		normalized = regexp.MustCompile(`\\+`).ReplaceAllString(normalized, "\\")
	}

	return normalized
}

// NormalizeLinuxPath normalizes a Linux/Unix path.
// Removes duplicate slashes and resolves . and .. components.
func NormalizeLinuxPath(path string) string {
	if path == "" {
		return path
	}

	// Use filepath.Clean for normalization
	normalized := filepath.Clean(path)

	// Ensure absolute paths start with /
	if strings.HasPrefix(path, "/") && !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}

	return normalized
}

// CompareStrings compares two strings with optional case-insensitive matching.
func CompareStrings(a, b string, ignoreCase bool) bool {
	if ignoreCase {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// EscapeRegex escapes special regex characters in a string.
func EscapeRegex(pattern string) string {
	specialChars := []string{".", "+", "*", "?", "^", "$", "(", ")", "[", "]", "{", "}", "|", "\\"}
	result := pattern
	for _, char := range specialChars {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}
	return result
}

// ContainsAny checks if the string contains any of the substrings.
func ContainsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// ContainsAll checks if the string contains all of the substrings.
func ContainsAll(s string, substrings []string) bool {
	for _, substr := range substrings {
		if !strings.Contains(s, substr) {
			return false
		}
	}
	return true
}

