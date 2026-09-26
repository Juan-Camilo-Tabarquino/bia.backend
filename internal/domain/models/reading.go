package models

import "time"

// MeterID is a string alias, for static typing.
type MeterID string

// Reading represents a single meter measurement.
type Reading struct {
	MeterID     MeterID
	Timestamp   time.Time
	Consumption float64 // kWh
	Voltage     float64
	Current     float64
	PowerFactor float64
	// Status is an optional textual indicator of the reading's quality, e.g., "OK".
	Status string `json:"status,omitempty"`
}

// EventType enumerates known operational events.
type EventType string

const (
	EventProductionLine EventType = "PRODUCTION_LINE"
	EventMaintenance    EventType = "MAINTENANCE"
	EventShutdown       EventType = "SHUTDOWN"

	// EventOperationalChange marks a change in how an installation is operated.
	EventOperationalChange EventType = "OPERATIONAL_CHANGE"
	// EventScheduledOutage marks a planned stop of the monitored installation.
	EventScheduledOutage EventType = "SCHEDULED_OUTAGE"
	// EventDataQuality marks a known measurement-quality problem.
	EventDataQuality EventType = "DATA_QUALITY"
	// EventUnknown marks an unspecified event, which never explains a deviation.
	EventUnknown EventType = "UNKNOWN"
)

// Event describes a known operational event.
type Event struct {
	ID          string
	Type        EventType
	Start       time.Time
	End         time.Time
	Description string
}

// AnomalyKind identifies which deterministic rule produced a candidate.
type AnomalyKind string

const (
	// KindConsumptionSpike is a consumption increase above the baseline.
	KindConsumptionSpike AnomalyKind = "CONSUMPTION_SPIKE"
	// KindConsumptionDrop is a consumption decrease below the baseline.
	KindConsumptionDrop AnomalyKind = "CONSUMPTION_DROP"
	// KindDataQuality is an electrical inconsistency with stable consumption.
	KindDataQuality AnomalyKind = "DATA_QUALITY"
)

// AnomalyCandidate is a candidate derived by the detection engine.
type AnomalyCandidate struct {
	MeterID   MeterID
	Timestamp time.Time
	Delta     float64 // signed relative change against the baseline mean (0.5 = +50%)
	Kind      AnomalyKind
	Reason    string // human-readable trigger detail, currently used for data quality
	Raw       Reading

	// Baseline is the per-meter statistics the candidate was compared against.
	// Carrying it on the candidate lets every consumer (API, LLM payloads)
	// report the exact baseline behind the classification instead of
	// re-deriving it, and keeps the detection rules unchanged.
	Baseline Baseline

	// Per-signal signed relative changes in percent against the meter's
	// baseline means. ConsumptionChangePct mirrors Delta*100 and keeps the
	// original Delta semantics; the electrical signals are computed from their
	// own baselines. A signal whose baseline mean is zero reports a 0 change
	// (never NaN or +/-Inf).
	ConsumptionChangePct float64
	VoltageChangePct     float64
	CurrentChangePct     float64
	PowerFactorChangePct float64
}

// EventCorrelation records if the anomaly is explained.
type EventCorrelation struct {
	Anomaly  AnomalyCandidate
	Events   []Event
	Explains bool
}

// Baseline holds the per-meter statistics the detector compares a reading
// against. Mean, StdDev and Count describe consumption and keep their original
// meaning (the standard deviation uses Bessel's correction). The voltage,
// current and power-factor means are computed over the same reading slice so
// every signal can be expressed relative to its own baseline.
type Baseline struct {
	Mean   float64
	StdDev float64
	Count  int

	VoltageMean     float64
	CurrentMean     float64
	PowerFactorMean float64
}

type Evidence struct {
	Anomaly        AnomalyCandidate
	Correlation    EventCorrelation
	Priority       int
	Explanation    string `json:"explanation,omitempty"`
	Recommendation string `json:"recommendation,omitempty"`
	LLMText        string `json:"llm_text,omitempty"`
	// Additional fields for anomaly classification.
	Type       AnomalyType   `json:"type,omitempty"`
	Severity   Severity      `json:"severity,omitempty"`
	Confidence float64       `json:"confidence,omitempty"`
	Status     AnomalyStatus `json:"status,omitempty"`
	// Additional data can be added later
}
