package analysis

import "github.com/neuralium/ai-energy/internal/domain/models"

// Scorer computes the operational Severity, a Confidence score and an
// investigation Priority for every classified evidence. It replaces the former
// identity implementation so the API exposes prioritised, risk-ranked
// anomalies instead of raw correlation records.

type Scorer interface {
	Score(evidence []models.Evidence) []models.Evidence
}

type ruleScorer struct{}

func NewScorer() Scorer { return &ruleScorer{} }

func (s *ruleScorer) Score(evidence []models.Evidence) []models.Evidence {
	scored := make([]models.Evidence, 0, len(evidence))
	for _, e := range evidence {
		e.Severity = severityFor(e.Type)
		e.Confidence = confidenceFor(e)
		e.Priority = priorityFor(e.Type)
		if e.Status == "" {
			e.Status = models.StatusUnexplained
		}
		scored = append(scored, e)
	}
	return scored
}

func severityFor(anomalyType models.AnomalyType) models.Severity {
	switch anomalyType {
	case models.AnomalyReal, models.AnomalyDataQuality:
		return models.SeverityHigh
	case models.AnomalyExplainable:
		return models.SeverityMedium
	case models.AnomalyFalsePositive:
		return models.SeverityLow
	default:
		return models.SeverityLow
	}
}

// priorityFor returns 1 for the most urgent evidence; lower numbers are
// investigated first.
func priorityFor(anomalyType models.AnomalyType) int {
	switch anomalyType {
	case models.AnomalyReal:
		return 1
	case models.AnomalyDataQuality:
		return 2
	case models.AnomalyExplainable:
		return 3
	case models.AnomalyFalsePositive:
		return 4
	default:
		return 5
	}
}

func confidenceFor(e models.Evidence) float64 {
	// Magnitude is the relative deviation, capped so a single extreme reading
	// cannot drive confidence to certainty.
	magnitude := abs(e.Anomaly.Delta)
	if magnitude > 1.2 {
		magnitude = 1.2
	}
	var confidence float64
	switch e.Type {
	case models.AnomalyReal:
		confidence = 0.85 + 0.10*magnitude
	case models.AnomalyDataQuality:
		confidence = 0.90
	case models.AnomalyExplainable:
		confidence = 0.80 + 0.05*magnitude
	case models.AnomalyFalsePositive:
		confidence = 0.75 + 0.05*magnitude
	default:
		confidence = 0.50
	}
	return clamp01(confidence)
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
