package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/neuralium/ai-energy/internal/analysis"
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
		w.Header().Set("Content-Type", "application/json")
		ids := rrepo.AllMeterIDs()
		json.NewEncoder(w).Encode(ids)
	}
}

// AnomalyDTO is the stable API representation of a single anomaly.
//
// The anomalies list and the anomaly detail endpoint serialise this exact shape
// and derive the same id, so the UI can move from a list row to the detail view
// using the id it already has. It is intentionally a DTO: models.Evidence stays
// a domain type and does not grow an API-only id field.
type AnomalyDTO struct {
	ID                string  `json:"id"`
	MeterID           string  `json:"meter_id"`
	DetectedAt        string  `json:"detected_at"`
	Type              string  `json:"type"`
	Severity          string  `json:"severity"`
	Confidence        float64 `json:"confidence"`
	Reason            string  `json:"reason"`
	RecommendedAction string  `json:"recommended_action"`
	Status            string  `json:"status"`
}

// AnomalyID derives the deterministic identifier of an evidence record from the
// meter id and the anomaly timestamp. Building it from data already carried by
// the evidence keeps the list and detail endpoints in agreement without an
// extra store. Both endpoints call this single helper.
func AnomalyID(ev models.Evidence) string {
	return string(ev.Anomaly.MeterID) + "-" + ev.Anomaly.Timestamp.UTC().Format(time.RFC3339)
}

// newAnomalyDTO maps a domain evidence record onto the API representation using
// the real pipeline values (type, severity, confidence, status, explanation and
// recommendation).
func newAnomalyDTO(ev models.Evidence) AnomalyDTO {
	status := string(ev.Status)
	if status == "" {
		if ev.Correlation.Explains {
			status = string(models.StatusExplained)
		} else {
			status = string(models.StatusUnexplained)
		}
	}
	return AnomalyDTO{
		ID:                AnomalyID(ev),
		MeterID:           string(ev.Anomaly.MeterID),
		DetectedAt:        ev.Anomaly.Timestamp.UTC().Format(time.RFC3339),
		Type:              string(ev.Type),
		Severity:          string(ev.Severity),
		Confidence:        ev.Confidence,
		Reason:            ev.Explanation,
		RecommendedAction: ev.Recommendation,
		Status:            status,
	}
}

// anomalyDTOs maps a whole evidence slice onto the API representation.
func anomalyDTOs(evidence []models.Evidence) []AnomalyDTO {
	out := make([]AnomalyDTO, 0, len(evidence))
	for _, ev := range evidence {
		out = append(out, newAnomalyDTO(ev))
	}
	return out
}

// writeJSON writes a JSON response with an explicit status code.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeJSONError writes a small JSON error body, so every 404 path is
// machine-readable instead of a plain-text reason phrase.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// List all anomalies (evidence) with the same DTO shape as the detail endpoint.
func Anomalies(orchestrator *analysis.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, anomalyDTOs(orchestrator.Evidence()))
	}
}
