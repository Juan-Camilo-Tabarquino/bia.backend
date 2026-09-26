package analysis

import "github.com/neuralium/ai-energy/internal/domain/models"

// Classifier turns per-candidate correlations into one classified Evidence per
// meter and anomaly kind. It assigns the deterministic AnomalyType and the
// explained/unexplained status; Severity, Confidence and Priority are added
// later by the Scorer.
//
// The mapping follows the specification:
//   - a data-quality candidate or a DATA_QUALITY event -> DATA_QUALITY;
//   - a SCHEDULED_OUTAGE or MAINTENANCE event -> FALSE_POSITIVE;
//   - an OPERATIONAL_CHANGE or PRODUCTION_LINE event -> EXPLAINABLE_ANOMALY;
//   - an UNKNOWN event or no event at all -> REAL_ANOMALY.

type Classifier interface {
	Classify(correlations []models.EventCorrelation) []models.Evidence
}

type simpleClassifier struct{}

func NewClassifier() Classifier { return &simpleClassifier{} }

// correlationGroup merges every candidate of the same meter and kind so a single
// sustained anomaly produces one evidence record instead of one per reading.
type correlationGroup struct {
	representative models.AnomalyCandidate
	events         []models.Event
	explains       bool
}

func (cl *simpleClassifier) Classify(correlations []models.EventCorrelation) []models.Evidence {
	groups := groupCorrelations(correlations)
	out := make([]models.Evidence, 0, len(groups))
	for _, g := range groups {
		anomalyType := classifyType(g)
		out = append(out, models.Evidence{
			Anomaly: g.representative,
			Correlation: models.EventCorrelation{
				Anomaly:  g.representative,
				Events:   g.events,
				Explains: g.explains,
			},
			Type:   anomalyType,
			Status: classifyStatus(anomalyType, g.explains),
		})
	}
	return out
}

func groupCorrelations(correlations []models.EventCorrelation) []correlationGroup {
	index := make(map[string]int)
	groups := make([]correlationGroup, 0, len(correlations))
	for _, c := range correlations {
		key := string(c.Anomaly.MeterID) + "|" + string(c.Anomaly.Kind)
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, correlationGroup{representative: c.Anomaly})
		}
		g := &groups[i]
		if abs(c.Anomaly.Delta) > abs(g.representative.Delta) {
			g.representative = c.Anomaly
		}
		g.events = appendUniqueEvents(g.events, c.Events)
		if c.Explains {
			g.explains = true
		}
	}
	return groups
}

func classifyType(g correlationGroup) models.AnomalyType {
	if g.representative.Kind == models.KindDataQuality ||
		hasEventType(g.events, models.EventDataQuality) {
		return models.AnomalyDataQuality
	}
	if hasEventType(g.events, models.EventScheduledOutage, models.EventMaintenance, models.EventShutdown) {
		return models.AnomalyFalsePositive
	}
	if hasEventType(g.events, models.EventOperationalChange, models.EventProductionLine) {
		return models.AnomalyExplainable
	}
	// UNKNOWN events and the absence of events both leave a real deviation.
	return models.AnomalyReal
}

func classifyStatus(anomalyType models.AnomalyType, explains bool) models.AnomalyStatus {
	if explains &&
		(anomalyType == models.AnomalyExplainable ||
			anomalyType == models.AnomalyFalsePositive ||
			anomalyType == models.AnomalyDataQuality) {
		return models.StatusExplained
	}
	return models.StatusUnexplained
}

func appendUniqueEvents(dst, src []models.Event) []models.Event {
	for _, candidate := range src {
		duplicate := false
		for _, existing := range dst {
			if existing.ID == candidate.ID &&
				existing.Type == candidate.Type &&
				existing.Start.Equal(candidate.Start) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst = append(dst, candidate)
		}
	}
	return dst
}

func hasEventType(events []models.Event, types ...models.EventType) bool {
	for _, e := range events {
		for _, t := range types {
			if e.Type == t {
				return true
			}
		}
	}
	return false
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
