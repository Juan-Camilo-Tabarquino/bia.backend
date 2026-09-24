package services

import (
    "context"
    "github.com/neuralium/ai-energy/internal/domain/models"
    "math"
)

// BaselineCalculator computes mean ± 2σ per meter.
// The result is a map[MeterID]Baseline where Baseline holds Mean and StdDev.
// The algorithm: for each meter, collect all consumption values, compute mean, then compute standard deviation.

type Baseline struct {
    Mean    float64
    StdDev  float64
    Count   int
}

type BaselineCalculator interface {
    Calculate(ctx context.Context, readings []models.Reading) (map[models.MeterID]Baseline, error)
}

func NewStaticBaseline() *StaticBaseline {
    return &StaticBaseline{}
}

type StaticBaseline struct{}

func (sb *StaticBaseline) Calculate(ctx context.Context, readings []models.Reading) (map[models.MeterID]Baseline, error) {
    buckets := make(map[models.MeterID][]float64)
    for _, r := range readings {
        buckets[r.MeterID] = append(buckets[r.MeterID], r.Consumption)
    }
    res := make(map[models.MeterID]Baseline)
    for id, values := range buckets {
        n := len(values)
        if n == 0 {
            continue
        }
        sum := 0.0
        for _, v := range values {
            sum += v
        }
        mean := sum / float64(n)
        variance := 0.0
        for _, v := range values {
            diff := v - mean
            variance += diff * diff
        }
        stddev := math.Sqrt(variance / float64(n))
        res[id] = Baseline{Mean: mean, StdDev: stddev, Count: n}
    }
    return res, nil
}
