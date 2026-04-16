package scoring

// RiskScoringConfig centralizes all tuning constants for the context-aware
// risk scoring algorithm. It is loaded from `sigma_engine_go/config/config.yaml`
// under the `risk_scoring:` section via the infrastructure config loader.
//
// IMPORTANT:
// - This config is intentionally deterministic (no randomness).
// - Defaults match the previously hardcoded constants so existing deployments
//   behave identically until you change YAML values.
type RiskScoringConfig struct {
	BaseScore     BaseScoreConfig     `yaml:"base_score"`
	Burst         BurstConfig         `yaml:"burst"`
	Privilege     PrivilegeConfig     `yaml:"privilege"`
	FalsePositive FalsePositiveConfig `yaml:"false_positive"`
	UEBA          UEBAConfig          `yaml:"ueba"`
	Interaction   InteractionConfig   `yaml:"interaction"`
	ContextPolicy ContextPolicyConfig `yaml:"context_policy"`
	Quality       QualityConfig       `yaml:"quality"`
	RiskLevels    RiskLevelsConfig    `yaml:"risk_levels"`
}

type BaseScoreConfig struct {
	Informational int `yaml:"informational"`
	Low           int `yaml:"low"`
	Medium        int `yaml:"medium"`
	High          int `yaml:"high"`
	Critical      int `yaml:"critical"`
	Unknown       int `yaml:"unknown"`

	// MatchCorrelationBonusPerRule is added per additional matched rule.
	MatchCorrelationBonusPerRule int `yaml:"match_correlation_bonus_per_rule"`
	MatchCorrelationBonusCap     int `yaml:"match_correlation_bonus_cap"`
}

type BurstConfig struct {
	// WindowSec is the burst counting window length in seconds.
	WindowSec int `yaml:"window_sec"`

	// Count thresholds in the sliding window.
	ThresholdLow  int64 `yaml:"threshold_low"`
	ThresholdMed  int64 `yaml:"threshold_med"`
	ThresholdHigh int64 `yaml:"threshold_high"`

	// Bonuses applied when thresholds are met.
	BonusLow  int `yaml:"bonus_low"`
	BonusMed  int `yaml:"bonus_med"`
	BonusHigh int `yaml:"bonus_high"`
}

type PrivilegeConfig struct {
	BonusSystemSID     int `yaml:"bonus_system_sid"`
	BonusAdminRID500   int `yaml:"bonus_admin_rid_500"`
	BonusIntegritySys  int `yaml:"bonus_integrity_system"`
	BonusHighElevated  int `yaml:"bonus_high_elevated"`
	BonusElevatedToken int `yaml:"bonus_elevated_token"`

	BonusUnsignedBinary int `yaml:"bonus_unsigned_binary"`
	BonusUnknownSig     int `yaml:"bonus_unknown_signature"`

	Cap int `yaml:"cap"`
}

type FalsePositiveConfig struct {
	DiscountMicrosoft            int `yaml:"discount_microsoft"`
	DiscountMicrosoftSystemPath  int `yaml:"discount_microsoft_system_path"`
	DiscountTrustedThirdParty    int `yaml:"discount_trusted_third_party"`
	DiscountCap                  int `yaml:"discount_cap"`
	RiskMicrosoftSystemPath      float64 `yaml:"risk_microsoft_system_path"`
	RiskMicrosoftOtherPath       float64 `yaml:"risk_microsoft_other_path"`
	RiskTrustedThirdParty        float64 `yaml:"risk_trusted_third_party"`
	RiskUnknownOrMissingSig      float64 `yaml:"risk_unknown_or_missing_signature"`
	RiskUnsigned                 float64 `yaml:"risk_unsigned"`
}

type UEBAConfig struct {
	ConfidenceGate float64 `yaml:"confidence_gate"`
	AnomalyBonus   int     `yaml:"anomaly_bonus"`
	NormalDiscount int     `yaml:"normal_discount"`
	ZScoreAnomalyThreshold float64 `yaml:"zscore_anomaly_threshold"`

	// FirstSeenHourAvgFloor controls the \"never seen at this hour\" heuristic.
	// If avg executions/hour is below this, treat it as first-seen for this hour.
	FirstSeenHourAvgFloor float64 `yaml:"first_seen_hour_avg_floor"`

	// NormalAvgThreshold is used for the stddev==0 case: if avg>=threshold,
	// we consider it normal and apply the normalcy discount.
	NormalAvgThreshold float64 `yaml:"normal_avg_threshold"`
}

type InteractionConfig struct {
	LineageHighThreshold   int `yaml:"lineage_high_threshold"`
	PrivilegeHighThreshold int `yaml:"privilege_high_threshold"`
	BurstHighThreshold     int `yaml:"burst_high_threshold"`

	BonusTwoSignals   int `yaml:"bonus_two_signals"`
	BonusThreeSignals int `yaml:"bonus_three_signals"`
	Cap              int `yaml:"cap"`
}

type ContextPolicyConfig struct {
	PerFactorClampMin float64 `yaml:"per_factor_clamp_min"`
	PerFactorClampMax float64 `yaml:"per_factor_clamp_max"`

	MultiplierClampMin float64 `yaml:"multiplier_clamp_min"`
	MultiplierClampMax float64 `yaml:"multiplier_clamp_max"`

	TrustedNetworkMultiplier   float64 `yaml:"trusted_network_multiplier"`
	UntrustedNetworkMultiplier float64 `yaml:"untrusted_network_multiplier"`
}

