package analysis

import (
    "time"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

// Correlator matches anomaly candidates with events that overlap in time
// or occur within the preceding hour.

type Correlator interface { Correlate(candidates []models.AnomalyCandidate, events []models.Event) []models.EventCorrelation }

// simpleCorrelator implements a basic time‑window correlate.

type simpleCorrelator struct{}

func NewEventCorrelator() Correlator { return &simpleCorrelator{} }

func (c *simpleCorrelator) Correlate(candidates []models.AnomalyCandidate, events []models.Event) []models.EventCorrelation {
    var out []models.EventCorrelation
    for _, a := range candidates {
        var matched []models.Event
        for _, e := range events {
            if (a.Timestamp.After(e.Start) && a.Timestamp.Before(e.End)) ||
                (e.End.Before(a.Timestamp) && a.Timestamp.Sub(e.End) <= time.Hour) {
                matched = append(matched, e)
            }
        }
        out = append(out, models.EventCorrelation{Anomaly: a, Events: matched, Explains: len(matched) > 0})
    }
    return out
}
