package analysis

import (
    "fmt"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

// LLMClient is an abstraction over any language‑model provider.
// It is used by the orchestrator to enrich Evidence with natural‑language
// explanations and recommendations.

type LLMClient interface {
    GenerateExplanation(evidence models.Evidence) (string, error)
}

// mockLLM is a lightweight implementation that returns deterministic text.
// It is suitable for unit tests and development environments that cannot
// access external LLM services.

type mockLLM struct{}

func NewMockLLM() LLMClient { return &mockLLM{} }

func (m *mockLLM) GenerateExplanation(e models.Evidence) (string, error) {
    txt := fmt.Sprintf("Anomaly on meter %s at %s: generated mock explanation.",
        e.Anomaly.MeterID, e.Anomaly.Timestamp.Format("2006-01-02 15:04"))
    return txt, nil
}
