package event

import (
	"testing"
	"time"
)

func TestNewBatcher(t *testing.T) {
	batcher := NewBatcher(50, time.Second, "snappy", nil)

	if batcher == nil {
		t.Fatal("NewBatcher returned nil")
	}
	if batcher.batchSize != 50 {
		t.Errorf("expected batch size 50, got %d", batcher.batchSize)
	}
	if batcher.interval != time.Second {
		t.Errorf("expected interval 1s, got %v", batcher.interval)
	}
}

func TestBatcherDefaults(t *testing.T) {
	batcher := NewBatcher(0, 0, "", nil)

	if batcher.batchSize != 50 {
		t.Errorf("expected default batch size 50, got %d", batcher.batchSize)
	}
	if batcher.interval != time.Second {
		t.Errorf("expected default interval 1s, got %v", batcher.interval)
	}
	if batcher.compression != "snappy" {
		t.Errorf("expected default compression snappy, got %s", batcher.compression)
	}
}

func TestBatcherAdd(t *testing.T) {
	batcher := NewBatcher(3, time.Second, "snappy", nil)

	// Add events
	evt1 := NewEvent(EventTypeProcess, SeverityLow, map[string]interface{}{"test": 1})
	evt2 := NewEvent(EventTypeProcess, SeverityLow, map[string]interface{}{"test": 2})
	evt3 := NewEvent(EventTypeProcess, SeverityLow, map[string]interface{}{"test": 3})

	// First two should not trigger batch
	batch := batcher.Add(evt1)
	if batch != nil {
		t.Error("batch returned too early after 1 event")
	}

	batch = batcher.Add(evt2)
	if batch != nil {
		t.Error("batch returned too early after 2 events")
	}

	// Third should trigger batch
	batch = batcher.Add(evt3)
	if batch == nil {
		t.Error("batch not returned after reaching threshold")
	}

	if batch.EventCount != 3 {
		t.Errorf("expected 3 events in batch, got %d", batch.EventCount)
	}
}

func TestBatcherFlush(t *testing.T) {
	batcher := NewBatcher(100, time.Second, "snappy", nil)

	evt := NewEvent(EventTypeNetwork, SeverityMedium, map[string]interface{}{"ip": "192.168.1.1"})
	batcher.Add(evt)

	// Force flush before threshold
	batch := batcher.Flush()
	if batch == nil {
		t.Error("Flush returned nil with pending events")
	}

	if batch.EventCount != 1 {
		t.Errorf("expected 1 event, got %d", batch.EventCount)
	}

	// Flush empty batcher
	emptyBatch := batcher.Flush()
	if emptyBatch != nil {
		t.Error("Flush should return nil when empty")
	}
}

func TestBatcherCompression(t *testing.T) {
	batcher := NewBatcher(2, time.Second, "snappy", nil)

	evt1 := NewEvent(EventTypeProcess, SeverityLow, map[string]interface{}{
		"name":         "test.exe",
		"command_line": "test.exe --long-argument-with-lots-of-text",
	})
	evt2 := NewEvent(EventTypeProcess, SeverityLow, map[string]interface{}{
		"name":         "test.exe",
		"command_line": "test.exe --another-long-argument-with-lots-of-text",
	})

	batcher.Add(evt1)
	batch := batcher.Add(evt2)

	if batch == nil {
		t.Fatal("batch not created")
	}

	if batch.Compression != "snappy" {
		t.Errorf("expected snappy compression, got %s", batch.Compression)
	}

	if len(batch.Payload) == 0 {
		t.Error("compressed payload is empty")
	}

	if batch.Checksum == "" {
		t.Error("checksum not generated")
	}
}

func TestBatcherFlushIfReady(t *testing.T) {
	batcher := NewBatcher(100, 50*time.Millisecond, "none", nil)

	evt := NewEvent(EventTypeFile, SeverityLow, map[string]interface{}{"path": "C:\\test.txt"})
	batcher.Add(evt)

	// Should not flush immediately
	batch := batcher.FlushIfReady()
	if batch != nil {
		t.Error("FlushIfReady triggered too early")
	}

	// Wait for interval
	time.Sleep(60 * time.Millisecond)

	batch = batcher.FlushIfReady()
	if batch == nil {
		t.Error("FlushIfReady should have triggered after interval")
	}
}

