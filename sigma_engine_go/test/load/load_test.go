// Package load provides load testing utilities for Sigma Engine.
package load

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// LoadTestConfig configures load test parameters.
type LoadTestConfig struct {
	TargetURL   string        `json:"target_url"`
	Duration    time.Duration `json:"duration"`
	RPS         int           `json:"rps"`         // Requests per second
	Concurrency int           `json:"concurrency"` // Concurrent workers
	RampUp      time.Duration `json:"ramp_up"`     // Ramp up period
	Timeout     time.Duration `json:"timeout"`     // Request timeout
}

// LoadTestResult contains load test results.
type LoadTestResult struct {
	TotalRequests int64         `json:"total_requests"`
	SuccessCount  int64         `json:"success_count"`
	ErrorCount    int64         `json:"error_count"`
	Duration      time.Duration `json:"duration"`
	RPS           float64       `json:"rps"`
	AvgLatencyMs  float64       `json:"avg_latency_ms"`
	P50LatencyMs  float64       `json:"p50_latency_ms"`
	P95LatencyMs  float64       `json:"p95_latency_ms"`
	P99LatencyMs  float64       `json:"p99_latency_ms"`
	MinLatencyMs  float64       `json:"min_latency_ms"`
	MaxLatencyMs  float64       `json:"max_latency_ms"`
	ErrorRate     float64       `json:"error_rate"`
}

// LoadTester runs load tests.
type LoadTester struct {
	config    LoadTestConfig
	client    *http.Client
	latencies []time.Duration
	mu        sync.Mutex

	totalRequests int64
	successCount  int64
	errorCount    int64
}

// NewLoadTester creates a new load tester.
func NewLoadTester(config LoadTestConfig) *LoadTester {
	return &LoadTester{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		latencies: make([]time.Duration, 0, 10000),
	}
}

// Run executes the load test.
func (lt *LoadTester) Run(ctx context.Context) (*LoadTestResult, error) {
	start := time.Now()

	// Create worker pool
	var wg sync.WaitGroup
	requestChan := make(chan struct{}, lt.config.RPS*2)

	// Start workers
	for i := 0; i < lt.config.Concurrency; i++ {
		wg.Add(1)
		go lt.worker(ctx, &wg, requestChan)
	}

	// Rate limiter
	ticker := time.NewTicker(time.Second / time.Duration(lt.config.RPS))
	defer ticker.Stop()

	endTime := time.Now().Add(lt.config.Duration)

	for time.Now().Before(endTime) {
		select {
		case <-ctx.Done():
			close(requestChan)
			wg.Wait()
			return lt.computeResults(time.Since(start)), ctx.Err()
		case <-ticker.C:
			select {
			case requestChan <- struct{}{}:
			default:
				// Channel full, skip
			}
		}
	}

	close(requestChan)
	wg.Wait()

	return lt.computeResults(time.Since(start)), nil
}

// worker processes requests.
func (lt *LoadTester) worker(ctx context.Context, wg *sync.WaitGroup, requests <-chan struct{}) {
	defer wg.Done()

	for range requests {
		select {
		case <-ctx.Done():
			return
		default:
			lt.makeRequest()
		}
	}
}

// makeRequest sends a single request.
func (lt *LoadTester) makeRequest() {
	start := time.Now()

	req, err := http.NewRequest("GET", lt.config.TargetURL+"/api/v1/sigma/stats/alerts", nil)
	if err != nil {
		atomic.AddInt64(&lt.errorCount, 1)
		return
	}

	resp, err := lt.client.Do(req)
	latency := time.Since(start)

	atomic.AddInt64(&lt.totalRequests, 1)

	if err != nil {
		atomic.AddInt64(&lt.errorCount, 1)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		atomic.AddInt64(&lt.successCount, 1)
	} else {
		atomic.AddInt64(&lt.errorCount, 1)
	}

	lt.mu.Lock()
	lt.latencies = append(lt.latencies, latency)
	lt.mu.Unlock()
}

