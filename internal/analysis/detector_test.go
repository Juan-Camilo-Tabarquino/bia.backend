package analysis

import (
    "testing"
    "time"

    "github.com/neuralium/ai-energy/internal/domain/models"
)

func TestAnomalyDetector(t *testing.T) {
    baseline := map[string]models.Baseline{
        "M1": {Mean: 10, StdDev: 2, Count: 2},
    }
    readings := []models.Reading{
        {MeterID: "M1", Timestamp: time.Now(), Consumption: 5},   // normal
        {MeterID: "M1", Timestamp: time.Now().Add(time.Hour), Consumption: 17}, // > mean+3*stddev (16) => anomaly
    }
    det := NewAnomalyDetector()
    anomalies := det.Detect(readings, baseline)
    if len(anomalies) != 1 {
        t.Fatalf("expected 1 anomaly, got %d", len(anomalies))
    }
    if anomalies[0].MeterID != "M1" {
        t.Fatalf("expected anomaly for M1, got %s", anomalies[0].MeterID)
    }
}
