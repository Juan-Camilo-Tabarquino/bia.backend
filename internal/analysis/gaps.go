package analysis

import (
	"sort"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// InsufficientReadingsReason is the exact user-visible reason published for a
// meter that survived the quality check but has too few readings to validate.
const InsufficientReadingsReason = "no hay suficiente información para validar: se requieren al menos 2 lecturas"

// DataGap describes a meter that survived the quality check but could not be
// validated because it has too few readings for a baseline.
//
// A data gap is NOT an anomaly. It is never turned into a candidate, never
// classified and therefore never appears in the anomaly list: the meter is
// reported in the dashboard summary as an unvalidated meter instead, so the UI
// can ask for more data without inventing a deviation that was never measured.
type DataGap struct {
	MeterID  string
	Readings int
	Reason   string
}

// dataGapsFor returns, sorted by meter id, one DataGap per meter that survived
// the quality check but has no entry in the baseline map. A meter is absent from
// the baseline map precisely when it had fewer than minReadingsForBaseline
// readings, so this is the unvalidated-meter signal. Every produced gap carries
// InsufficientReadingsReason.
func dataGapsFor(good []models.Reading, baseline map[string]models.Baseline) []DataGap {
	counts := make(map[string]int)
	for _, r := range good {
		counts[string(r.MeterID)]++
	}
	gaps := make([]DataGap, 0, len(counts))
	for id, readings := range counts {
		if _, ok := baseline[id]; ok {
			continue
		}
		gaps = append(gaps, DataGap{
			MeterID:  id,
			Readings: readings,
			Reason:   InsufficientReadingsReason,
		})
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].MeterID < gaps[j].MeterID })
	return gaps
}
