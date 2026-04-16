package scoring

// RiskLevelFromScore maps a final_score (0..100) into a discrete tier used for
// triage and UI consistency. This is intentionally separate from Sigma rule
// severity (which is static).
//
// Defaults match the dashboard tiering:
// - critical: >= 90
// - high:     70-89
// - medium:   40-69
// - low:      < 40
func RiskLevelFromScore(score int, cfg RiskLevelsConfig) string {
	def := DefaultRiskScoringConfig().RiskLevels
	if cfg.CriticalMin == 0 && cfg.HighMax == 0 && cfg.MediumMax == 0 && cfg.LowMax == 0 {
		cfg = def
	}

	// Defensive: clamp score to avoid weird negative/overflow inputs.
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	if score >= cfg.CriticalMin {
		return "critical"
	}
	if score <= cfg.LowMax {
		return "low"
	}
	if score <= cfg.MediumMax {
		return "medium"
	}
	if score <= cfg.HighMax {
		return "high"
	}
	// If thresholds are misconfigured, fallback to high for safety.
	return "high"
}

