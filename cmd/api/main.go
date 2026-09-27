package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

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
	// first request already sees correct evidence. A failure here is fatal:
	// there is no usable data behind the API.
	if err := orchestrator.Detect(); err != nil {
		log.Fatalf("failed to load data: %v", err)
	}

	log.Printf("Server starting on port %d", cfg.ServerPort)

	// The slow LLM enrichment runs in the background so it never delays the
	// listening socket. It updates each evidence item's narrative under a short
	// lock, so read endpoints stay responsive while it runs. Unlike Detect, a
	// failure here must not kill the server: the deterministic payload is
	// already complete and only the optional llm_analysis text is missing.
	go func() {
		start := time.Now()
		log.Printf("LLM enrichment started")
		orchestrator.Enrich()
		log.Printf("LLM enrichment finished in %s", time.Since(start).Round(time.Millisecond))
	}()

	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.ServerPort), router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
