package services

import (
    "context"
    "sort"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

// AnomalyDetector flags readings that deviate from baseline or exhibit sudden jumps.
// A reading is flagged if (consumption > mean + 3σ) OR (delta >= 45% compared to previous reading for same meter).
// Timestamp order is assumed ascending.

type AnomalyDetector interface {
    Detect(ctx context.Context, readings []models.Reading, baseline map[models.MeterID]Baseline) []models.AnomalyCandidate
}

func NewStaticDetector() *StaticDetector {
    return &StaticDetector{}
}

type StaticDetector struct{}

func (ad *StaticDetector) Detect(ctx context.Context, readings []models.Reading, baseline map[models.MeterID]Baseline) []models.AnomalyCandidate {
    // Build a map of previous reading per meter
    prev := make(map[models.MeterID]models.Reading)
    var result []models.AnomalyCandidate
    sort.Slice(readings, func(i, j int) bool { return readings[i].Timestamp.Before(readings[j].Timestamp) })
    for _, r := range readings {
        b, ok := baseline[r.MeterID]
        if !ok {
            continue
        }
        // 1 – baseline violation
        if r.Consumption > b.Mean+3*b.StdDev {
            cand := models.AnomalyCandidate{MeterID: r.MeterID, Timestamp: r.Timestamp, Delta: 0, Raw: r}
            result = append(result, cand)
        }
        // 2 – sudden jump vs previous
        if p, exists := prev[r.MeterID]; exists {
            if p.Consumption > 0 {
                delta := (r.Consumption - p.Consumption) / p.Consumption
                if delta >= 0.45 {
                    cand := models.AnomalyCandidate{MeterID: r.MeterID, Timestamp: r.Timestamp, Delta: delta, Raw: r}
                    result = append(result, cand)
                }
            }
        }
        prev[r.MeterID] = r
    }
    return result
}
