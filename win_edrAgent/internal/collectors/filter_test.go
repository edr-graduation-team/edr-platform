package collectors

import (
	"testing"

	"github.com/edr-platform/win-agent/internal/event"
)

func TestNewFilter(t *testing.T) {
	cfg := FilterConfig{
		ExcludeProcesses: []string{"svchost.exe", "csrss.exe"},
		ExcludeIPs:       []string{"127.0.0.1", "10.0.0.0/8"},
		ExcludeRegistry:  []string{"HKLM\\SOFTWARE\\Microsoft"},
		ExcludePaths:     []string{"C:\\Windows\\Temp"},
		IncludePaths:     []string{"C:\\Windows\\System32"},
	}

	filter := NewFilter(cfg, nil)

	if filter == nil {
		t.Fatal("NewFilter returned nil")
	}
	if len(filter.excludeProcesses) != 2 {
		t.Errorf("expected 2 excluded processes, got %d", len(filter.excludeProcesses))
	}
	if len(filter.excludeIPs) != 2 {
		t.Errorf("expected 2 excluded IP ranges, got %d", len(filter.excludeIPs))
	}
}

func TestFilterProcess(t *testing.T) {
	cfg := FilterConfig{
		ExcludeProcesses: []string{"svchost.exe", "csrss.exe", "agent.exe"},
	}
	filter := NewFilter(cfg, nil)

	tests := []struct {
		name         string
		processName  string
		shouldFilter bool
	}{
		{"excluded svchost", "svchost.exe", true},
		{"excluded csrss", "csrss.exe", true},
		{"excluded agent", "agent.exe", true},
		{"case insensitive", "SVCHOST.EXE", true},
		{"allowed notepad", "notepad.exe", false},
		{"allowed powershell", "powershell.exe", false},
		{"allowed chrome", "chrome.exe", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
				"name": tt.processName,
			})

			result := filter.ShouldFilter(evt)
			if result != tt.shouldFilter {
				t.Errorf("%s: expected filter=%v, got %v", tt.name, tt.shouldFilter, result)
			}
		})
	}
}

func TestFilterNetwork(t *testing.T) {
	cfg := FilterConfig{
		ExcludeIPs: []string{"127.0.0.1", "10.0.0.0/8", "192.168.0.0/16"},
	}
	filter := NewFilter(cfg, nil)

	tests := []struct {
		name         string
		srcIP        string
		dstIP        string
		shouldFilter bool
	}{
		{"localhost src", "127.0.0.1", "8.8.8.8", true},
		{"localhost dst", "8.8.8.8", "127.0.0.1", true},
		{"internal 10.x", "10.1.2.3", "8.8.8.8", true},
		{"internal 192.168.x", "192.168.1.100", "8.8.8.8", true},
		{"external to external", "8.8.8.8", "1.1.1.1", false},
		{"external connection", "100.100.100.100", "8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := event.NewEvent(event.EventTypeNetwork, event.SeverityLow, map[string]interface{}{
				"source_ip":      tt.srcIP,
				"destination_ip": tt.dstIP,
			})

			result := filter.ShouldFilter(evt)
			if result != tt.shouldFilter {
				t.Errorf("%s: expected filter=%v, got %v", tt.name, tt.shouldFilter, result)
			}
		})
	}
}

func TestFilterRegistry(t *testing.T) {
	cfg := FilterConfig{
		ExcludeRegistry: []string{
			"HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Component Based Servicing",
			"HKLM\\SYSTEM\\CurrentControlSet\\Services\\bam",
		},
	}
	filter := NewFilter(cfg, nil)

	tests := []struct {
		name         string
		keyPath      string
		shouldFilter bool
	}{
		{"excluded CBS", "HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Component Based Servicing\\Packages", true},
		{"excluded bam", "HKLM\\SYSTEM\\CurrentControlSet\\Services\\bam\\State\\UserSettings", true},
		{"allowed Run key", "HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run", false},
		{"allowed Services", "HKLM\\SYSTEM\\CurrentControlSet\\Services\\MyService", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := event.NewEvent(event.EventTypeRegistry, event.SeverityMedium, map[string]interface{}{
				"key_path": tt.keyPath,
			})

			result := filter.ShouldFilter(evt)
			if result != tt.shouldFilter {
				t.Errorf("%s: expected filter=%v, got %v", tt.name, tt.shouldFilter, result)
			}
		})
	}
}

