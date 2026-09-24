package handlers

import (
    "encoding/json"
    "net/http"
    "time"

    "log"
    "github.com/neuralium/ai-energy/internal/analysis"
    "github.com/go-chi/chi/v5"
    "github.com/neuralium/ai-energy/internal/data/memory"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

// Health check
func Health(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    // Write JSON response directly to match test expectations (no trailing newline)
    _, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Meter list
func Meters(rrepo *memory.ReadingRepo) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ids := rrepo.AllMeterIDs()
        json.NewEncoder(w).Encode(ids)
    }
}

// Readings for a specific meter with optional time range
func Readings(rrepo *memory.ReadingRepo) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        mid := chi.URLParam(r, "id")
        var from, to *time.Time
        if s := r.URL.Query().Get("from"); s != "" {
            ts, err := time.Parse(time.RFC3339, s)
            if err == nil {from = &ts}
        }
        if s := r.URL.Query().Get("to"); s != "" {
            ts, err := time.Parse(time.RFC3339, s)
            if err == nil {to = &ts}
        }
        data := rrepo.ReadingsFor([]string{mid}, from, to)
        json.NewEncoder(w).Encode(data)
    }
}

// List all anomalies (evidence)
func Anomalies(orchestrator *analysis.Orchestrator) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if err := orchestrator.Run(); err != nil {
            log.Printf("error: %v", err)
            http.Error(w, "pipeline error", http.StatusInternalServerError)
            return
        }
        json.NewEncoder(w).Encode(orchestrator.Evidence())
    }
}

// Detail for a specific meter's anomalies – placeholder
func AnomalyDetail(orchestrator *analysis.Orchestrator) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        mid := chi.URLParam(r, "meter")
        if err := orchestrator.Run(); err != nil {
            http.Error(w, "pipeline error", http.StatusInternalServerError)
            return
        }
        ev := orchestrator.Evidence()
        // filter by meter
        var filtered []models.Evidence
        for _, e := range ev {
            if string(e.Anomaly.MeterID) == mid {
                filtered = append(filtered, e)
            }
        }
        json.NewEncoder(w).Encode(filtered)
    }
}
