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
		return fmt.Sprintf("Revisión determinista del medidor %s: el consumo está %.1f%% por encima de su línea base sin ningún evento operativo; las firmas eléctricas respaldan una anomalía real (confianza %.2f).",
			meter, e.Anomaly.Delta*100, e.Confidence), nil
	case models.AnomalyDataQuality:
		return fmt.Sprintf("Revisión determinista del medidor %s: el consumo es estable pero las lecturas eléctricas son inconsistentes; esto debe tratarse como un problema de calidad de datos (confianza %.2f).",
			meter, e.Confidence), nil
	case models.AnomalyExplainable:
		return fmt.Sprintf("Revisión determinista del medidor %s: la variación de %.1f%% coincide con un cambio operativo registrado (confianza %.2f).",
			meter, e.Anomaly.Delta*100, e.Confidence), nil
	case models.AnomalyFalsePositive:
		return fmt.Sprintf("Revisión determinista del medidor %s: la variación es un falso positivo causado por mantenimiento planificado (confianza %.2f).",
			meter, e.Confidence), nil
	default:
		return fmt.Sprintf("Revisión determinista del medidor %s: no hay una clasificación de anomalía disponible.", meter), nil
	}
}
