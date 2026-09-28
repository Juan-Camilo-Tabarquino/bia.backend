package api

import (
	"encoding/json"
	"github.com/neuralium/ai-energy/internal/analysis"
	"github.com/neuralium/ai-energy/internal/api/handlers"
	"github.com/neuralium/ai-energy/internal/domain/models"
	"net/http"
	"strings"
	"time"
)

func corsWrapper(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NewRouter builds the single HTTP router for the API.
//
// Prefix decision (intentional): every route is registered under the "/api"
// prefix and nowhere else. This project deliberately keeps one single prefix;
// there is no legacy unprefixed alias and no "/api/v1". Do not reintroduce one:
// adding a second prefix would force clients and the UI to track two base paths
// for the same contract.
func NewRouter(orchestrator *analysis.Orchestrator) http.Handler {
	mux := http.NewServeMux()
	// Wrap handlers with CORS
	health := corsWrapper(http.HandlerFunc(handlers.Health))
	reports := corsWrapper(reportsHandler(orchestrator))

	// Meters handlers (list, detail and readings combined)
	metersHandler := corsWrapper(handlers.Meters(orchestrator.ReadingRepo))
	meterDetail := handlers.MeterDetail(orchestrator)
	// Combined handler for /api/meters/{meterId} and /api/meters/{meterId}/readings
	customMeters := corsWrapper(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Routes are registered under /api, so normalise the path before
		// matching the meter sub-paths (the router is never StripPrefix-ed).
		path := strings.TrimPrefix(r.URL.Path, "/api")
		// Expected base path after /api: /meters/{...}
		prefix := "/meters/"
		if !strings.HasPrefix(path, prefix) {
			http.NotFound(w, r)
			return
		}
		rest := strings.TrimPrefix(path, prefix)
		// If the request ends with "/readings", treat as readings endpoint
		if strings.HasSuffix(rest, "/readings") {
			id := strings.TrimSuffix(rest, "/readings")
			var from, to *time.Time
			if s := r.URL.Query().Get("from"); s != "" {
				if ts, err := time.Parse(time.RFC3339, s); err == nil {
					from = &ts
				}
			}
			if s := r.URL.Query().Get("to"); s != "" {
				if ts, err := time.Parse(time.RFC3339, s); err == nil {
					to = &ts
				}
			}
			data := orchestrator.ReadingRepo.ReadingsFor([]string{id}, from, to)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(data)
			return
		}
		// Otherwise, return the real meter metadata.
		meterDetail.ServeHTTP(w, r)
	}))

	// Anomalies handlers
	anomaliesHandler := corsWrapper(handlers.Anomalies(orchestrator))
	anomalyDetail := corsWrapper(handlers.AnomalyDetailByID(orchestrator))

	// All routes live under the single /api prefix by design.
	mux.Handle("/api/health", health)
	mux.Handle("/api/reports", reports)

	// Register meters routes
	mux.Handle("/api/meters", metersHandler)
	mux.Handle("/api/meters/", customMeters)

	// Register anomalies routes
	mux.Handle("/api/anomalies", anomaliesHandler)
	// Detail of a single anomaly (GET /api/anomalies/{id}).
	mux.Handle("/api/anomalies/", anomalyDetail)

	// AI analysis endpoints
	aiAnalyze := corsWrapper(handlers.AnalyzePOST(orchestrator))
	mux.Handle("/api/ai/analyze", aiAnalyze)
	aiResult := corsWrapper(http.HandlerFunc(handlers.AnalysisGET))
	mux.Handle("/api/ai/analysis/", aiResult) // pattern handled inside handler
	// Dashboard summary
	dashboard := corsWrapper(handlers.DashboardSummary(orchestrator))
	mux.Handle("/api/dashboard/summary", dashboard)

	// Demo login: verifies the committed CSV credential store and ISSUES an
	// HS256 JWT. Deliberate scope decision: no other route validates that token,
	// so the frontend guard is UX and not a security boundary (docs/architecture.md).
	mux.Handle("/api/auth/login", corsWrapper(http.HandlerFunc(handlers.AuthLogin)))

	return mux
}

func reportsHandler(orch *analysis.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For MVP we ignore query params, use all events as empty slice
		evidences := orch.Evidence()
		w.Header().Set("Content-Type", "application/json")
		// simple Marshal
		_ = json.NewEncoder(w).Encode(struct {
			Reports []models.Evidence `json:"reports"`
		}{Reports: evidences})
	}
}
