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
