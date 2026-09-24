package services

import (
    "context"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

func NewStaticScorer() *StaticScorer {
    return &StaticScorer{}
}

type StaticScorer struct{}

func (sc *StaticScorer) Score(ctx context.Context, classification string, corr models.EventCorrelation) (int, error) {
    switch classification {
    case "anomaly":
        return 10, nil
    case "explained":
        return 5, nil
    default:
        return 3, nil
    }
}
