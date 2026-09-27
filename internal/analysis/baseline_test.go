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

// TestBaselineCalculatorRequiresTwoReadings pins the minimum-readings rule: the
// standard deviation uses Bessel's correction, so its denominator is (n-1) and a
// single reading has no variance to divide. Such a meter must get NO baseline
// entry at all, instead of an entry whose StdDev is NaN, and every baseline that
// IS produced must be finite.
func TestBaselineCalculatorRequiresTwoReadings(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	readings := []models.Reading{
		// Exactly one reading: the variance denominator would be zero.
		{MeterID: "ONE", Timestamp: base, Consumption: 12, Voltage: 240, Current: 45, PowerFactor: 0.95},
		// Two readings: the smallest sample that can produce a baseline.
		{MeterID: "TWO", Timestamp: base, Consumption: 10, Voltage: 220, Current: 40, PowerFactor: 0.9},
		{MeterID: "TWO", Timestamp: base.Add(time.Hour), Consumption: 20, Voltage: 220, Current: 60, PowerFactor: 0.95},
	}

	baselines := NewBaselineCalculator().Calculate(readings)

	if got, ok := baselines["ONE"]; ok {
		t.Fatalf("a meter with a single reading must get no baseline, got %+v", got)
	}
	// A meter with zero readings has no accumulator at all, so it must not
	// appear in the result either.
	if got, ok := baselines["ZERO"]; ok {
		t.Fatalf("a meter with zero readings must get no baseline, got %+v", got)
	}
	if len(baselines) != 1 {
		t.Fatalf("expected exactly 1 baseline (TWO), got %d: %+v", len(baselines), baselines)
	}
	two, ok := baselines["TWO"]
	if !ok {
		t.Fatalf("expected a baseline for TWO, got %+v", baselines)
	}
	assertAlmostEqual(t, "TWO mean", two.Mean, 15)

	for id, b := range baselines {
		if math.IsNaN(b.Mean) || math.IsInf(b.Mean, 0) {
			t.Errorf("meter %s: Mean must be finite, got %v", id, b.Mean)
		}
		if math.IsNaN(b.StdDev) || math.IsInf(b.StdDev, 0) {
			t.Errorf("meter %s: StdDev must be finite, got %v", id, b.StdDev)
		}
	}
}

// assertAlmostEqual compares two floats with a tolerance tight enough to catch
// real errors but immune to the binary representation of decimals.
func assertAlmostEqual(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}
