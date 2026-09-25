package analysis

import (
    "testing"
    "time"

    "github.com/neuralium/ai-energy/internal/domain/models"
)

func TestBaselineCalculator(t *testing.T) {
    readings := []models.Reading{
        {MeterID: "M1", Timestamp: time.Now(), Consumption: 10},
        {MeterID: "M1", Timestamp: time.Now().Add(time.Hour), Consumption: 20},
        {MeterID: "M2", Timestamp: time.Now(), Consumption: 30},
        {MeterID: "M2", Timestamp: time.Now().Add(time.Hour), Consumption: 50},
    }
    calc := NewBaselineCalculator()
    baselines := calc.Calculate(readings)

    if len(baselines) != 2 {
        t.Fatalf("expected 2 baselines, got %d", len(baselines))
    }
    if b, ok := baselines["M1"]; !ok || b.Mean != 15 {
        t.Fatalf("expected mean 15 for M1, got %+v", b)
    }
    if b, ok := baselines["M2"]; !ok || b.Mean != 40 {
        t.Fatalf("expected mean 40 for M2, got %+v", b)
    }
}
