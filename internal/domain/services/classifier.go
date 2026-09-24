package services

import (
    "context"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

func NewStaticClassifier() *StaticClassifier {
    return &StaticClassifier{}
}

type StaticClassifier struct{}

func (cl *StaticClassifier) Classify(ctx context.Context, corr models.EventCorrelation) (string, error) {
    if len(corr.Events) > 0 {
        return "explained", nil
    }
    return "anomaly", nil
}
