package analysis

import (
	"math"
	"testing"
	"time"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

func TestAnomalyDetector(t *testing.T) {
	baseline := map[string]models.Baseline{
		"M1": {Mean: 10, StdDev: 2, Count: 2},
	}
	readings := []models.Reading{
		{MeterID: "M1", Timestamp: time.Now(), Consumption: 5},                 // normal
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

// TestDetectorPerSignalChanges proves the candidate carries the four signed
// per-signal changes in percent against the meter baseline, and that a zero
// baseline mean yields exactly 0 rather than NaN or +/-Inf.
func TestDetectorPerSignalChanges(t *testing.T) {
	baseline := map[string]models.Baseline{
		"M1": {
			Mean:            100,
			StdDev:          5,
			Count:           4,
			VoltageMean:     200,
			CurrentMean:     50,
			PowerFactorMean: 0.9,
		},
		// M2 exercises the division-by-zero rule for the electrical signals.
		"M2": {
			Mean:            100,
			StdDev:          5,
			Count:           4,
			VoltageMean:     0,
			CurrentMean:     0,
			PowerFactorMean: 0,
		},
	}
	readings := []models.Reading{
		{MeterID: "M1", Timestamp: time.Now(), Consumption: 200, Voltage: 190, Current: 60, PowerFactor: 0.81},
		{MeterID: "M2", Timestamp: time.Now(), Consumption: 200, Voltage: 190, Current: 60, PowerFactor: 0.81},
	}
	det := NewAnomalyDetector()
	anomalies := det.Detect(readings, baseline)
	if len(anomalies) != 2 {
		t.Fatalf("expected 2 anomalies, got %d", len(anomalies))
	}

	byMeter := make(map[models.MeterID]models.AnomalyCandidate, len(anomalies))
	for _, a := range anomalies {
		byMeter[a.MeterID] = a
	}

	m1 := byMeter["M1"]
	assertAlmostEqual(t, "M1 consumption change pct", m1.ConsumptionChangePct, 100)
	assertAlmostEqual(t, "M1 voltage change pct", m1.VoltageChangePct, -5)
	assertAlmostEqual(t, "M1 current change pct", m1.CurrentChangePct, 20)
	assertAlmostEqual(t, "M1 power factor change pct", m1.PowerFactorChangePct, -10)

	m2 := byMeter["M2"]
	for signal, value := range map[string]float64{
		"voltage":      m2.VoltageChangePct,
		"current":      m2.CurrentChangePct,
		"power factor": m2.PowerFactorChangePct,
	} {
		if value != 0 {
			t.Fatalf("zero baseline %s change = %v, want exactly 0", signal, value)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("zero baseline %s change must be finite, got %v", signal, value)
		}
	}
}
