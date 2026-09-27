package analysis

import (
	"testing"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// evidenceFor builds a minimal evidence record for the given anomaly type so
// the deterministic explanation and recommendation can be asserted in
// isolation.
func evidenceFor(anomalyType models.AnomalyType) models.Evidence {
	return models.Evidence{
		Anomaly: models.AnomalyCandidate{
			MeterID: "M1",
			Delta:   0.5,
		},
		Type: anomalyType,
	}
}

// TestDescribeEvidenceIsSpanish proves the deterministic explanation prose is
// neutral Spanish, that the interpolated meter id and percentage still land in
// their original positions, and that an interpolated event description or data
// quality reason is passed through untouched.
func TestDescribeEvidenceIsSpanish(t *testing.T) {
	tests := []struct {
		name string
		in   models.Evidence
		want string
	}{
		{
			name: "data quality with trigger detail",
			in: func() models.Evidence {
				e := evidenceFor(models.AnomalyDataQuality)
				e.Anomaly.Reason = "reason-x"
				return e
			}(),
			want: "El medidor M1 presenta lecturas eléctricas inconsistentes (reason-x) mientras su consumo se mantiene cerca de la línea base horaria.",
		},
		{
			name: "data quality without trigger detail",
			in:   evidenceFor(models.AnomalyDataQuality),
			want: "El medidor M1 presenta lecturas eléctricas inconsistentes mientras su consumo se mantiene cerca de la línea base horaria.",
		},
		{
			name: "real anomaly",
			in:   evidenceFor(models.AnomalyReal),
			want: "El consumo del medidor M1 está 50.0% por encima de su línea base sin ningún evento operativo conocido.",
		},
		{
			name: "explainable anomaly",
			in: func() models.Evidence {
				e := evidenceFor(models.AnomalyExplainable)
				e.Correlation.Events = []models.Event{{Description: "Desc", Type: models.EventOperationalChange}}
				return e
			}(),
			want: "El consumo del medidor M1 está 50.0% por encima de su línea base y coincide con un cambio operativo conocido: Desc (OPERATIONAL_CHANGE).",
		},
		{
			name: "false positive",
			in: func() models.Evidence {
				e := evidenceFor(models.AnomalyFalsePositive)
				e.Correlation.Events = []models.Event{{Description: "Desc", Type: models.EventMaintenance}}
				return e
			}(),
			want: "La desviación del consumo del medidor M1 de 50.0% se explica por mantenimiento planificado: Desc (MAINTENANCE).",
		},
		{
			name: "default",
			in:   evidenceFor("SOMETHING_ELSE"),
			want: "El medidor M1 presenta una desviación no explicada de 50.0%.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeEvidence(tc.in); got != tc.want {
				t.Fatalf("describeEvidence() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEventDescriptionIsSpanish proves the empty-event placeholder and the
// join separator are Spanish while the interpolated dataset description and
// enum token stay byte-identical.
func TestEventDescriptionIsSpanish(t *testing.T) {
	if got, want := eventDescription(nil), "sin evento registrado"; got != want {
		t.Fatalf("eventDescription(nil) = %q, want %q", got, want)
	}

	events := []models.Event{
		{Description: "Parada programada", Type: models.EventScheduledOutage},
		{Type: models.EventOperationalChange},
	}
	want := "Parada programada (SCHEDULED_OUTAGE); OPERATIONAL_CHANGE"
	if got := eventDescription(events); got != want {
		t.Fatalf("eventDescription(events) = %q, want %q", got, want)
	}
}

// TestRecommendActionIsSpanish proves every recommendation branch returns the
// exact neutral Spanish copy.
func TestRecommendActionIsSpanish(t *testing.T) {
	tests := []struct {
		in   models.AnomalyType
		want string
	}{
		{models.AnomalyReal, "Revisar el medidor y su instalación."},
		{models.AnomalyDataQuality, "Revisar la calibración del sensor y el proceso de calidad de datos de este medidor."},
		{models.AnomalyExplainable, "No se requiere acción; la desviación coincide con un cambio operativo registrado."},
		{models.AnomalyFalsePositive, "No se requiere acción; la desviación se explica por mantenimiento planificado."},
		{"SOMETHING_ELSE", "Revisar la lectura antes de escalar."},
	}
	for _, tc := range tests {
		if got := recommendAction(tc.in); got != tc.want {
			t.Errorf("recommendAction(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
