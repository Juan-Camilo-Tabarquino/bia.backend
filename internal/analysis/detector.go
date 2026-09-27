package analysis

import (
	"fmt"
	"math"
	"sort"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// Detector identifies anomalies based on baseline statistics.
//
// The deterministic rules are:
//   - a consumption spike when a reading is more than three standard deviations
//     above the baseline mean or at least 45% above it;
//   - a consumption drop when a reading is at least 55% below the baseline mean,
//     which captures planned outages that the pure increase rule cannot see;
//   - a data-quality candidate when consumption stays close to the meter's own
//     hour-of-day median but the electrical readings are inconsistent (voltage
//     outside the nominal band or a power factor below a sane threshold).
//
// The data-quality rule is deliberately evaluated only for readings that are not
// already a consumption anomaly, so an elevated reading with a low power factor
// (a genuine electrical problem) is never downgraded to a data-quality issue.

type Detector interface {
	Detect(readings []models.Reading, baseline map[string]models.Baseline) []models.AnomalyCandidate
}

const (
	// spikeRatioThreshold is the relative increase over the baseline mean that
	// qualifies as a consumption spike.
	spikeRatioThreshold = 0.45
	// dropRatioThreshold is the relative decrease below the baseline mean that
	// qualifies as a consumption drop.
	dropRatioThreshold = 0.55
	// nearMedianTolerance is how far consumption may sit from the meter's
	// hour-of-day median and still count as "stable" for data-quality checks.
	nearMedianTolerance = 0.25
	// voltageMin and voltageMax bound a sane mains voltage band (220 V +/- 5%).
	voltageMin = 209.0
	voltageMax = 231.0
	// powerFactorMin is the lowest power factor considered healthy.
	powerFactorMin = 0.85
)

// anomalyDetector implements the rule-based detector described above.

type anomalyDetector struct{}

func NewAnomalyDetector() Detector { return &anomalyDetector{} }

func (d *anomalyDetector) Detect(readings []models.Reading, baseline map[string]models.Baseline) []models.AnomalyCandidate {
	hourlyMedian := hourlyConsumptionMedians(readings)
	var out []models.AnomalyCandidate
	for _, r := range readings {
		b, ok := baseline[string(r.MeterID)]
		if !ok || b.Mean == 0 {
			continue
		}
		delta := (r.Consumption - b.Mean) / b.Mean
		// candidate builds a fully enriched candidate for the current reading.
		// The detection rules stay unchanged: only the kind (and, for data
		// quality, the reason) differ between the branches below, while every
		// percentage is derived from the reading and the meter baseline.
		candidate := func(kind models.AnomalyKind, reason string) models.AnomalyCandidate {
			return models.AnomalyCandidate{
				MeterID:              r.MeterID,
				Timestamp:            r.Timestamp,
				Delta:                delta,
				Kind:                 kind,
				Reason:               reason,
				Raw:                  r,
				Baseline:             b,
				ConsumptionChangePct: delta * 100,
				VoltageChangePct:     signedChangePct(r.Voltage, b.VoltageMean),
				CurrentChangePct:     signedChangePct(r.Current, b.CurrentMean),
				PowerFactorChangePct: signedChangePct(r.PowerFactor, b.PowerFactorMean),
			}
		}
		switch {
		case r.Consumption > b.Mean+3*b.StdDev || delta >= spikeRatioThreshold:
			out = append(out, candidate(models.KindConsumptionSpike, ""))
		case -delta >= dropRatioThreshold:
			out = append(out, candidate(models.KindConsumptionDrop, ""))
		default:
			reason, inconsistent := electricalInconsistency(r)
			if inconsistent && consumptionNearHourMedian(hourlyMedian, r) {
				out = append(out, candidate(models.KindDataQuality, reason))
			}
		}
	}
	return out
}

// signedChangePct returns the signed relative change of value against mean, in
// percent.
//
// A zero baseline mean carries no scale, so the change is reported as exactly 0
// instead of NaN or +/-Inf. This is the documented division-by-zero rule for
// all per-signal changes and keeps the enriched payload finite and
// JSON-serialisable for signals whose baseline is absent (for example a meter
// that never reports a power factor).
func signedChangePct(value, mean float64) float64 {
	if mean == 0 {
		return 0
	}
	return (value - mean) / mean * 100
}

// hourlyConsumptionMedians returns, for every meter, the median consumption per
// hour of day. Using the median removes the strong diurnal cycle and keeps the
// data-quality check independent from the magnitude of the daily peak.
func hourlyConsumptionMedians(readings []models.Reading) map[models.MeterID]map[int]float64 {
	grouped := make(map[models.MeterID]map[int][]float64)
	for _, r := range readings {
		byHour := grouped[r.MeterID]
		if byHour == nil {
			byHour = make(map[int][]float64)
			grouped[r.MeterID] = byHour
		}
		hour := r.Timestamp.Hour()
		byHour[hour] = append(byHour[hour], r.Consumption)
	}
	out := make(map[models.MeterID]map[int]float64, len(grouped))
	for id, byHour := range grouped {
		medians := make(map[int]float64, len(byHour))
		for hour, values := range byHour {
			medians[hour] = median(values)
		}
		out[id] = medians
	}
	return out
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func consumptionNearHourMedian(medians map[models.MeterID]map[int]float64, r models.Reading) bool {
	byHour, ok := medians[r.MeterID]
	if !ok {
		return false
	}
	hourMedian, ok := byHour[r.Timestamp.Hour()]
	if !ok || hourMedian <= 0 {
		return false
	}
	return math.Abs(r.Consumption-hourMedian)/hourMedian <= nearMedianTolerance
}

// electricalInconsistency reports whether the electrical readings of a single
// measurement are outside a sane operating range. Zero values are treated as
// "not reported" so synthetic readings without electrical data stay clean.
func electricalInconsistency(r models.Reading) (string, bool) {
	if r.PowerFactor > 0 && r.PowerFactor < powerFactorMin {
		return fmt.Sprintf("factor de potencia %.3f por debajo de %.2f", r.PowerFactor, powerFactorMin), true
	}
	if r.Voltage > 0 && (r.Voltage < voltageMin || r.Voltage > voltageMax) {
		return fmt.Sprintf("tensión %.1f V fuera de [%.0f, %.0f] V", r.Voltage, voltageMin, voltageMax), true
	}
	return "", false
}
