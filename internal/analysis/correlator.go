package analysis

import (
	"time"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// Correlator matches anomaly candidates with operational events.
//
// A candidate correlates with an event when it happens within a short window
// before the event or during the following correlationHorizon. A correlation
// only counts as an explanation when at least one matched event has an
// explaining type; an UNKNOWN event is never proof that a deviation is
// explained.

type Correlator interface {
	Correlate(candidates []models.AnomalyCandidate, events []models.Event) []models.EventCorrelation
}

const (
	// correlationLeadTime allows a candidate that slightly precedes an event.
	correlationLeadTime = time.Hour
	// correlationHorizon covers the following hours of a sustained change.
	correlationHorizon = 12 * time.Hour
)

// simpleCorrelator implements a time-window correlation.

type simpleCorrelator struct{}

func NewEventCorrelator() Correlator { return &simpleCorrelator{} }

func (c *simpleCorrelator) Correlate(candidates []models.AnomalyCandidate, events []models.Event) []models.EventCorrelation {
	// Events loaded from the simplified CSV schema carry the meter id in
	// their ID field. When an event ID identifies one of the known meters it is
	// scoped to that meter only; otherwise it falls back to time-window matching.
	knownMeters := make(map[string]bool, len(candidates))
	for _, a := range candidates {
		knownMeters[string(a.MeterID)] = true
	}
	out := make([]models.EventCorrelation, 0, len(candidates))
	for _, a := range candidates {
		var matched []models.Event
		for _, e := range events {
			if eventAppliesTo(e, a, knownMeters) {
				matched = append(matched, e)
			}
		}
		out = append(out, models.EventCorrelation{Anomaly: a, Events: matched, Explains: anyExplainsDeviation(matched)})
	}
	return out
}

func eventAppliesTo(e models.Event, a models.AnomalyCandidate, knownMeters map[string]bool) bool {
	if knownMeters[e.ID] {
		// Meter-scoped event: it can only explain its own meter's deviation.
		return e.ID == string(a.MeterID) && eventOverlaps(e, a.Timestamp)
	}
	return eventOverlaps(e, a.Timestamp)
}

func eventOverlaps(e models.Event, ts time.Time) bool {
	end := e.End
	if end.Before(e.Start) {
		end = e.Start
	}
	start := e.Start.Add(-correlationLeadTime)
	finish := end.Add(correlationHorizon)
	return !ts.Before(start) && !ts.After(finish)
}

func anyExplainsDeviation(events []models.Event) bool {
	for _, e := range events {
		if explainsDeviation(e.Type) {
			return true
		}
	}
	return false
}

// explainsDeviation reports whether an event type is a valid explanation for a
// consumption deviation. EventUnknown is explicitly not an explanation.
func explainsDeviation(eventType models.EventType) bool {
	switch eventType {
	case models.EventScheduledOutage,
		models.EventMaintenance,
		models.EventShutdown,
		models.EventOperationalChange,
		models.EventProductionLine,
		models.EventDataQuality:
		return true
	default:
		return false
	}
}
