package analysis

import (
	"fmt"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// LLMClient is an abstraction over any language-model provider.
// It is used by the orchestrator to enrich Evidence with natural-language
// explanations after the deterministic detection stage. The LLM never decides
// whether a reading is anomalous; it only narrates the classification.

type LLMClient interface {
	GenerateExplanation(evidence models.Evidence) (string, error)
}

// mockLLM is a lightweight implementation that returns deterministic text.
// It is suitable for unit tests and development environments that cannot
// access external LLM services.

type mockLLM struct{}

func NewMockLLM() LLMClient { return &mockLLM{} }

func (m *mockLLM) GenerateExplanation(e models.Evidence) (string, error) {
	meter := string(e.Anomaly.MeterID)
	switch e.Type {
	case models.AnomalyReal:
		return fmt.Sprintf("Deterministic review of meter %s: consumption is %.1f%% above baseline with no operational event; electrical signatures support a real anomaly (confidence %.2f).",
			meter, e.Anomaly.Delta*100, e.Confidence), nil
	case models.AnomalyDataQuality:
		return fmt.Sprintf("Deterministic review of meter %s: consumption is stable but the electrical readings are inconsistent; treat this as a data-quality problem (confidence %.2f).",
			meter, e.Confidence), nil
	case models.AnomalyExplainable:
		return fmt.Sprintf("Deterministic review of meter %s: the %.1f%% change matches a registered operational change (confidence %.2f).",
			meter, e.Anomaly.Delta*100, e.Confidence), nil
	case models.AnomalyFalsePositive:
		return fmt.Sprintf("Deterministic review of meter %s: the change is a false positive caused by planned maintenance (confidence %.2f).",
			meter, e.Confidence), nil
	default:
		return fmt.Sprintf("Deterministic review of meter %s: no anomaly classification available.", meter), nil
	}
}
