package scoring_test

import (
	"testing"

	"github.com/edr-platform/sigma-engine/internal/application/scoring"
	"github.com/stretchr/testify/assert"
)

func TestRiskLevelFromScore_DefaultThresholds(t *testing.T) {
	cfg := scoring.DefaultRiskScoringConfig().RiskLevels

	assert.Equal(t, "low", scoring.RiskLevelFromScore(0, cfg))
	assert.Equal(t, "low", scoring.RiskLevelFromScore(39, cfg))
	assert.Equal(t, "medium", scoring.RiskLevelFromScore(40, cfg))
	assert.Equal(t, "medium", scoring.RiskLevelFromScore(69, cfg))
	assert.Equal(t, "high", scoring.RiskLevelFromScore(70, cfg))
	assert.Equal(t, "high", scoring.RiskLevelFromScore(89, cfg))
	assert.Equal(t, "critical", scoring.RiskLevelFromScore(90, cfg))
	assert.Equal(t, "critical", scoring.RiskLevelFromScore(100, cfg))
}

func TestRiskLevelFromScore_ClampsOutOfRange(t *testing.T) {
	cfg := scoring.DefaultRiskScoringConfig().RiskLevels
	assert.Equal(t, "low", scoring.RiskLevelFromScore(-10, cfg))
	assert.Equal(t, "critical", scoring.RiskLevelFromScore(999, cfg))
}

