package analysis

import (
	"path/filepath"
	"testing"

	"github.com/neuralium/ai-energy/internal/data/csv"
	"github.com/neuralium/ai-energy/internal/data/memory"
	"github.com/neuralium/ai-energy/internal/domain/models"
)

// TestRequirementsDatasetOutcomes runs the real pipeline over the data/ CSV
// files and asserts the four outcomes the specification requires for the
// critical meters, plus the general contract that evidence is produced and
// every record is fully populated.
func TestRequirementsDatasetOutcomes(t *testing.T) {
	// The package lives at <root>/internal/analysis, so the repository root is
	// two levels up from the test's working directory.
	readingsPath := filepath.Join("..", "..", "data", "readings.csv")
	eventsPath := filepath.Join("..", "..", "data", "events.csv")

	orchestrator := NewOrchestrator(
		csv.NewLoader(readingsPath, eventsPath),
		memory.NewReadingRepo(),
		memory.NewEventRepo(),
		NewQualityChecker(),
		NewBaselineCalculator(),
		NewAnomalyDetector(),
		NewEventCorrelator(),
		NewClassifier(),
		NewScorer(),
		NewMockLLM(),
		NewEvidenceBuilder(),
	)
	if err := orchestrator.Run(); err != nil {
		t.Fatalf("orchestrator run failed: %v", err)
	}

	evidence := orchestrator.Evidence()
	if len(evidence) == 0 {
		t.Fatal("expected non-empty evidence, got an empty slice")
	}
	for _, e := range evidence {
		t.Logf("produced meter=%s kind=%s type=%s severity=%s confidence=%.4f status=%s priority=%d events=%d",
			e.Anomaly.MeterID, e.Anomaly.Kind, e.Type, e.Severity, e.Confidence, e.Status, e.Priority, len(e.Correlation.Events))
	}

	expected := []struct {
		meter         models.MeterID
		anomalyType   models.AnomalyType
		severity      models.Severity
		minConfidence float64
	}{
		{meter: "M-104", anomalyType: models.AnomalyExplainable, severity: models.SeverityMedium},
		{meter: "M-106", anomalyType: models.AnomalyFalsePositive, severity: models.SeverityLow},
		{meter: "M-109", anomalyType: models.AnomalyReal, severity: models.SeverityHigh, minConfidence: 0.90},
		{meter: "M-112", anomalyType: models.AnomalyDataQuality, severity: models.SeverityHigh},
	}

	if len(evidence) != len(expected) {
		t.Errorf("expected exactly %d evidence records (one per critical meter), got %d", len(expected), len(evidence))
	}

	seen := make(map[models.MeterID]bool)
	for _, want := range expected {
		got, ok := strongestEvidenceFor(evidence, want.meter)
		if !ok {
			t.Errorf("no evidence produced for meter %s", want.meter)
			continue
		}
		seen[want.meter] = true
		if got.Type != want.anomalyType {
			t.Errorf("meter %s: expected type %s, got %s", want.meter, want.anomalyType, got.Type)
		}
		if got.Severity != want.severity {
			t.Errorf("meter %s: expected severity %s, got %s", want.meter, want.severity, got.Severity)
		}
		if want.minConfidence > 0 && got.Confidence <= want.minConfidence {
			t.Errorf("meter %s: expected confidence > %.2f, got %.4f", want.meter, want.minConfidence, got.Confidence)
		}
		if got.Explanation == "" || got.Recommendation == "" || got.LLMText == "" {
			t.Errorf("meter %s: evidence is not fully populated: %+v", want.meter, got)
		}
		if got.Status == "" {
			t.Errorf("meter %s: status is empty", want.meter)
		}
		// The candidate must carry the per-meter baseline it was compared
		// against, so consumers can report the statistics behind the
		// classification without re-deriving them.
		if got.Anomaly.Baseline.Mean == 0 || got.Anomaly.Baseline.Count == 0 {
			t.Errorf("meter %s: candidate does not carry the meter baseline: %+v", want.meter, got.Anomaly.Baseline)
		}
	}

	for _, e := range evidence {
		if !seen[e.Anomaly.MeterID] {
			t.Errorf("unexpected evidence for meter %s (type %s); normal meters must not produce anomalies",
				e.Anomaly.MeterID, e.Type)
		}
	}
}

// strongestEvidenceFor returns the evidence record with the highest confidence
// for the given meter.
func strongestEvidenceFor(evidence []models.Evidence, meter models.MeterID) (models.Evidence, bool) {
	var best models.Evidence
	found := false
	for _, e := range evidence {
		if e.Anomaly.MeterID != meter {
			continue
		}
		if !found || e.Confidence > best.Confidence {
			best = e
			found = true
		}
	}
	return best, found
}
