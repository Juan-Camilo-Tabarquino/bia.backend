package services

import (
    "context"
    "github.com/neuralium/ai-energy/internal/domain/models"
    "time"
)

// EventCorrelator decides if an anomaly is explained by events.
// An event explains if it overlaps the anomaly timestamp or
// starts within the previous hour.
func NewStaticCorrelator() *StaticCorrelator {
    return &StaticCorrelator{}
}

type StaticCorrelator struct{}

func (ec *StaticCorrelator) Correlate(ctx context.Context, candidate models.AnomalyCandidate, events []models.Event) (models.EventCorrelation, error) {
    var matched []models.Event
    for _, e := range events {
        if (candidate.Timestamp.After(e.Start) || candidate.Timestamp.Equal(e.Start)) && candidate.Timestamp.Before(e.End) {
            matched = append(matched, e)
            continue
        }
        // previous hour window
        if candidate.Timestamp.Sub(e.End) <= time.Hour && candidate.Timestamp.After(e.End) {
            matched = append(matched, e)
        }
    }
    corr := models.EventCorrelation{Anomaly: candidate, Events: matched, Explains: len(matched) > 0}
    return corr, nil
}
