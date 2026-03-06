// Package optimization provides performance tests.
package optimization

import (
	"testing"
	"time"
)

func TestCache_SetGet(t *testing.T) {
	cache := NewCache(5*time.Minute, 100)

	// Set value
	cache.Set("key1", "value1")

	// Get value
	val, found := cache.Get("key1")
	if !found {
		t.Error("Expected value to be found")
	}
	if val != "value1" {
		t.Errorf("Expected 'value1', got '%v'", val)
	}
}

func TestCache_Expiration(t *testing.T) {
	cache := NewCache(50*time.Millisecond, 100)

	cache.Set("key1", "value1")

	// Should be found immediately
	_, found := cache.Get("key1")
	if !found {
		t.Error("Value should be found")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	_, found = cache.Get("key1")
	if found {
		t.Error("Value should have expired")
	}
}

func TestCache_Delete(t *testing.T) {
	cache := NewCache(5*time.Minute, 100)

	cache.Set("key1", "value1")
	cache.Delete("key1")

	_, found := cache.Get("key1")
	if found {
		t.Error("Value should be deleted")
	}
}

func TestCache_Clear(t *testing.T) {
	cache := NewCache(5*time.Minute, 100)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Cache should be empty, got %d", cache.Size())
	}
}

func TestCache_Eviction(t *testing.T) {
	cache := NewCache(5*time.Minute, 3)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")
	cache.Set("key4", "value4") // Should evict oldest

	if cache.Size() != 3 {
		t.Errorf("Cache size should be 3, got %d", cache.Size())
	}
}

func TestCache_Stats(t *testing.T) {
	cache := NewCache(5*time.Minute, 100)

	cache.Set("key1", "value1")

	stats := cache.Stats()
	if stats["size"].(int) != 1 {
		t.Error("Stats should show 1 entry")
	}
	if stats["max_size"].(int) != 100 {
		t.Error("Stats should show max_size 100")
	}
}

func TestResponseCache(t *testing.T) {
	rc := NewResponseCache()

	// Test alerts cache
	rc.SetAlerts("test", []string{"alert1", "alert2"})
	val, found := rc.GetAlerts("test")
	if !found {
		t.Error("Alerts should be found")
	}
	if len(val.([]string)) != 2 {
		t.Error("Should have 2 alerts")
	}

	// Test invalidate
	rc.InvalidateAlerts()
	_, found = rc.GetAlerts("test")
	if found {
		t.Error("Alerts should be invalidated")
	}
}

func BenchmarkCache_Set(b *testing.B) {
	cache := NewCache(5*time.Minute, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("key", "value")
	}
}

func BenchmarkCache_Get(b *testing.B) {
	cache := NewCache(5*time.Minute, 10000)
	cache.Set("key", "value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("key")
	}
}
