package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neuralium/ai-energy/internal/analysis"
	"github.com/neuralium/ai-energy/internal/domain/models"
)

// analysisStore keeps an immutable snapshot of the evidence produced by each
// POST /api/ai/analyze run, keyed by the generated analysis id. A mutex guards
// the map so concurrent requests for the same id neither race nor panic.
var (
	analysisStore = make(map[string][]models.Evidence)
	storeMutex    sync.Mutex
)

// analysisResultDTO is the body returned by GET /api/ai/analysis/{id}.
type analysisResultDTO struct {
	AnalysisID string       `json:"analysisId"`
	Status     string       `json:"status"`
	Anomalies  []AnomalyDTO `json:"anomalies"`
}

// meterDTO is the body returned by GET /api/meters/{meterId}.
type meterDTO struct {
	ID            string `json:"id"`
	MeterID       string `json:"meter_id"`
	Name          string `json:"name"`
	Location      string `json:"location"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	ReadingsCount int    `json:"readings_count"`
	LastReadingAt string `json:"last_reading_at"`
}

// POST /api/ai/analyze – re-run the pipeline and return a fresh analysis id.
func AnalyzePOST(orchestrator *analysis.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Re-run the real pipeline through the existing orchestrator. Run is
		// idempotent, so calling it here never duplicates the stored readings.
		if err := orchestrator.Run(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Evidence returns a copy, so the snapshot is immune to a later run.
		snapshot := orchestrator.Evidence()
		analysisID := uuid.New().String()
		storeMutex.Lock()
		analysisStore[analysisID] = snapshot
		storeMutex.Unlock()
		writeJSON(w, http.StatusOK, map[string]string{"analysisId": analysisID})
	}
}

// GET /api/ai/analysis/{id} – retrieve the stored result of a previous analysis.
func AnalysisGET(w http.ResponseWriter, r *http.Request) {
	// Normalize path: the router mounts everything under /api.
	path := strings.TrimPrefix(r.URL.Path, "/api")
	// Expected pattern: /ai/analysis/{id}
	prefix := "/ai/analysis/"
	if !strings.HasPrefix(path, prefix) {
		writeJSONError(w, http.StatusNotFound, "analysis not found")
		return
	}
	id := strings.TrimPrefix(path, prefix)
	storeMutex.Lock()
	snapshot, ok := analysisStore[id]
	storeMutex.Unlock()
	if !ok {
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("analysis %s not found", id))
		return
	}
	writeJSON(w, http.StatusOK, analysisResultDTO{
		AnalysisID: id,
		Status:     "completed",
		Anomalies:  anomalyDTOs(snapshot),
	})
}

// GET /api/dashboard/summary – provide a high‑level summary for the UI.
func DashboardSummary(orchestrator *analysis.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary := map[string]any{
			"health":    "ok",
			"meters":    len(orchestrator.ReadingRepo.AllMeterIDs()),
			"anomalies": len(orchestrator.Evidence()),
			"lastRun":   "latest", // placeholder
		}
		writeJSON(w, http.StatusOK, summary)
	}
}

// GET /api/meters/{meterId} – return real, derivable meter metadata.
//
// The values come from the reading repository. This project has no data source
// for a meter's name or location, so Name and Location are always returned as
// empty strings rather than invented values.
func MeterDetail(orchestrator *analysis.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api")
		prefix := "/meters/"
		if !strings.HasPrefix(path, prefix) {
			writeJSONError(w, http.StatusNotFound, "meter not found")
			return
		}
		id := strings.TrimPrefix(path, prefix)
		if idx := strings.Index(id, "/"); idx != -1 {
			id = id[:idx]
		}
		readings := orchestrator.ReadingRepo.ReadingsFor([]string{id}, nil, nil)
		if len(readings) == 0 {
			writeJSONError(w, http.StatusNotFound, fmt.Sprintf("meter %s not found", id))
			return
		}
		sort.Slice(readings, func(i, j int) bool {
			return readings[i].Timestamp.Before(readings[j].Timestamp)
		})
		// A meter is healthy only when every reading carries an OK status.
		status := "OK"
		for _, reading := range readings {
			if !strings.EqualFold(strings.TrimSpace(reading.Status), "OK") {
				status = "DEGRADED"
				break
			}
		}
		resp := meterDTO{
			ID:            id,
			MeterID:       id,
			Name:          "", // no meter-name source exists in this project
			Location:      "", // no meter-location source exists in this project
			Status:        status,
			CreatedAt:     readings[0].Timestamp.UTC().Format(time.RFC3339),
			ReadingsCount: len(readings),
			LastReadingAt: readings[len(readings)-1].Timestamp.UTC().Format(time.RFC3339),
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// GET /api/anomalies/{id} – returns the full anomaly detail for the stable id
// published by the list endpoint. Every field comes from the real evidence.
func AnomalyDetailByID(orchestrator *analysis.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api")
		prefix := "/anomalies/"
		if !strings.HasPrefix(path, prefix) {
			writeJSONError(w, http.StatusNotFound, "anomaly not found")
			return
		}
		id := strings.TrimPrefix(path, prefix)
		for _, ev := range orchestrator.Evidence() {
			if AnomalyID(ev) == id {
				writeJSON(w, http.StatusOK, newAnomalyDTO(ev))
				return
			}
		}
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("anomaly %s not found", id))
	}
}