type QualityConfig struct {
	// Buckets map context_quality_score to a bounded multiplier.
	ScoreGE80 float64 `yaml:"score_ge_80"`
	ScoreGE60 float64 `yaml:"score_ge_60"`
	ScoreGE40 float64 `yaml:"score_ge_40"`
	ScoreGT0  float64 `yaml:"score_gt_0"`
	ScoreEQ0Missing float64 `yaml:"score_eq_0_missing"`
}

type RiskLevelsConfig struct {
	LowMax      int `yaml:"low_max"`
	MediumMax   int `yaml:"medium_max"`
	HighMax     int `yaml:"high_max"`
	CriticalMin int `yaml:"critical_min"`
}

func DefaultRiskScoringConfig() RiskScoringConfig {
	return RiskScoringConfig{
		BaseScore: BaseScoreConfig{
			Informational: 10,
			Low:           25,
			Medium:        45,
			High:          65,
			Critical:      85,
			Unknown:       35,
			MatchCorrelationBonusPerRule: 5,
			MatchCorrelationBonusCap:     15,
		},
		Burst: BurstConfig{
			WindowSec:     300,
			ThresholdLow:  3,
			ThresholdMed:  10,
			ThresholdHigh: 30,
			BonusLow:      10,
			BonusMed:      20,
			BonusHigh:     30,
		},
		Privilege: PrivilegeConfig{
			BonusSystemSID:     20,
			BonusAdminRID500:   15,
			BonusIntegritySys:  15,
			BonusHighElevated:  10,
			BonusElevatedToken: 10,
			BonusUnsignedBinary: 15,
			BonusUnknownSig:     8,
			Cap:                 40,
		},
		FalsePositive: FalsePositiveConfig{
			DiscountMicrosoft:           15,
			DiscountMicrosoftSystemPath: 10,
			DiscountTrustedThirdParty:   8,
			DiscountCap:                 30,
			RiskMicrosoftSystemPath:     0.70,
			RiskMicrosoftOtherPath:      0.45,
			RiskTrustedThirdParty:       0.30,
			RiskUnknownOrMissingSig:     0.15,
			RiskUnsigned:                0.05,
		},
		UEBA: UEBAConfig{
			ConfidenceGate:         0.30,
			AnomalyBonus:           15,
			NormalDiscount:         10,
			ZScoreAnomalyThreshold: 3.0,
			FirstSeenHourAvgFloor:  0.05,
			NormalAvgThreshold:     0.5,
		},
		Interaction: InteractionConfig{
			LineageHighThreshold:   20,
			PrivilegeHighThreshold: 15,
			BurstHighThreshold:     20,
			BonusTwoSignals:        10,
			BonusThreeSignals:      15,
			Cap:                    15,
		},
		ContextPolicy: ContextPolicyConfig{
			PerFactorClampMin: 0.5,
			PerFactorClampMax: 2.0,
			MultiplierClampMin: 0.25,
			MultiplierClampMax: 4.0,
			TrustedNetworkMultiplier:   0.9,
			UntrustedNetworkMultiplier: 1.1,
		},
		Quality: QualityConfig{
			ScoreGE80:       1.00,
			ScoreGE60:       0.97,
			ScoreGE40:       0.93,
			ScoreGT0:        0.90,
			ScoreEQ0Missing: 0.85,
		},
		RiskLevels: RiskLevelsConfig{
			LowMax:      39,
			MediumMax:   69,
			HighMax:     89,
			CriticalMin: 90,
		},
	}
}

func (c *RiskScoringConfig) ValidateAndSetDefaults() {
	// If the config section is absent from YAML, this struct will be zero-value.
	// Fill it with defaults to preserve backwards-compatible behavior.
	def := DefaultRiskScoringConfig()

	// BaseScore: treat 0 as unset (we never intentionally configure 0).
	if c.BaseScore.Informational == 0 {
		*c = def
		return
	}

	// ContextPolicy safe bounds (avoid division-by-zero style extremes).
	if c.ContextPolicy.PerFactorClampMin <= 0 {
		c.ContextPolicy.PerFactorClampMin = def.ContextPolicy.PerFactorClampMin
	}
	if c.ContextPolicy.PerFactorClampMax <= 0 {
		c.ContextPolicy.PerFactorClampMax = def.ContextPolicy.PerFactorClampMax
	}
	if c.ContextPolicy.MultiplierClampMin <= 0 {
		c.ContextPolicy.MultiplierClampMin = def.ContextPolicy.MultiplierClampMin
	}
	if c.ContextPolicy.MultiplierClampMax <= 0 {
		c.ContextPolicy.MultiplierClampMax = def.ContextPolicy.MultiplierClampMax
	}
	if c.ContextPolicy.TrustedNetworkMultiplier <= 0 {
		c.ContextPolicy.TrustedNetworkMultiplier = def.ContextPolicy.TrustedNetworkMultiplier
	}
	if c.ContextPolicy.UntrustedNetworkMultiplier <= 0 {
		c.ContextPolicy.UntrustedNetworkMultiplier = def.ContextPolicy.UntrustedNetworkMultiplier
	}
}

