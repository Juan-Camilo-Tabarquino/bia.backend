package ai

import (
	"path/filepath"
	"testing"

	"github.com/neuralium/ai-energy/internal/analysis"
	"github.com/neuralium/ai-energy/internal/data/csv"
	"github.com/neuralium/ai-energy/internal/data/memory"
	"github.com/neuralium/ai-energy/internal/domain/models"
)

// TestBuildAnomalyPayloadRunsRealPipeline runs the real pipeline over the data
// CSV files and asserts the enriched LLM payload for the critical meters. The
// M-109 line is logged so the literal JSON can be inspected.
func TestBuildAnomalyPayloadRunsRealPipeline(t *testing.T) {
	evidence := runPipeline(t)

	m109 := BuildAnomalyPayload(evidenceFor(t, evidence, "M-109"))
	if m109.MeterID != "M-109" {
		t.Errorf("M-109 meter_id = %q, want %q", m109.MeterID, "M-109")
	}
	if m109.Classification != string(models.AnomalyReal) {
		t.Errorf("M-109 classification = %q, want %q", m109.Classification, models.AnomalyReal)
	}
	if m109.Severity != string(models.SeverityHigh) {
		t.Errorf("M-109 severity = %q, want %q", m109.Severity, models.SeverityHigh)
	}
	if m109.HasOperationalEvent {
		t.Errorf("M-109 has_operational_event = true; its UNKNOWN event must not explain the deviation")
	}
	if m109.ConsumptionChangePct <= 100 {
		t.Errorf("M-109 consumption_change_pct = %v, want > 100", m109.ConsumptionChangePct)
	}
	if m109.AnomalyScore <= 0.90 {
		t.Errorf("M-109 anomaly_score = %v, want > 0.90", m109.AnomalyScore)
	}
	t.Logf("M-109 payload: %s", mustJSON(t, m109))

	m112 := BuildAnomalyPayload(evidenceFor(t, evidence, "M-112"))
	if m112.Classification != string(models.AnomalyDataQuality) {
		t.Errorf("M-112 classification = %q, want %q", m112.Classification, models.AnomalyDataQuality)
	}
	if m112.HasOperationalEvent {
		t.Errorf("M-112 has_operational_event = true; a DATA_QUALITY event is not an operational event")
	}
	t.Logf("M-112 payload: %s", mustJSON(t, m112))
}

// TestBuildAnomalyPayloadJSONShape pins the exact JSON keys, key order and
// rounding of the payload, so the LLM contract cannot drift unnoticed.
func TestBuildAnomalyPayloadJSONShape(t *testing.T) {
	evidence := models.Evidence{
		Anomaly: models.AnomalyCandidate{
			MeterID:              "M-109",
			ConsumptionChangePct: 103.74,
			VoltageChangePct:     -4.24,
			CurrentChangePct:     31.76,
			PowerFactorChangePct: -12.44,
		},
		Confidence: 0.96,
		Type:       models.AnomalyReal,
		Severity:   models.SeverityHigh,
	}

	raw := mustJSON(t, BuildAnomalyPayload(evidence))
	const want = `{"meter_id":"M-109","consumption_change_pct":103.7,"voltage_change_pct":-4.2,"current_change_pct":31.8,"power_factor_change_pct":-12.4,"has_operational_event":false,"anomaly_score":0.96,"classification":"REAL_ANOMALY","severity":"HIGH"}`
	if raw != want {
		t.Fatalf("payload JSON =\n%s\nwant\n%s", raw, want)
	}
}

// TestHasOperationalEventSemantics pins the meaning of has_operational_event:
// only an explaining operational event sets it, so an UNKNOWN event and a
// DATA_QUALITY event both leave it false.
func TestHasOperationalEventSemantics(t *testing.T) {
	cases := []struct {
		name        string
		correlation models.EventCorrelation
		want        bool
	}{
		{
			name: "unknown event never explains",
			correlation: models.EventCorrelation{
				Explains: false,
				Events:   []models.Event{{ID: "M-109", Type: models.EventUnknown}},
			},
			want: false,
		},
		{
			name: "data quality is not operational",
			correlation: models.EventCorrelation{
				Explains: true,
				Events:   []models.Event{{ID: "M-112", Type: models.EventDataQuality}},
			},
			want: false,
		},
		{
			name: "operational change explains",
			correlation: models.EventCorrelation{
				Explains: true,
				Events:   []models.Event{{ID: "M-104", Type: models.EventOperationalChange}},
			},
			want: true,
		},
		{
			name: "scheduled outage explains",
			correlation: models.EventCorrelation{
				Explains: true,
				Events:   []models.Event{{ID: "M-106", Type: models.EventScheduledOutage}},
			},
			want: true,
		},
		{
			name:        "no events",
			correlation: models.EventCorrelation{},
			want:        false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildAnomalyPayload(models.Evidence{Correlation: tc.correlation}).HasOperationalEvent
			if got != tc.want {
				t.Fatalf("has_operational_event = %v, want %v", got, tc.want)
			}
		})
	}
}

// runPipeline executes the real deterministic pipeline over data/ and returns
// the produced evidence.
func runPipeline(t *testing.T) []models.Evidence {
	t.Helper()
	// This package lives at <root>/internal/ai, one level below the repository
	// root, so the data files resolve two levels up.
	readingsPath := filepath.Join("..", "..", "data", "readings.csv")
	eventsPath := filepath.Join("..", "..", "data", "events.csv")

	orchestrator := analysis.NewOrchestrator(
		csv.NewLoader(readingsPath, eventsPath),
		memory.NewReadingRepo(),
		memory.NewEventRepo(),
		analysis.NewQualityChecker(),
		analysis.NewBaselineCalculator(),
		analysis.NewAnomalyDetector(),
		analysis.NewEventCorrelator(),
		analysis.NewClassifier(),
		analysis.NewScorer(),
		analysis.NewMockLLM(),
		analysis.NewEvidenceBuilder(),
	)
	if err := orchestrator.Run(); err != nil {
		t.Fatalf("orchestrator run failed: %v", err)
	}
	return orchestrator.Evidence()
}

// evidenceFor returns the evidence record for the given meter.
func evidenceFor(t *testing.T, evidence []models.Evidence, meter models.MeterID) models.Evidence {
	t.Helper()
	for _, e := range evidence {
		if e.Anomaly.MeterID == meter {
			return e
		}
	}
	t.Fatalf("no evidence produced for meter %s", meter)
	return models.Evidence{}
}

// mustJSON serialises the payload or fails the test.
func mustJSON(t *testing.T, payload AnomalyPayload) string {
	t.Helper()
	raw, err := payload.JSON()
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(raw)
}
