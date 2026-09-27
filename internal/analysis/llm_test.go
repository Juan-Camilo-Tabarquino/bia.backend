package analysis

import (
	"strings"
	"testing"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// mockEvidenceFor builds the evidence the deterministic mock narrates so the
// exact Spanish copy of every branch can be pinned in isolation.
func mockEvidenceFor(anomalyType models.AnomalyType) models.Evidence {
	return models.Evidence{
		Anomaly: models.AnomalyCandidate{
			MeterID: "M1",
			Delta:   0.5,
		},
		Type:       anomalyType,
		Confidence: 0.85,
	}
}

// TestMockLLMExplanationIsSpanish pins the deterministic mock narrative to its
// exact neutral Spanish text for all four anomaly types plus the default
// branch, and proves the interpolated meter id, delta percentage and
// confidence still land in the sentence.
func TestMockLLMExplanationIsSpanish(t *testing.T) {
	client := NewMockLLM()

	tests := []struct {
		name     string
		in       models.Evidence
		want     string
		contains []string
	}{
		{
			name: "real anomaly",
			in:   mockEvidenceFor(models.AnomalyReal),
			want: "Revisión determinista del medidor M1: el consumo está 50.0% por encima de su línea base sin ningún evento operativo; las firmas eléctricas respaldan una anomalía real (confianza 0.85).",
			contains: []string{
				"M1", "50.0%", "0.85",
			},
		},
		{
			name: "data quality",
			in:   mockEvidenceFor(models.AnomalyDataQuality),
			want: "Revisión determinista del medidor M1: el consumo es estable pero las lecturas eléctricas son inconsistentes; esto debe tratarse como un problema de calidad de datos (confianza 0.85).",
			contains: []string{
				"M1", "0.85",
			},
		},
		{
			name: "explainable anomaly",
			in:   mockEvidenceFor(models.AnomalyExplainable),
			want: "Revisión determinista del medidor M1: la variación de 50.0% coincide con un cambio operativo registrado (confianza 0.85).",
			contains: []string{
				"M1", "50.0%", "0.85",
			},
		},
		{
			name: "false positive",
			in:   mockEvidenceFor(models.AnomalyFalsePositive),
			want: "Revisión determinista del medidor M1: la variación es un falso positivo causado por mantenimiento planificado (confianza 0.85).",
			contains: []string{
				"M1", "0.85",
			},
		},
		{
			name: "default",
			in:   mockEvidenceFor("SOMETHING_ELSE"),
			want: "Revisión determinista del medidor M1: no hay una clasificación de anomalía disponible.",
			contains: []string{
				"M1",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := client.GenerateExplanation(tc.in)
			if err != nil {
				t.Fatalf("GenerateExplanation() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Fatalf("GenerateExplanation() = %q, want %q", got, tc.want)
			}
			for _, substr := range tc.contains {
				if !strings.Contains(got, substr) {
					t.Errorf("GenerateExplanation() = %q, want it to contain %q", got, substr)
				}
			}
		})
	}
}
