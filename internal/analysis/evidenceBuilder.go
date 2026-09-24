package analysis

import "github.com/neuralium/ai-energy/internal/domain/models"

// EvidenceBuilder could enrich evidence with LLM output or other fields.
// Here it simply forwards what the classifier produced.

type EvidenceBuilder interface { Build(candidates []models.AnomalyCandidate, correlations []models.EventCorrelation) []models.Evidence }

func NewEvidenceBuilder() EvidenceBuilder { return &defaultEvidenceBuilder{} }

type defaultEvidenceBuilder struct{}

func (b *defaultEvidenceBuilder) Build(candidates []models.AnomalyCandidate, correlations []models.EventCorrelation) []models.Evidence {
    // Currently, no additional evidence enrichment is performed.
    // Return an empty slice of Evidence to satisfy the method contract.
    return []models.Evidence{}
}
