package analysis_test

import (
	"os"
	"testing"

	"github.com/neuralium/ai-energy/internal/analysis"
	"github.com/neuralium/ai-energy/internal/data/csv"
	"github.com/neuralium/ai-energy/internal/data/memory"
)

func createTempCSV(content string) (string, error) {
	f, err := os.CreateTemp("", "tmp-*.csv")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// TestOrchestrator_DataGapsForSingleReadingMeter pins the analysis-side signal:
// a meter that survives the quality check but has too few readings for a
// baseline is reported as a data gap, is never turned into evidence, and the gap
// carries the exact user-visible reason.
func TestOrchestrator_DataGapsForSingleReadingMeter(t *testing.T) {
	readingsCSV := "meter_id,timestamp,consumption,voltage,current,power_factor,status\n" +
		// Exactly one reading: no baseline can be computed for this meter.
		"M-ONE,2026-09-01 00:00:00,12,240,45,0.95,OK\n" +
		// A normal meter with several readings, so the gap signal is selective.
		"M-MANY,2026-09-01 00:00:00,10,220,45,0.95,OK\n" +
		"M-MANY,2026-09-01 01:00:00,10,220,45,0.95,OK\n" +
		"M-MANY,2026-09-01 02:00:00,10,220,45,0.95,OK\n"
	rPath, err := createTempCSV(readingsCSV)
	if err != nil {
		t.Fatalf("failed to create temp readings csv: %v", err)
	}
	defer os.Remove(rPath)
	ePath, err := createTempCSV("event_id,event_type,start_time,end_time,description\n")
	if err != nil {
		t.Fatalf("failed to create temp events csv: %v", err)
	}
	defer os.Remove(ePath)

	orch := analysis.NewOrchestrator(
		csv.NewLoader(rPath, ePath),
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
	if err := orch.Detect(); err != nil {
		t.Fatalf("Detect returned an error: %v", err)
	}

	gaps := orch.DataGaps()
	if len(gaps) != 1 {
		t.Fatalf("expected exactly 1 data gap, got %d: %+v", len(gaps), gaps)
	}
	gap := gaps[0]
	if gap.MeterID != "M-ONE" {
		t.Errorf("expected gap for M-ONE, got %q", gap.MeterID)
	}
	if gap.Readings != 1 {
		t.Errorf("expected the gap to report 1 reading, got %d", gap.Readings)
	}
	if gap.Reason != analysis.InsufficientReadingsReason {
		t.Errorf("expected reason %q, got %q", analysis.InsufficientReadingsReason, gap.Reason)
	}

	for _, e := range orch.Evidence() {
		if e.Anomaly.MeterID == "M-ONE" {
			t.Errorf("a meter reported as a data gap must not produce evidence: %+v", e)
		}
	}
}

func TestOrchestrator_Run_Empty(t *testing.T) {
	// Prepare minimal CSV files with only headers.
	readingsHeader := "meter_id,timestamp,consumption,voltage,current,power_factor\n"
	eventsHeader := "event_id,event_type,start_time,end_time,description\n"
	rPath, err := createTempCSV(readingsHeader)
	if err != nil {
		t.Fatalf("failed to create temp readings csv: %v", err)
	}
	defer os.Remove(rPath)
	ePath, err := createTempCSV(eventsHeader)
	if err != nil {
		t.Fatalf("failed to create temp events csv: %v", err)
	}
	defer os.Remove(ePath)

	loader := csv.NewLoader(rPath, ePath)
	rrepo := memory.NewReadingRepo()
	erepo := memory.NewEventRepo()
	qc := analysis.NewQualityChecker()
	bc := analysis.NewBaselineCalculator()
	det := analysis.NewAnomalyDetector()
	co := analysis.NewEventCorrelator()
	cl := analysis.NewClassifier()
	sc := analysis.NewScorer()
	llm := analysis.NewMockLLM()
	eb := analysis.NewEvidenceBuilder()

	orch := analysis.NewOrchestrator(loader, rrepo, erepo, qc, bc, det, co, cl, sc, llm, eb)

	if err := orch.Run(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ev := orch.Evidence(); len(ev) != 0 {
		t.Fatalf("expected empty evidence slice, got %d items", len(ev))
	}
}
