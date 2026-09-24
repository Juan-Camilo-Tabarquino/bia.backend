package analysis

import (
    "math"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

// BaselineCalc defines an interface for computing baseline statistics per meter.
// It returns a map keyed by meter ID with the mean, standard deviation and count.

type BaselineCalc interface {
    Calculate(readings []models.Reading) map[string]models.Baseline
}

// baselineCalculator is a simple implementation that computes the mean and
// standard deviation for each meter. The standard deviation is calculated
// with Bessel's correction.

type baselineCalculator struct{}

func NewBaselineCalculator() BaselineCalc { return &baselineCalculator{} }

func (b *baselineCalculator) Calculate(readings []models.Reading) map[string]models.Baseline {
    bases := make(map[string]float64)
    counters := make(map[string]int)
    for _, r := range readings {
        bases[string(r.MeterID)] += r.Consumption
        counters[string(r.MeterID)]++
    }
    result := make(map[string]models.Baseline)
    for id, sum := range bases {
        cnt := counters[id]
        mean := sum / float64(cnt)
        varSum := 0.0
        for _, r := range readings {
            if string(r.MeterID) != id { continue }
            diff := r.Consumption - mean
            varSum += diff * diff
        }
        variance := varSum / float64(cnt-1)
        sd := math.Sqrt(variance)
        result[id] = models.Baseline{Mean: mean, StdDev: sd, Count: cnt}
    }
    return result
}
