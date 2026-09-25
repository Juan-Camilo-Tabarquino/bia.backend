package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/neuralium/ai-energy/internal/ai"
	"github.com/neuralium/ai-energy/internal/analysis"
	"github.com/neuralium/ai-energy/internal/api"
	"github.com/neuralium/ai-energy/internal/config"
	"github.com/neuralium/ai-energy/internal/data/csv"
	"github.com/neuralium/ai-energy/internal/data/memory"
)

func main() {
	_ = godotenv.Load() // ignore missing .env
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

	// Select the LLM from configuration: LLM_API_KEY set -> real provider,
	// unset -> deterministic mock (the default).
	llm := ai.NewProvider(cfg.LLMAPIKey)
	orchestrator := analysis.NewOrchestrator(loader, rrepo, erepo,
		quality, baseline, detector, correlator, classifier, scorer, llm, builder)

	router := api.NewRouter(orchestrator)
	// Run the orchestrator once at startup (ignore error)
	if err := orchestrator.Run(); err != nil {
		log.Fatalf("failed to load data: %v", err)
	}
	log.Printf("Server starting on port %d", cfg.ServerPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.ServerPort), router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
