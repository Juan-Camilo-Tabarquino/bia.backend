package handlers

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
	// AnomalyDTO now carries a slice (correlated_events), so it is no longer
	// comparable with ==; compare the list entry and the detail body by value.
	if !reflect.DeepEqual(detail, list[0]) {
		t.Fatalf("list entry and detail body disagree:\nlist   %+v\ndetail %+v", list[0], detail)
	}

	if list[0].ID != wantID {
		t.Fatalf("list id = %q, expected %q", list[0].ID, wantID)
	}
	if list[0].Type != "REAL_ANOMALY" || list[0].Severity != "HIGH" || list[0].Confidence != 0.97 {
		t.Fatalf("DTO did not carry the real values: %+v", list[0])
	}
}

// TestNotFoundErrorBodiesAreSpanish pins the user-visible 404 body of every
// handler branch that rejects a request path before it can parse an id. The
// frontend renders error.message via getErrorMessage, so these strings are
// user-facing copy rather than developer-only text.
func TestNotFoundErrorBodiesAreSpanish(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		target  string
		want    string
	}{
		{"analysis", AnalysisGET, "/wrong", `{"error":"análisis no encontrado"}`},
		{"meter", MeterDetail(nil), "/wrong", `{"error":"medidor no encontrado"}`},
		{"anomaly", AnomalyDetailByID(nil), "/wrong", `{"error":"anomalía no encontrada"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tc.target, nil)
			tc.handler(rr, req)
			if rr.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d", rr.Code)
			}
			if got := strings.TrimSpace(rr.Body.String()); got != tc.want {
				t.Fatalf("expected error body %s, got %s", tc.want, got)
			}
		})
	}
}
