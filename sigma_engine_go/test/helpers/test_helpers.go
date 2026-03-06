package helpers

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

// GenerateWindowsProcessCreationEvent creates a realistic Windows process creation event.
func GenerateWindowsProcessCreationEvent() map[string]interface{} {
	rand.Seed(time.Now().UnixNano())
	pid := rand.Intn(65535) + 1000

	processes := []struct {
		Image       string
		CommandLine string
		Parent      string
	}{
		{"C:\\Windows\\System32\\cmd.exe", "cmd.exe /c dir", "C:\\Windows\\explorer.exe"},
		{"C:\\Windows\\System32\\powershell.exe", "powershell.exe -EncodedCommand ...", "C:\\Windows\\System32\\cmd.exe"},
		{"C:\\Windows\\System32\\notepad.exe", "notepad.exe", "C:\\Windows\\explorer.exe"},
		{"C:\\Program Files\\Test\\app.exe", "app.exe --flag", "C:\\Windows\\System32\\svchost.exe"},
	}

	proc := processes[rand.Intn(len(processes))]

	return map[string]interface{}{
		"@timestamp": time.Now().Format(time.RFC3339Nano),
		"event.code": 1,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":             proc.Image,
				"CommandLine":       proc.CommandLine,
				"ProcessId":         pid,
				"ParentImage":       proc.Parent,
				"ParentProcessId":   pid - 100,
				"User":              "DOMAIN\\user",
				"LogonId":           rand.Intn(1000000),
				"ProcessGuid":       fmt.Sprintf("{%s}", generateGUID()),
				"ParentProcessGuid": fmt.Sprintf("{%s}", generateGUID()),
			},
		},
		"host.name":   "TEST-HOST",
		"user.domain": "DOMAIN",
		"user.name":   "user",
	}
}

// GenerateWindowsNetworkEvent creates a network connection event.
func GenerateWindowsNetworkEvent() map[string]interface{} {
	rand.Seed(time.Now().UnixNano())
	ips := []string{"192.168.1.100", "10.0.0.50", "172.16.0.10", "8.8.8.8", "1.1.1.1"}
	ports := []int{80, 443, 53, 3389, 22, 445}

	return map[string]interface{}{
		"@timestamp": time.Now().Format(time.RFC3339Nano),
		"event.code": 3,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":           "C:\\Windows\\System32\\svchost.exe",
				"ProcessId":       rand.Intn(65535) + 1000,
				"DestinationIp":   ips[rand.Intn(len(ips))],
				"DestinationPort": ports[rand.Intn(len(ports))],
				"Protocol":        "tcp",
				"Initiated":       true,
				"SourceIp":        "192.168.1.1",
				"SourcePort":      rand.Intn(65535),
			},
		},
		"host.name": "TEST-HOST",
	}
}

// GenerateWindowsRegistryEvent creates a registry modification event.
func GenerateWindowsRegistryEvent() map[string]interface{} {
	rand.Seed(time.Now().UnixNano())
	keys := []string{
		"HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run",
		"HKU\\S-1-5-21-*\\Software\\Microsoft\\Windows\\CurrentVersion\\Run",
		"HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run",
	}

	return map[string]interface{}{
		"@timestamp": time.Now().Format(time.RFC3339Nano),
		"event.code": 13,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":        "C:\\Windows\\System32\\reg.exe",
				"ProcessId":    rand.Intn(65535) + 1000,
				"TargetObject": keys[rand.Intn(len(keys))],
				"Details":      "test_value",
			},
		},
		"host.name": "TEST-HOST",
	}
}

// GenerateWindowsFileEvent creates a file creation event.
func GenerateWindowsFileEvent() map[string]interface{} {
	rand.Seed(time.Now().UnixNano())
	files := []string{
		"C:\\Users\\user\\Downloads\\file.exe",
		"C:\\Temp\\suspicious.dll",
		"C:\\Windows\\Temp\\payload.bat",
	}

	return map[string]interface{}{
		"@timestamp": time.Now().Format(time.RFC3339Nano),
		"event.code": 11,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":          "C:\\Windows\\explorer.exe",
				"ProcessId":      rand.Intn(65535) + 1000,
				"TargetFilename": files[rand.Intn(len(files))],
			},
		},
		"host.name": "TEST-HOST",
	}
}

// GenerateSuspiciousPowerShellEvent creates a suspicious PowerShell event.
func GenerateSuspiciousPowerShellEvent() map[string]interface{} {
	return map[string]interface{}{
		"@timestamp": time.Now().Format(time.RFC3339Nano),
		"event.code": 1,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":       "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
				"CommandLine": "powershell.exe -EncodedCommand SQBuAHYAbwBrAGUALQBXAGUAYgBSAGUAcQB1AGUAcwB0AA==",
				"ProcessId":   1234,
				"ParentImage": "C:\\Windows\\System32\\cmd.exe",
			},
		},
		"host.name":   "TEST-HOST",
		"user.domain": "DOMAIN",
		"user.name":   "user",
	}
}

// GenerateBatchEvents generates N events for batch testing.
func GenerateBatchEvents(count int, eventType string) []*domain.LogEvent {
	events := make([]*domain.LogEvent, 0, count)

	for i := 0; i < count; i++ {
		var eventData map[string]interface{}

		switch eventType {
		case "process":
			eventData = GenerateWindowsProcessCreationEvent()
		case "network":
			eventData = GenerateWindowsNetworkEvent()
		case "registry":
			eventData = GenerateWindowsRegistryEvent()
		case "file":
			eventData = GenerateWindowsFileEvent()
		case "powershell":
			eventData = GenerateSuspiciousPowerShellEvent()
		default:
			eventData = GenerateWindowsProcessCreationEvent()
		}

		event, err := domain.NewLogEvent(eventData)
		if err == nil {
			events = append(events, event)
		}
	}

	return events
}

// GenerateTestRule creates a valid Sigma rule for testing.
func GenerateTestRule(id, title, product, category string) *domain.SigmaRule {
	productPtr := &product
	categoryPtr := &category
	service := "sysmon"
	servicePtr := &service

	selection := &domain.Selection{
		Name: "selection1",
		Fields: []domain.SelectionField{
			{
				FieldName: "Image",
				Values:    []interface{}{"test.exe"},
				Modifiers: []string{"endswith"},
			},
			{
				FieldName: "CommandLine",
				Values:    []interface{}{"test"},
				Modifiers: []string{"contains"},
			},
		},
	}

	detection := &domain.Detection{
		Selections: map[string]*domain.Selection{
			"selection1": selection,
		},
		Condition: "selection1",
	}

	return &domain.SigmaRule{
		ID:          id,
		Title:       title,
		Description: "Test rule",
		LogSource: domain.LogSource{
			Product:  productPtr,
			Category: categoryPtr,
			Service:  servicePtr,
		},
		Detection: *detection,
		Level:     "medium",
		Status:    "stable",
		Tags:      []string{"attack.execution", "attack.t1059"},
	}
}

// generateGUID generates a simple GUID-like string for testing.
func generateGUID() string {
	rand.Seed(time.Now().UnixNano())
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Uint32(),
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint64()&0xffffffffffff,
	)
}
