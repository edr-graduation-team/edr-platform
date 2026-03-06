// Package ml provides tests for ML components.
package ml

import (
	"testing"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

func TestBaselineManager_LearnBaseline(t *testing.T) {
	manager := NewBaselineManager()

	events := []map[string]interface{}{
		{"process_name": "cmd.exe", "parent_process": "explorer.exe"},
		{"process_name": "cmd.exe", "parent_process": "explorer.exe"},
		{"process_name": "powershell.exe", "parent_process": "cmd.exe"},
		{"process_name": "notepad.exe", "parent_process": "explorer.exe"},
	}

	baseline, err := manager.LearnBaseline("agent-001", BaselineProcess, events)
	if err != nil {
		t.Fatalf("LearnBaseline failed: %v", err)
	}

	if baseline.ID == "" {
		t.Error("Baseline ID should be generated")
	}
	if baseline.AgentID != "agent-001" {
		t.Error("Agent ID should be agent-001")
	}
	if baseline.LearnedFrom != 4 {
		t.Errorf("Expected 4 events, got %d", baseline.LearnedFrom)
	}
}

func TestBaselineManager_GetBaseline(t *testing.T) {
	manager := NewBaselineManager()

	events := []map[string]interface{}{
		{"process_name": "cmd.exe"},
	}
	manager.LearnBaseline("agent-001", BaselineProcess, events)

	// Found
	baseline, err := manager.GetBaseline("agent-001", BaselineProcess)
	if err != nil {
		t.Errorf("GetBaseline should find baseline: %v", err)
	}
	if baseline == nil {
		t.Error("Baseline should not be nil")
	}

	// Not found
	_, err = manager.GetBaseline("agent-999", BaselineProcess)
	if err == nil {
		t.Error("GetBaseline should return error for non-existent")
	}
}

func TestBaselineManager_CalculateAnomalyScore(t *testing.T) {
	manager := NewBaselineManager()

	// Learn baseline
	events := []map[string]interface{}{
		{"process_name": "cmd.exe"},
		{"process_name": "powershell.exe"},
	}
	manager.LearnBaseline("", BaselineProcess, events)

	// Score alert
	alert := &domain.Alert{
		ID:        "alert-001",
		Severity:  domain.SeverityHigh,
		Timestamp: time.Now(),
		EventData: map[string]interface{}{
			"process_name": "malware.exe", // Unknown process
		},
	}

	score, err := manager.CalculateAnomalyScore(alert)
	if err != nil {
		t.Fatalf("CalculateAnomalyScore failed: %v", err)
	}

	if score.Score < 50 {
		t.Errorf("Unknown process should have high anomaly score, got %f", score.Score)
	}
}

func TestBaselineManager_Confidence(t *testing.T) {
	manager := NewBaselineManager()

	tests := []struct {
		sampleSize int
		minConf    float64
	}{
		{5, 0.3},
		{20, 0.5},
		{100, 0.7},
		{500, 0.85},
		{2000, 0.95},
	}

	for _, tt := range tests {
		conf := manager.calculateConfidence(tt.sampleSize)
		if conf < tt.minConf {
			t.Errorf("Sample size %d: expected min confidence %f, got %f", tt.sampleSize, tt.minConf, conf)
		}
	}
}

func TestBaselineManager_GetMLStatus(t *testing.T) {
	manager := NewBaselineManager()

	// Add baseline
	manager.LearnBaseline("agent-001", BaselineProcess, []map[string]interface{}{{"p": "test"}})

	status := manager.GetMLStatus()
	if status["total_baselines"].(int) != 1 {
		t.Error("Should have 1 baseline")
	}
}
