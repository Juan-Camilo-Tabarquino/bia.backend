package analysis

import "github.com/neuralium/ai-energy/internal/domain/models"

// Classifier assigns a coarse classification to an anomaly based on
// correlation.
// REAL_ANOMALY if correlated with an event or if the anomaly is large, else
// FALSE_POSITIVE.  DATA_QUALITY is handled earlier.

type Classifier interface { Classify(correlations []models.EventCorrelation) []models.Evidence }

type simpleClassifier struct{}

func NewClassifier() Classifier { return &simpleClassifier{} }

func (cl *simpleClassifier) Classify(correlations []models.EventCorrelation) []models.Evidence {
    var out []models.Evidence
    for _, c := range correlations {
        priority := 3
        if len(c.Events) > 0 || c.Anomaly.Delta > 10 {
            priority = 1
        }
        out = append(out, models.Evidence{Anomaly: c.Anomaly, Correlation: c, Priority: priority})
    }
    return out
}