func TestFilterFile(t *testing.T) {
	cfg := FilterConfig{
		ExcludePaths: []string{"C:\\Windows\\Temp", "C:\\Users\\*\\AppData\\Local\\Temp"},
		IncludePaths: []string{"C:\\Windows\\System32"},
	}
	filter := NewFilter(cfg, nil)

	tests := []struct {
		name         string
		path         string
		shouldFilter bool
	}{
		{"excluded Temp", "C:\\Windows\\Temp\\file.txt", true},
		{"included System32", "C:\\Windows\\System32\\cmd.exe", false},
		{"not excluded", "C:\\Program Files\\app.exe", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := event.NewEvent(event.EventTypeFile, event.SeverityLow, map[string]interface{}{
				"path": tt.path,
			})

			result := filter.ShouldFilter(evt)
			if result != tt.shouldFilter {
				t.Errorf("%s: expected filter=%v, got %v", tt.name, tt.shouldFilter, result)
			}
		})
	}
}

func TestFilterStats(t *testing.T) {
	cfg := FilterConfig{
		ExcludeProcesses: []string{"svchost.exe"},
	}
	filter := NewFilter(cfg, nil)

	// Generate some events
	for i := 0; i < 10; i++ {
		evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
			"name": "svchost.exe",
		})
		filter.ShouldFilter(evt)
	}
	for i := 0; i < 5; i++ {
		evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
			"name": "notepad.exe",
		})
		filter.ShouldFilter(evt)
	}

	stats := filter.Stats()

	if stats.TotalEvents != 15 {
		t.Errorf("expected 15 total events, got %d", stats.TotalEvents)
	}
	if stats.FilteredEvents != 10 {
		t.Errorf("expected 10 filtered events, got %d", stats.FilteredEvents)
	}
	if stats.PassedEvents != 5 {
		t.Errorf("expected 5 passed events, got %d", stats.PassedEvents)
	}
}

func TestFilterUpdateExclusions(t *testing.T) {
	cfg := FilterConfig{
		ExcludeProcesses: []string{"old.exe"},
	}
	filter := NewFilter(cfg, nil)

	// Initial state
	evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
		"name": "new.exe",
	})
	if filter.ShouldFilter(evt) {
		t.Error("new.exe should not be filtered initially")
	}

	// Update exclusions
	newCfg := FilterConfig{
		ExcludeProcesses: []string{"new.exe"},
	}
	filter.UpdateExclusions(newCfg)

	// After update
	if !filter.ShouldFilter(evt) {
		t.Error("new.exe should be filtered after update")
	}
}

func BenchmarkFilterProcess(b *testing.B) {
	cfg := FilterConfig{
		ExcludeProcesses: []string{"svchost.exe", "csrss.exe", "services.exe", "lsass.exe", "dwm.exe"},
	}
	filter := NewFilter(cfg, nil)

	evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
		"name": "notepad.exe",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.ShouldFilter(evt)
	}
}

func BenchmarkFilterNetwork(b *testing.B) {
	cfg := FilterConfig{
		ExcludeIPs: []string{"127.0.0.1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
	}
	filter := NewFilter(cfg, nil)

	evt := event.NewEvent(event.EventTypeNetwork, event.SeverityLow, map[string]interface{}{
		"source_ip":      "100.100.100.100",
		"destination_ip": "8.8.8.8",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.ShouldFilter(evt)
	}
}
