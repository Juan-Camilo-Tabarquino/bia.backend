package analysis

import "github.com/neuralium/ai-energy/internal/domain/models"

// Detector identifies anomalies based on baseline statistics.
// A reading is considered anomalous if its consumption is greater than
// mean + 3*stdDev or the relative increase over the baseline mean is 45% or more.

type Detector interface {
    Detect(readings []models.Reading, baseline map[string]models.Baseline) []models.AnomalyCandidate
}

// anomalyDetector implements the simple rule‑based detector described above.

type anomalyDetector struct{}

func NewAnomalyDetector() Detector { return &anomalyDetector{} }

func (d *anomalyDetector) Detect(readings []models.Reading, baseline map[string]models.Baseline) []models.AnomalyCandidate {
    var out []models.AnomalyCandidate
    for _, r := range readings {
        b, ok := baseline[string(r.MeterID)]
        if !ok { continue }
        if r.Consumption > b.Mean+3*b.StdDev || (r.Consumption-b.Mean)/b.Mean >= 0.45 {
            out = append(out, models.AnomalyCandidate{MeterID: r.MeterID, Timestamp: r.Timestamp, Delta: r.Consumption - b.Mean, Raw: r})
        }
    }
    return out
}
