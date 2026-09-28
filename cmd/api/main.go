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

	// Select the LLM from configuration: LLM_API_KEY set -> real Ollama
	// provider, unset -> deterministic mock (the default). LLM_BASE_URL and
	// LLM_MODEL carry the endpoint and the model:tag for the real provider.
	llm := ai.NewProvider(cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel)
	orchestrator := analysis.NewOrchestrator(loader, rrepo, erepo,
		quality, baseline, detector, correlator, classifier, scorer, llm, builder)

	router := api.NewRouter(orchestrator)

	// Run only the fast deterministic stage before serving. It populates the
	// repositories and publishes the complete anomaly snapshot, so the very
	// first request already sees correct evidence: the list and detail pages are
	// usable immediately. A failure here is fatal because there is no usable data
	// behind the API.
	//
	// The LLM narrative is deliberately NOT started here. It is now a per-meter,
	// on-demand job: the client starts one through POST /api/ai/analyze, which
	// narrates only the requested meter in the background while the client polls
	// GET /api/ai/analysis/{id} for progress. Starting it at boot would issue
	// whole-platform LLM calls nobody asked for.
	if err := orchestrator.Detect(); err != nil {
		log.Fatalf("failed to load data: %v", err)
	}

	log.Printf("Server starting on port %d", cfg.ServerPort)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.ServerPort), router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
