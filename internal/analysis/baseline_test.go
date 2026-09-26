package analysis

import (
	"math"
	"testing"
	"time"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

func TestBaselineCalculator(t *testing.T) {
	readings := []models.Reading{
		{MeterID: "M1", Timestamp: time.Now(), Consumption: 10, Voltage: 200, Current: 40, PowerFactor: 0.9},
		{MeterID: "M1", Timestamp: time.Now().Add(time.Hour), Consumption: 20, Voltage: 220, Current: 60, PowerFactor: 0.8},
		{MeterID: "M2", Timestamp: time.Now(), Consumption: 30, Voltage: 210, Current: 30, PowerFactor: 0.7},
		{MeterID: "M2", Timestamp: time.Now().Add(time.Hour), Consumption: 50, Voltage: 230, Current: 50, PowerFactor: 0.9},
	}
	calc := NewBaselineCalculator()
	baselines := calc.Calculate(readings)

	if len(baselines) != 2 {
		t.Fatalf("expected 2 baselines, got %d", len(baselines))
	}
	m1, ok := baselines["M1"]
	if !ok || m1.Mean != 15 {
		t.Fatalf("expected mean 15 for M1, got %+v", m1)
	}
	m2, ok := baselines["M2"]
	if !ok || m2.Mean != 40 {
		t.Fatalf("expected mean 40 for M2, got %+v", m2)
	}

	// The consumption statistics keep their original meaning, and the new
	// per-signal means cover the same reading slice.
	assertAlmostEqual(t, "M1 voltage mean", m1.VoltageMean, 210)
	assertAlmostEqual(t, "M1 current mean", m1.CurrentMean, 50)
	assertAlmostEqual(t, "M1 power factor mean", m1.PowerFactorMean, 0.85)
	assertAlmostEqual(t, "M2 voltage mean", m2.VoltageMean, 220)
	assertAlmostEqual(t, "M2 current mean", m2.CurrentMean, 40)
	assertAlmostEqual(t, "M2 power factor mean", m2.PowerFactorMean, 0.8)
}

// assertAlmostEqual compares two floats with a tolerance tight enough to catch
// real errors but immune to the binary representation of decimals.
func assertAlmostEqual(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}
