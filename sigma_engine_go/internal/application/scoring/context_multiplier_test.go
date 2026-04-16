package scoring_test

import (
	"testing"

	"github.com/edr-platform/sigma-engine/internal/application/scoring"
	"github.com/stretchr/testify/assert"
)

func TestContextFactors_Multiplier_UsesConfigClamp(t *testing.T) {
	f := scoring.ContextFactors{
		UserRoleWeight:          10,
		DeviceCriticalityWeight: 10,
		NetworkAnomalyFactor:    10,
	}
	cfg := scoring.ContextPolicyConfig{
		MultiplierClampMin: 0.25,
		MultiplierClampMax: 4.0,
		PerFactorClampMin:  0.5,
		PerFactorClampMax:  2.0,
		TrustedNetworkMultiplier:   0.9,
		UntrustedNetworkMultiplier: 1.1,
	}
	assert.Equal(t, 4.0, f.Multiplier(cfg), "multiplier should clamp at configured max")
}

