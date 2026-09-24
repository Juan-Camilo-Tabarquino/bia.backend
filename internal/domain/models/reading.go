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
}

// EventType enumerates known operational events.
type EventType string

const (
    EventProductionLine EventType = "PRODUCTION_LINE"
    EventMaintenance    EventType = "MAINTENANCE"
    EventShutdown       EventType = "SHUTDOWN"
)

// Event describes a known operational event.
type Event struct {
    ID          string
    Type        EventType
    Start       time.Time
    End         time.Time
    Description string
}

// AnomalyCandidate is a candidate derived by the detection engine.
type AnomalyCandidate struct {
    MeterID   MeterID
    Timestamp time.Time
    Delta     float64 // +percentage change
    Raw       Reading
}

// EventCorrelation records if the anomaly is explained.
type EventCorrelation struct {
    Anomaly   AnomalyCandidate
    Events    []Event
    Explains  bool
}

type Baseline struct {
    Mean   float64
    StdDev float64
    Count  int
}

type Evidence struct {
    Anomaly   AnomalyCandidate
    Correlation EventCorrelation
    Priority  int
    Explanation     string `json:"explanation,omitempty"`
    Recommendation string `json:"recommendation,omitempty"`
    LLMText        string `json:"llm_text,omitempty"`
    // Additional data can be added later
}