func TestBatcherCount(t *testing.T) {
	batcher := NewBatcher(100, time.Second, "snappy", nil)

	if batcher.Count() != 0 {
		t.Error("empty batcher should have count 0")
	}

	evt := NewEvent(EventTypeRegistry, SeverityHigh, map[string]interface{}{"key": "HKLM\\Run"})
	batcher.Add(evt)

	if batcher.Count() != 1 {
		t.Errorf("expected count 1, got %d", batcher.Count())
	}
}

func TestBatcherSetBatchSize(t *testing.T) {
	batcher := NewBatcher(50, time.Second, "snappy", nil)

	batcher.SetBatchSize(100)
	if batcher.batchSize != 100 {
		t.Errorf("expected batch size 100, got %d", batcher.batchSize)
	}

	// Test bounds
	batcher.SetBatchSize(0)
	if batcher.batchSize != 1 {
		t.Errorf("expected minimum batch size 1, got %d", batcher.batchSize)
	}

	batcher.SetBatchSize(20000)
	if batcher.batchSize != 10000 {
		t.Errorf("expected maximum batch size 10000, got %d", batcher.batchSize)
	}
}

func TestBatcherSetInterval(t *testing.T) {
	batcher := NewBatcher(50, time.Second, "snappy", nil)

	batcher.SetInterval(5 * time.Second)
	if batcher.interval != 5*time.Second {
		t.Errorf("expected interval 5s, got %v", batcher.interval)
	}

	// Test bounds
	batcher.SetInterval(10 * time.Millisecond)
	if batcher.interval != 100*time.Millisecond {
		t.Errorf("expected minimum interval 100ms, got %v", batcher.interval)
	}

	batcher.SetInterval(2 * time.Minute)
	if batcher.interval != 60*time.Second {
		t.Errorf("expected maximum interval 60s, got %v", batcher.interval)
	}
}

func TestBatchChecksum(t *testing.T) {
	batcher := NewBatcher(1, time.Second, "snappy", nil)

	evt := NewEvent(EventTypeProcess, SeverityLow, map[string]interface{}{"test": true})
	batch := batcher.Add(evt)

	if batch == nil {
		t.Fatal("batch not created")
	}

	if len(batch.Checksum) != 64 { // SHA256 hex = 64 chars
		t.Errorf("expected 64 char checksum, got %d", len(batch.Checksum))
	}
}

func TestNewEvent(t *testing.T) {
	data := map[string]interface{}{
		"pid":  1234,
		"name": "test.exe",
	}

	evt := NewEvent(EventTypeProcess, SeverityHigh, data)

	if evt.ID == "" {
		t.Error("event ID should not be empty")
	}
	if evt.Type != EventTypeProcess {
		t.Errorf("expected type process, got %s", evt.Type)
	}
	if evt.Severity != SeverityHigh {
		t.Errorf("expected severity high, got %v", evt.Severity)
	}
	if evt.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
	if evt.Data["pid"] != 1234 {
		t.Error("data not set correctly")
	}
}

func BenchmarkBatcherAdd(b *testing.B) {
	batcher := NewBatcher(1000, time.Second, "snappy", nil)

	evt := NewEvent(EventTypeProcess, SeverityLow, map[string]interface{}{
		"pid":          1234,
		"name":         "test.exe",
		"command_line": "test.exe --arg1 --arg2 --arg3",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batcher.Add(evt)
	}
}

func BenchmarkBatcherFlush(b *testing.B) {
	for i := 0; i < b.N; i++ {
		batcher := NewBatcher(100, time.Second, "snappy", nil)
		for j := 0; j < 100; j++ {
			evt := NewEvent(EventTypeProcess, SeverityLow, map[string]interface{}{
				"pid":  j,
				"name": "test.exe",
			})
			batcher.Add(evt)
		}
		batcher.Flush()
	}
}
