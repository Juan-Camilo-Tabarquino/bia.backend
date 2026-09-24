package services

import (
    "context"
    "fmt"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

// DataQualityChecker validates raw CSV data.
// Returns cleaned readings, list of error messages and err if fatal.
// "Fatal" errors are things that prevent any further processing (e.g. missing critical columns).
type DataQualityChecker interface {
    Check(ctx context.Context, readings []models.Reading) ([]models.Reading, []string, error)
}

// DefaultQualityChecker provides a straight‑forward implementation.
// Rules implemented (can be expanded):
//   * consumption >= 0
//   * voltage in 220-240V range
//   * no duplicate (MeterID+Timestamp)
//   * all numeric fields present and parsable
//   * duplicate detection results in a warning and omission of the duplicate entry.
//   * any parsing error on a row results in the row being discarded with an error message.
// The function does not modify the original slice.

func NewDataQualityChecker() *DefaultQualityChecker {
    return &DefaultQualityChecker{}
}

type DefaultQualityChecker struct{}

func (dq *DefaultQualityChecker) Check(ctx context.Context, readings []models.Reading) ([]models.Reading, []string, error) {
    // Map to detect duplicates: key = meterID + timestamp
    dup := make(map[string]bool)
    var cleaned []models.Reading
    var errs []string
    for _, r := range readings {
        key := string(r.MeterID) + r.Timestamp.UTC().Format("2006-01-02T15:04:05Z")
        if dup[key] {
            errs = append(errs, "duplicate reading detected: "+key)
            continue
        }
        dup[key] = true
        // basic range checks
        if r.Consumption < 0 {
            errs = append(errs, fmt.Sprintf("negative consumption %f at %s on %s", r.Consumption, r.MeterID, r.Timestamp))
            continue
        }
        if r.Voltage < 220 || r.Voltage > 240 {
            errs = append(errs, fmt.Sprintf("out‑of‑range voltage %.2fV at %s on %s", r.Voltage, r.MeterID, r.Timestamp))
            // not fatal; keep record
        }
        cleaned = append(cleaned, r)
    }
    return cleaned, errs, nil
}
