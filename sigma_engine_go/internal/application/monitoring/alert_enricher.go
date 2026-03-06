package monitoring

import (
	"github.com/edr-platform/sigma-engine/internal/domain"
)

// AlertEnricher enriches alerts with event counting statistics,
// adjusts confidence scores, and determines escalation conditions.
type AlertEnricher struct {
	eventCounter *EventCounter

	// Escalation thresholds
	countThreshold           int
	rateThresholdPerMin      float64
	enableCriticalEscalation bool
}

// NewAlertEnricher creates a new alert enricher.
// Parameters:
//   - eventCounter: Event counter for statistics
//   - countThreshold: Escalate if count > this (default: 100)
//   - rateThresholdPerMin: Escalate if rate > this (default: 10.0)
//   - enableCriticalEscalation: Escalate all critical severity alerts (default: true)
func NewAlertEnricher(
	eventCounter *EventCounter,
	countThreshold int,
	rateThresholdPerMin float64,
	enableCriticalEscalation bool,
) *AlertEnricher {
	if countThreshold <= 0 {
		countThreshold = 100
	}
	if rateThresholdPerMin <= 0 {
		rateThresholdPerMin = 10.0
	}

	return &AlertEnricher{
		eventCounter:             eventCounter,
		countThreshold:           countThreshold,
		rateThresholdPerMin:     rateThresholdPerMin,
		enableCriticalEscalation: enableCriticalEscalation,
	}
}

// EnrichAlert enriches an alert with event statistics and escalation logic.
// Takes a base Alert and returns an EnhancedAlert with:
//   - Event counting statistics
//   - Adjusted confidence score
//   - Escalation flags and reasons
func (ae *AlertEnricher) EnrichAlert(alert *domain.Alert, event *domain.LogEvent, sourceFile string) *domain.EnhancedAlert {
	if alert == nil {
		return nil
	}

	// Create enhanced alert from base alert
	enhanced := domain.NewEnhancedAlert(alert)
	if enhanced == nil {
		return nil
	}

	// Set source file
	enhanced.SetSourceFile(sourceFile)

	// Generate signature for event
	signature := ae.eventCounter.RecordEvent(event)

	// Get statistics
	stats, exists := ae.eventCounter.GetStatistics(signature)
	if !exists {
		// No statistics yet, return basic enhanced alert
		return enhanced
	}

	// Update statistics
	enhanced.UpdateStatistics(
		stats.EventCount,
		stats.FirstSeen,
		stats.LastSeen,
		stats.RatePerMinute,
		stats.CountTrend,
		stats.WindowSize,
	)

	// Adjust confidence based on statistics
	enhanced.AdjustConfidence()

	// Check escalation conditions
	enhanced.CheckEscalation(
		ae.countThreshold,
		ae.rateThresholdPerMin,
		ae.enableCriticalEscalation,
	)

	return enhanced
}

// EnrichAlerts enriches multiple alerts.
func (ae *AlertEnricher) EnrichAlerts(
	alerts []*domain.Alert,
	event *domain.LogEvent,
	sourceFile string,
) []*domain.EnhancedAlert {
	if len(alerts) == 0 {
		return nil
	}

	enhanced := make([]*domain.EnhancedAlert, 0, len(alerts))
	for _, alert := range alerts {
		if enriched := ae.EnrichAlert(alert, event, sourceFile); enriched != nil {
			enhanced = append(enhanced, enriched)
		}
	}

	return enhanced
}

