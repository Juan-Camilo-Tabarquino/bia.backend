package main

import (
    "github.com/neuralium/ai-energy/internal/analysis"
    "github.com/neuralium/ai-energy/internal/data/csv"
    "github.com/neuralium/ai-energy/internal/data/memory"
    "github.com/neuralium/ai-energy/internal/config"
    "github.com/neuralium/ai-energy/internal/api"
    "net/http"
    "fmt"
    "log"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("failed to load config: %v", err)
    }

    // Instantiate services (stubs)
    loader := csv.NewLoader(cfg.ReadingsCSV, cfg.EventsCSV)
    rrepo := memory.NewReadingRepo()
    erepo := memory.NewEventRepo()
    quality := analysis.NewQualityChecker()
    baseline := analysis.NewBaselineCalculator()
    detector := analysis.NewAnomalyDetector()
    correlator := analysis.NewEventCorrelator()
    classifier := analysis.NewClassifier()
    scorer := analysis.NewScorer()
    builder := analysis.NewEvidenceBuilder()

    llm := analysis.NewMockLLM()
    orchestrator := analysis.NewOrchestrator(loader, rrepo, erepo,
        quality, baseline, detector, correlator, classifier, scorer, llm, builder)

    router := api.NewRouter(orchestrator, cfg.ServerPort)
    log.Printf("Server starting on port %d", cfg.ServerPort)
    if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.ServerPort), router); err != nil {
        log.Fatalf("server failed: %v", err)
    }
}
