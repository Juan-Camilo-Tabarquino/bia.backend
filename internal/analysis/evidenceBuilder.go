package analysis

import (
	"fmt"
	"strings"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// EvidenceBuilder produces the final, JSON-ready evidence records. It keeps the
// deterministic classification and score produced by the previous stages and
// adds the human-readable explanation and the recommended action, so every
// evidence record travels fully populated through the API.

type EvidenceBuilder interface {
	Build(evidence []models.Evidence) []models.Evidence
}

func NewEvidenceBuilder() EvidenceBuilder { return &defaultEvidenceBuilder{} }

type defaultEvidenceBuilder struct{}

func (b *defaultEvidenceBuilder) Build(evidence []models.Evidence) []models.Evidence {
	out := make([]models.Evidence, 0, len(evidence))
	for _, e := range evidence {
		e.Explanation = describeEvidence(e)
		e.Recommendation = recommendAction(e.Type)
		out = append(out, e)
	}
	return out
}

func describeEvidence(e models.Evidence) string {
	meter := string(e.Anomaly.MeterID)
	percent := e.Anomaly.Delta * 100
	switch e.Type {
	case models.AnomalyDataQuality:
		if e.Anomaly.Reason != "" {
			return fmt.Sprintf("Meter %s shows inconsistent electrical readings (%s) while its consumption stays close to the hourly baseline.", meter, e.Anomaly.Reason)
		}
		return fmt.Sprintf("Meter %s shows inconsistent electrical readings while its consumption stays close to the hourly baseline.", meter)
	case models.AnomalyReal:
		return fmt.Sprintf("Meter %s consumption is %.1f%% above its baseline with no known operational event.", meter, percent)
	case models.AnomalyExplainable:
		return fmt.Sprintf("Meter %s consumption is %.1f%% above its baseline and matches a known operational change: %s.", meter, percent, eventDescription(e.Correlation.Events))
	case models.AnomalyFalsePositive:
		return fmt.Sprintf("Meter %s consumption deviation of %.1f%% is explained by planned maintenance: %s.", meter, percent, eventDescription(e.Correlation.Events))
	default:
		return fmt.Sprintf("Meter %s shows an unexplained deviation of %.1f%%.", meter, percent)
	}
}

func eventDescription(events []models.Event) string {
	if len(events) == 0 {
		return "no event recorded"
	}
	parts := make([]string, 0, len(events))
	for _, e := range events {
		if e.Description != "" {
			parts = append(parts, fmt.Sprintf("%s (%s)", e.Description, e.Type))
			continue
		}
		parts = append(parts, string(e.Type))
	}
	return strings.Join(parts, "; ")
}

func recommendAction(anomalyType models.AnomalyType) string {
	switch anomalyType {
	case models.AnomalyReal:
		return "Investigate the meter and its installation."
	case models.AnomalyDataQuality:
		return "Review sensor calibration and the data-quality pipeline for this meter."
	case models.AnomalyExplainable:
		return "No action required; the deviation matches a registered operational change."
	case models.AnomalyFalsePositive:
		return "No action required; the deviation is explained by planned maintenance."
	default:
		return "Review the reading before escalating."
	}
}
