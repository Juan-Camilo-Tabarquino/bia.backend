package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
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
	// Priority is the deterministic investigation order computed by the
	// scorer: 1 is the most urgent. It is published as-is and never recomputed
	// in the API layer.
	Priority int `json:"priority"`
	// Baseline is the per-meter statistics this anomaly was compared against.
	Baseline BaselineDTO `json:"baseline"`
	// ConsumptionChangePct, VoltageChangePct, CurrentChangePct and
	// PowerFactorChangePct are the signed per-signal changes against the
	// baseline means, in percent.
	ConsumptionChangePct float64 `json:"consumption_change_pct"`
	VoltageChangePct     float64 `json:"voltage_change_pct"`
	CurrentChangePct     float64 `json:"current_change_pct"`
	PowerFactorChangePct float64 `json:"power_factor_change_pct"`
	// CorrelatedEvents lists the operational events matched to this anomaly.
	// It is always serialised as an array, never null.
	CorrelatedEvents []AnomalyEventDTO `json:"correlated_events"`
	// DataQuality flags an anomaly that is really a measurement-quality
	// problem rather than a consumption deviation.
	DataQuality DataQualityDTO `json:"data_quality"`
	// LLMAnalysis carries the LLM-generated interpretation of the anomaly.
	// It is EMPTY when no LLM provider is configured or when the provider call
	// failed, and it is NOT the source of truth for the classification: type,
	// severity, confidence, reason and recommended_action always come from the
	// deterministic pipeline. Because the field is tagged omitempty, an empty
	// narrative is left out of the JSON entirely.
	LLMAnalysis string `json:"llm_analysis,omitempty"`
}

// BaselineDTO is the per-meter baseline an anomaly was compared against.
type BaselineDTO struct {
	Mean            float64 `json:"mean"`
	StdDev          float64 `json:"stddev"`
	Count           int     `json:"count"`
	VoltageMean     float64 `json:"voltage_mean"`
	CurrentMean     float64 `json:"current_mean"`
	PowerFactorMean float64 `json:"power_factor_mean"`
}

// AnomalyEventDTO is the API representation of a correlated operational event.
type AnomalyEventDTO struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Description string `json:"description"`
}

// DataQualityDTO marks an anomaly as a measurement-quality problem and carries
// the human-readable deterministic reason for that flag.
type DataQualityDTO struct {
	Flagged bool   `json:"flagged"`
	Reason  string `json:"reason"`
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
// recommendation) plus the statistical evidence the frontend needs: the
// deterministic priority, the baseline, the per-signal changes, the correlated
// events and the data-quality flag.
func newAnomalyDTO(ev models.Evidence) AnomalyDTO {
	status := string(ev.Status)
	if status == "" {
		if ev.Correlation.Explains {
			status = string(models.StatusExplained)
		} else {
			status = string(models.StatusUnexplained)
		}
	}
	// CorrelatedEvents publishes the matched events that actually explain this
	// anomaly, always as an array. An anomaly whose correlation does not explain
	// the deviation reports an empty array: the time-overlapping events that did
	// not explain it (for example an UNKNOWN event) must never be presented by
	// the UI as a cause.
	events := make([]AnomalyEventDTO, 0, len(ev.Correlation.Events))
	if ev.Correlation.Explains {
		for _, e := range ev.Correlation.Events {
			events = append(events, AnomalyEventDTO{
				ID:          e.ID,
				Type:        string(e.Type),
				Start:       e.Start.UTC().Format(time.RFC3339),
				End:         e.End.UTC().Format(time.RFC3339),
				Description: e.Description,
			})
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
		Priority:          ev.Priority,
		Baseline: BaselineDTO{
			Mean:            ev.Anomaly.Baseline.Mean,
			StdDev:          ev.Anomaly.Baseline.StdDev,
			Count:           ev.Anomaly.Baseline.Count,
			VoltageMean:     ev.Anomaly.Baseline.VoltageMean,
			CurrentMean:     ev.Anomaly.Baseline.CurrentMean,
			PowerFactorMean: ev.Anomaly.Baseline.PowerFactorMean,
		},
		ConsumptionChangePct: ev.Anomaly.ConsumptionChangePct,
		VoltageChangePct:     ev.Anomaly.VoltageChangePct,
		CurrentChangePct:     ev.Anomaly.CurrentChangePct,
		PowerFactorChangePct: ev.Anomaly.PowerFactorChangePct,
		CorrelatedEvents:     events,
		DataQuality: DataQualityDTO{
			Flagged: ev.Anomaly.Kind == models.KindDataQuality,
			Reason:  ev.Anomaly.Reason,
		},
		LLMAnalysis: ev.LLMText,
	}
}

// sortedEvidence returns a copy of the evidence ordered for the UI: the most
// urgent anomaly first (priority ascending, 1 = most urgent), then oldest
// first, then by meter id. The three keys give a total order, so the list
// endpoint and the stored analysis result always agree on the sequence.
func sortedEvidence(evidence []models.Evidence) []models.Evidence {
	out := append([]models.Evidence(nil), evidence...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		if !out[i].Anomaly.Timestamp.Equal(out[j].Anomaly.Timestamp) {
			return out[i].Anomaly.Timestamp.Before(out[j].Anomaly.Timestamp)
		}
		return out[i].Anomaly.MeterID < out[j].Anomaly.MeterID
	})
	return out
}

// anomalyDTOs maps a whole evidence slice onto the API representation, sorted
// by the deterministic UI order. Both the anomalies list endpoint and the
// stored analysis result go through this single mapping, so they inherit the
// same fields and the same ordering.
func anomalyDTOs(evidence []models.Evidence) []AnomalyDTO {
	sorted := sortedEvidence(evidence)
	out := make([]AnomalyDTO, 0, len(sorted))
	for _, ev := range sorted {
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