// computeResults calculates final results.
func (lt *LoadTester) computeResults(duration time.Duration) *LoadTestResult {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	result := &LoadTestResult{
		TotalRequests: lt.totalRequests,
		SuccessCount:  lt.successCount,
		ErrorCount:    lt.errorCount,
		Duration:      duration,
	}

	if duration.Seconds() > 0 {
		result.RPS = float64(lt.totalRequests) / duration.Seconds()
	}

	if lt.totalRequests > 0 {
		result.ErrorRate = float64(lt.errorCount) / float64(lt.totalRequests)
	}

	if len(lt.latencies) > 0 {
		// Sort latencies for percentiles
		sorted := make([]time.Duration, len(lt.latencies))
		copy(sorted, lt.latencies)

		// Simple sort (for accuracy, use a proper sort)
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[i] > sorted[j] {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}

		// Calculate metrics
		var total time.Duration
		for _, l := range sorted {
			total += l
		}
		result.AvgLatencyMs = float64(total.Milliseconds()) / float64(len(sorted))
		result.MinLatencyMs = float64(sorted[0].Milliseconds())
		result.MaxLatencyMs = float64(sorted[len(sorted)-1].Milliseconds())

		p50Idx := len(sorted) * 50 / 100
		p95Idx := len(sorted) * 95 / 100
		p99Idx := len(sorted) * 99 / 100

		result.P50LatencyMs = float64(sorted[p50Idx].Milliseconds())
		result.P95LatencyMs = float64(sorted[p95Idx].Milliseconds())
		result.P99LatencyMs = float64(sorted[p99Idx].Milliseconds())
	}

	return result
}

// String returns a formatted result.
func (r *LoadTestResult) String() string {
	data, _ := json.MarshalIndent(r, "", "  ")
	return string(data)
}

// TestBaselineLoad tests baseline performance.
func TestBaselineLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	config := LoadTestConfig{
		TargetURL:   "http://localhost:8080",
		Duration:    30 * time.Second,
		RPS:         100,
		Concurrency: 10,
		Timeout:     5 * time.Second,
	}

	tester := NewLoadTester(config)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := tester.Run(ctx)
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("Load test completed with: %v", err)
	}

	t.Logf("Results:\n%s", result.String())

	// Assertions
	assert.Less(t, result.ErrorRate, 0.01, "Error rate should be <1%")
	assert.Less(t, result.P95LatencyMs, 100.0, "P95 latency should be <100ms")
	assert.Greater(t, result.RPS, 50.0, "RPS should be >50")
}

// TestRampLoad tests ramp-up performance.
func TestRampLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	// Ramp from 10 to 500 RPS
	rpsLevels := []int{10, 50, 100, 200, 500}

	for _, rps := range rpsLevels {
		t.Run(fmt.Sprintf("RPS_%d", rps), func(t *testing.T) {
			config := LoadTestConfig{
				TargetURL:   "http://localhost:8080",
				Duration:    10 * time.Second,
				RPS:         rps,
				Concurrency: rps / 10,
				Timeout:     5 * time.Second,
			}

			if config.Concurrency < 1 {
				config.Concurrency = 1
			}

			tester := NewLoadTester(config)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, _ := tester.Run(ctx)
			t.Logf("RPS %d: actual=%.1f, p95=%.1fms, errors=%.2f%%",
				rps, result.RPS, result.P95LatencyMs, result.ErrorRate*100)

			assert.Less(t, result.ErrorRate, 0.05, "Error rate should be <5%")
		})
	}
}

// TestSustainedLoad tests sustained performance.
func TestSustainedLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	config := LoadTestConfig{
		TargetURL:   "http://localhost:8080",
		Duration:    5 * time.Minute,
		RPS:         200,
		Concurrency: 20,
		Timeout:     5 * time.Second,
	}

	tester := NewLoadTester(config)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := tester.Run(ctx)
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("Load test completed with: %v", err)
	}

	t.Logf("Sustained Results:\n%s", result.String())

	// Sustained should maintain performance
	assert.Less(t, result.ErrorRate, 0.01, "Error rate should be <1%")
	assert.Less(t, result.P95LatencyMs, 100.0, "P95 latency should be <100ms")
}
