package api

import (
    "encoding/json"
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/neuralium/ai-energy/internal/analysis"
    "github.com/neuralium/ai-energy/internal/domain/models"
    "net/http"
)

func NewRouter(orchestrator *analysis.Orchestrator, port int) http.Handler {
    r := chi.NewRouter()
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    r.Get("/health", healthHandler)
    r.Get("/reports", reportsHandler(orchestrator))
    return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Write([]byte(`{"status":"ok"}`))
}

func reportsHandler(orch *analysis.Orchestrator) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // For MVP we ignore query params, use all events as empty slice
        err := orch.Run()
            if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            evidences := orch.Evidence()
        w.Header().Set("Content-Type", "application/json")
        // simple Marshal
        _ = json.NewEncoder(w).Encode(struct {
            Reports []models.Evidence `json:"reports"`
        }{Reports: evidences})
    }
}
