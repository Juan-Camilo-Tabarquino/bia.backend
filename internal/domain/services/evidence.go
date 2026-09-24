package services

import (
	"context"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

func NewStaticEvidenceBuilder() *StaticEvidenceBuilder {
    return &StaticEvidenceBuilder{}
}

type StaticEvidenceBuilder struct{}

func (eb *StaticEvidenceBuilder) Build(ctx context.Context, corr models.EventCorrelation) (models.Evidence, error) {
    evidence := models.Evidence{Anomaly: corr.Anomaly, Correlation: corr}
    return evidence, nil
}
