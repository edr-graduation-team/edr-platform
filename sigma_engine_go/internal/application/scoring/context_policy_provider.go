package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ContextFactors struct {
	UserRoleWeight          float64
	DeviceCriticalityWeight float64
	NetworkAnomalyFactor    float64
}

func (f ContextFactors) Multiplier() float64 {
	return clampFloat(f.UserRoleWeight*f.DeviceCriticalityWeight*f.NetworkAnomalyFactor, 0.25, 4.0)
}

func DefaultContextFactors() ContextFactors {
	return ContextFactors{
		UserRoleWeight:          1.0,
		DeviceCriticalityWeight: 1.0,
		NetworkAnomalyFactor:    1.0,
	}
}

type ContextPolicyProvider interface {
	Resolve(ctx context.Context, agentID, userName, sourceIP string) (ContextFactors, error)
}

type NoopContextPolicyProvider struct{}

func (NoopContextPolicyProvider) Resolve(_ context.Context, _, _, _ string) (ContextFactors, error) {
	return DefaultContextFactors(), nil
}

type policyRecord struct {
	ID                      int64
	ScopeType               string
	ScopeValue              string
	Enabled                 bool
	UserRoleWeight          float64
	DeviceCriticalityWeight float64
	NetworkAnomalyFactor    float64
	TrustedNetworks         []string
}

type PostgresContextPolicyProvider struct {
	pool *pgxpool.Pool

	mu        sync.RWMutex
	cacheTTL  time.Duration
	cachedAt  time.Time
	cachedSet []policyRecord
}

func NewPostgresContextPolicyProvider(pool *pgxpool.Pool, cacheTTL time.Duration) *PostgresContextPolicyProvider {
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Second
	}
	return &PostgresContextPolicyProvider{
		pool:     pool,
		cacheTTL: cacheTTL,
	}
}

func (p *PostgresContextPolicyProvider) Resolve(ctx context.Context, agentID, userName, sourceIP string) (ContextFactors, error) {
	rows, err := p.getPolicies(ctx)
	if err != nil {
		return DefaultContextFactors(), err
	}
	factors := DefaultContextFactors()
	var trustedNetworks []string

	userLower := strings.ToLower(strings.TrimSpace(userName))
	agentID = strings.TrimSpace(agentID)

	// Apply policies in deterministic precedence order:
	// global -> agent -> user (ordered in SQL), then stable by policy id.
	// This ensures reproducible scoring even with factor clamping.
	for _, r := range rows {
		if !r.Enabled {
			continue
		}
		match := false
		switch r.ScopeType {
		case "global":
			match = r.ScopeValue == "*" || r.ScopeValue == ""
		case "agent":
			match = agentID != "" && strings.EqualFold(r.ScopeValue, agentID)
		case "user":
			match = userLower != "" && strings.EqualFold(strings.TrimSpace(r.ScopeValue), userLower)
		}
		if !match {
			continue
		}
		factors.UserRoleWeight = clampFloat(factors.UserRoleWeight*r.UserRoleWeight, 0.5, 2.0)
		factors.DeviceCriticalityWeight = clampFloat(factors.DeviceCriticalityWeight*r.DeviceCriticalityWeight, 0.5, 2.0)
		factors.NetworkAnomalyFactor = clampFloat(factors.NetworkAnomalyFactor*r.NetworkAnomalyFactor, 0.5, 2.0)
		if len(r.TrustedNetworks) > 0 {
			trustedNetworks = append(trustedNetworks, r.TrustedNetworks...)
		}
	}

	trustedNetworks = uniqueStrings(trustedNetworks)

	if sourceIP != "" {
		if isIPInTrustedNetworks(sourceIP, trustedNetworks) {
			factors.NetworkAnomalyFactor = clampFloat(factors.NetworkAnomalyFactor*0.9, 0.5, 2.0)
		} else {
			factors.NetworkAnomalyFactor = clampFloat(factors.NetworkAnomalyFactor*1.1, 0.5, 2.0)
		}
	}

	return factors, nil
}

func (p *PostgresContextPolicyProvider) getPolicies(ctx context.Context) ([]policyRecord, error) {
	p.mu.RLock()
	if len(p.cachedSet) > 0 && time.Since(p.cachedAt) <= p.cacheTTL {
		out := make([]policyRecord, len(p.cachedSet))
		copy(out, p.cachedSet)
		p.mu.RUnlock()
		return out, nil
	}
	p.mu.RUnlock()

	rows, err := p.pool.Query(ctx, `
		SELECT id, scope_type, scope_value, enabled,
		       user_role_weight, device_criticality_weight, network_anomaly_factor,
		       trusted_networks
		FROM context_policies
		WHERE enabled = TRUE
		ORDER BY
			CASE scope_type
				WHEN 'global' THEN 0
				WHEN 'agent' THEN 1
				WHEN 'user' THEN 2
				ELSE 3
			END ASC,
			id ASC`)
	if err != nil {
		return nil, fmt.Errorf("context policy query failed: %w", err)
	}
	defer rows.Close()

	var out []policyRecord
	for rows.Next() {
		var r policyRecord
		var trustedJSON []byte
		if err := rows.Scan(
			&r.ID, &r.ScopeType, &r.ScopeValue, &r.Enabled,
			&r.UserRoleWeight, &r.DeviceCriticalityWeight, &r.NetworkAnomalyFactor,
			&trustedJSON,
		); err != nil {
			return nil, fmt.Errorf("context policy scan failed: %w", err)
		}
		if len(trustedJSON) > 0 {
			_ = json.Unmarshal(trustedJSON, &r.TrustedNetworks)
		}
		out = append(out, r)
	}

	p.mu.Lock()
	p.cachedSet = out
	p.cachedAt = time.Now()
	p.mu.Unlock()
	return out, nil
}

func isIPInTrustedNetworks(ipStr string, cidrs []string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(strings.TrimSpace(c))
		if err != nil {
			continue
		}
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		k := strings.TrimSpace(strings.ToLower(s))
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, s)
	}
	return out
}
