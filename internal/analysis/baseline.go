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
//
// Consumption stays the reference statistics: Mean, StdDev and Count are
// computed exactly as before and keep their meaning. The voltage, current and
// power-factor means are derived from the same reading slice, so the detector
// can express each electrical signal as a signed change against its own
// baseline.

type baselineCalculator struct{}

func NewBaselineCalculator() BaselineCalc { return &baselineCalculator{} }

// meterAccumulator holds the running sums needed to derive a meter's baseline
// in a single pass over the readings.
type meterAccumulator struct {
	count       int
	consumption float64
	voltage     float64
	current     float64
	powerFactor float64
}

func (b *baselineCalculator) Calculate(readings []models.Reading) map[string]models.Baseline {
	accumulators := make(map[string]*meterAccumulator)
	for _, r := range readings {
		id := string(r.MeterID)
		a := accumulators[id]
		if a == nil {
			a = &meterAccumulator{}
			accumulators[id] = a
		}
		a.count++
		a.consumption += r.Consumption
		a.voltage += r.Voltage
		a.current += r.Current
		a.powerFactor += r.PowerFactor
	}
	result := make(map[string]models.Baseline)
	for id, a := range accumulators {
		cnt := a.count
		mean := a.consumption / float64(cnt)
		varSum := 0.0
		for _, r := range readings {
			if string(r.MeterID) != id {
				continue
			}
			diff := r.Consumption - mean
			varSum += diff * diff
		}
		variance := varSum / float64(cnt-1)
		sd := math.Sqrt(variance)
		result[id] = models.Baseline{
			Mean:            mean,
			StdDev:          sd,
			Count:           cnt,
			VoltageMean:     a.voltage / float64(cnt),
			CurrentMean:     a.current / float64(cnt),
			PowerFactorMean: a.powerFactor / float64(cnt),
		}
	}
	return result
}
