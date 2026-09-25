package handlers

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

func TestHealthHandler(t *testing.T) {
	// Create a new recorder and request.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", nil)

	// Call the Health handler directly.
	Health(rr, req)

	// Verify the status code.
	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	// Verify the response body.
	expected := `{"status":"ok"}`
	if rr.Body.String() != expected {
		t.Fatalf("expected body %s, got %s", expected, rr.Body.String())
	}
}

// TestAnomalyIDIsSharedAndDeterministic proves the list and the detail
// endpoints derive the exact same stable id from the evidence, and that the DTO
// carries the real pipeline values instead of hardcoded placeholders.
func TestAnomalyIDIsSharedAndDeterministic(t *testing.T) {
	evidence := models.Evidence{
		Anomaly: models.AnomalyCandidate{
			MeterID:   "M-109",
			Timestamp: time.Date(2026, 9, 12, 14, 0, 0, 0, time.UTC),
		},
		Type:           models.AnomalyReal,
		Severity:       models.SeverityHigh,
		Confidence:     0.97,
		Status:         models.StatusUnexplained,
		Explanation:    "consumption spike with no operational event",
		Recommendation: "inspect the installation",
	}

	const wantID = "M-109-2026-09-12T14:00:00Z"
	if got := AnomalyID(evidence); got != wantID {
		t.Fatalf("AnomalyID = %q, expected %q", got, wantID)
	}
	if got := AnomalyID(evidence); got != wantID {
		t.Fatalf("AnomalyID is not deterministic: second call returned %q", got)
	}

	// The list endpoint builds its rows through anomalyDTOs...
	list := anomalyDTOs([]models.Evidence{evidence})
	if len(list) != 1 {
		t.Fatalf("expected 1 DTO, got %d", len(list))
	}
	// ...and the detail endpoint builds its body through newAnomalyDTO.
	detail := newAnomalyDTO(evidence)
	if detail != list[0] {
		t.Fatalf("list entry and detail body disagree:\nlist   %+v\ndetail %+v", list[0], detail)
	}

	if list[0].ID != wantID {
		t.Fatalf("list id = %q, expected %q", list[0].ID, wantID)
	}
	if list[0].Type != "REAL_ANOMALY" || list[0].Severity != "HIGH" || list[0].Confidence != 0.97 {
		t.Fatalf("DTO did not carry the real values: %+v", list[0])
	}
}
