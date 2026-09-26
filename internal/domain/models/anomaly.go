package models

import "time"

// AnomalyType enumerates the deterministic anomaly categories required by the
// specification. The string values are part of the public API contract.
type AnomalyType string

const (
	// AnomalyReal is a genuine anomaly with no known explaining event.
	AnomalyReal AnomalyType = "REAL_ANOMALY"
	// AnomalyExplainable is a deviation explained by an operational event.
	AnomalyExplainable AnomalyType = "EXPLAINABLE_ANOMALY"
	// AnomalyFalsePositive is a deviation explained by planned activity.
	AnomalyFalsePositive AnomalyType = "FALSE_POSITIVE"
	// AnomalyDataQuality is an inconsistency in the measurements themselves.
	AnomalyDataQuality AnomalyType = "DATA_QUALITY"
)

// Severity is the operational severity assigned to an anomaly.
type Severity string

const (
	// SeverityLow is the lowest operational severity.
	SeverityLow Severity = "LOW"
	// SeverityMedium is the middle operational severity.
	SeverityMedium Severity = "MEDIUM"
	// SeverityHigh is the highest operational severity.
	SeverityHigh Severity = "HIGH"
)

// AnomalyStatus describes whether an anomaly is explained by events.
type AnomalyStatus string

const (
	// StatusExplained means an operational event explains the deviation.
	StatusExplained AnomalyStatus = "explained"
	// StatusUnexplained means no event explains the deviation.
	StatusUnexplained AnomalyStatus = "unexplained"
)

// Meter holds the descriptive metadata of an electrical meter.
type Meter struct {
	ID        string
	MeterID   string
	Name      string
	Location  string
	Status    string
	CreatedAt time.Time
}
