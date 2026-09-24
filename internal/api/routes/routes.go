package routes

import (

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"

    "github.com/neuralium/ai-energy/internal/api/handlers"
    "github.com/neuralium/ai-energy/internal/analysis"
    "github.com/neuralium/ai-energy/internal/data/memory"
)

func Make(r chi.Router, orchestrator *analysis.Orchestrator, rrepo *memory.ReadingRepo) {
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)

    r.Get("/api/health", handlers.Health)
    r.Get("/api/meters", handlers.Meters(rrepo))
    r.Get("/api/meters/{id}/readings", handlers.Readings(rrepo))
    r.Get("/api/anomalies", handlers.Anomalies(orchestrator))
    r.Get("/api/anomalies/{meter}", handlers.AnomalyDetail(orchestrator))
}

// NewRouter creates handler tree used by http.ListenAndServe.
func NewRouter(orchestrator *analysis.Orchestrator, port int) chi.Router {
    r := chi.NewRouter()
    // reading repo comes from orchestrator
    rrepo := orchestrator.ReadingRepo
    Make(r, orchestrator, rrepo)
    return r
}
