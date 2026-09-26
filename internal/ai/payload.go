package ai

import (
	"encoding/json"
	"math"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// AnomalyPayload is the enriched statistical summary of a single anomaly that
// is handed to the LLM. The JSON keys are a fixed external contract: prompts
// and downstream consumers depend on them, so they must not be renamed.
//
// The four change percentages are signed relative changes against the meter's
// own baseline means, rounded to one decimal place. Rounding keeps the
// serialised object matching its documented shape (for example 103.7 instead of
// 103.70000000000002) and free of floating-point noise.
type AnomalyPayload struct {
	MeterID              string  `json:"meter_id"`
	ConsumptionChangePct float64 `json:"consumption_change_pct"`
	VoltageChangePct     float64 `json:"voltage_change_pct"`
	CurrentChangePct     float64 `json:"current_change_pct"`
	PowerFactorChangePct float64 `json:"power_factor_change_pct"`
	HasOperationalEvent  bool    `json:"has_operational_event"`
	AnomalyScore         float64 `json:"anomaly_score"`
	Classification       string  `json:"classification"`
	Severity             string  `json:"severity"`
}

// BuildAnomalyPayload maps a fully populated evidence record onto the payload
// the LLM consumes. Every field is read from the evidence: the meter id and the
// four per-signal changes come from the anomaly candidate, the score is the
// evidence confidence, and the classification and severity keep their public
// string values.
//
// HasOperationalEvent is the explaining-event flag, not merely "an event
// overlapped": it is true only when an *operational* event (production line,
// maintenance, shutdown, operational change or scheduled outage) is the event
// that explains the anomaly. A DATA_QUALITY event explains a data-quality
// finding but is not an operational event, and an UNKNOWN event never explains
// anything, so both keep the flag false.
func BuildAnomalyPayload(e models.Evidence) AnomalyPayload {
	return AnomalyPayload{
		MeterID:              string(e.Anomaly.MeterID),
		ConsumptionChangePct: roundToTenth(e.Anomaly.ConsumptionChangePct),
		VoltageChangePct:     roundToTenth(e.Anomaly.VoltageChangePct),
		CurrentChangePct:     roundToTenth(e.Anomaly.CurrentChangePct),
		PowerFactorChangePct: roundToTenth(e.Anomaly.PowerFactorChangePct),
		HasOperationalEvent:  hasOperationalEvent(e.Correlation),
		AnomalyScore:         e.Confidence,
		Classification:       string(e.Type),
		Severity:             string(e.Severity),
	}
}

// JSON serialises the payload to the exact JSON object sent to the LLM.
func (p AnomalyPayload) JSON() ([]byte, error) {
	return json.Marshal(p)
}

// roundToTenth rounds a percentage to one decimal place.
func roundToTenth(v float64) float64 {
	return math.Round(v*10) / 10
}

// hasOperationalEvent reports whether an operational event explains the anomaly.
// Only events that actually explain a deviation are considered, so an UNKNOWN
// event that merely overlaps the reading never sets the flag, and a
// DATA_QUALITY event does not count as operational.
func hasOperationalEvent(correlation models.EventCorrelation) bool {
	if !correlation.Explains {
		return false
	}
	for _, event := range correlation.Events {
		switch event.Type {
		case models.EventProductionLine,
			models.EventMaintenance,
			models.EventShutdown,
			models.EventOperationalChange,
			models.EventScheduledOutage:
			return true
		}
	}
	return false
}
