package services

import (
	"context"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// DataLoader loads readings from the CSV.
type DataLoader interface {
    Load(ctx context.Context) ([]models.Reading, error)
}

// DataQualityChecker validates the dataset.
// It returns cleaned readings, list of error messages, and an error if it cannot continue.
// type DataQualityChecker interface {
//     Check(ctx context.Context, readings []models.Reading) ([]models.Reading, []string, error)
// }

// BaselineCalculator produces a baseline per meter.
// Return a map[meterID]baselineConsumption.
// type BaselineCalculator interface {
//     Calculate(ctx context.Context, readings []models.Reading) (map[models.MeterID]float64, error)
// }

// AnomalyDetector finds candidate anomalous readings.
// It should use the produced baseline.
// type AnomalyDetector interface {
//     Detect(ctx context.Context, readings []models.Reading, baseline map[models.MeterID]float64) ([]models.AnomalyCandidate, error)
// }

// EventCorrelator correlates anomalies with events.
// It returns if the event explains the anomaly.
type EventCorrelator interface {
    Correlate(ctx context.Context, candidate models.AnomalyCandidate, events []models.Event) (models.EventCorrelation, error)
}

// Classifier assigns the semantic label.
// Expects: "anomaly", "explained", "quality_issue"
type Classifier interface {
    Classify(ctx context.Context, correlation models.EventCorrelation) (string, error)
}

// Scorer maps classification + correlation to a priority.
// e.g., higher for unexplained anomaly.
type Scorer interface {
    Score(ctx context.Context, classification string, correlation models.EventCorrelation) (int, error)
}

// EvidenceBuilder builds the object sent to the LLM.
type EvidenceBuilder interface {
    Build(ctx context.Context, correlation models.EventCorrelation) (models.Evidence, error)
}

// Orchestrator bundles all services.
// type Orchestrator struct {
//     Loader       DataLoader
//     Qualifier    DataQualityChecker
//     Baseline     BaselineCalculator
//     Detector     AnomalyDetector
//     Correlator   EventCorrelator
//     Classifier   Classifier
//     Scorer       Scorer
//     Builder      EvidenceBuilder
//     LLMProvider  interface{} // placeholder
// }

// Duplicate Run implementation removed; use the version in `orchestrator.go`.