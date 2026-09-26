package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// capturedRequestRecord holds the request the provider sent to the test server.
// Recording it through a channel keeps the write in the handler goroutine and
// the read in the test goroutine synchronised.
type capturedRequestRecord struct {
	method string
	path   string
	auth   string
	body   []byte
}

// newCapturingServer starts a server that records every request on requests and
// answers with the given status and body.
func newCapturingServer(t *testing.T, status int, responseBody string, requests chan<- capturedRequestRecord) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		requests <- capturedRequestRecord{
			method: r.Method,
			path:   r.URL.Path,
			auth:   r.Header.Get("Authorization"),
			body:   raw,
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, responseBody)
	}))
}

// waitForRequest returns the request the provider sent. The handler signals it
// on the channel before the response is written, so by the time the call
// returns the record is already buffered.
func waitForRequest(t *testing.T, requests <-chan capturedRequestRecord) capturedRequestRecord {
	t.Helper()
	select {
	case req := <-requests:
		return req
	default:
		t.Fatal("provider did not send an HTTP request")
		return capturedRequestRecord{}
	}
}

// chatRequestBody mirrors the native Ollama /api/chat request body.
type chatRequestBody struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// m109Evidence is the M-109 example payload from the product spec.
func m109Evidence() models.Evidence {
	return models.Evidence{
		Anomaly: models.AnomalyCandidate{
			MeterID:              "M-109",
			ConsumptionChangePct: 125.3,
			VoltageChangePct:     -2.7,
			CurrentChangePct:     111.2,
			PowerFactorChangePct: -18.2,
		},
		Confidence: 0.97,
		Type:       models.AnomalyReal,
		Severity:   models.SeverityHigh,
	}
}

// TestRealProviderSendsNativeOllamaChatRequest is the happy path: it pins the
// exact request shape (method, path, auth header, model, stream, message role
// and prompt) and the parsed response.
func TestRealProviderSendsNativeOllamaChatRequest(t *testing.T) {
	const (
		apiKey = "sk-test-key-123"
		model  = "gpt-oss:20b"
		reply  = "respuesta de prueba"
	)

	requests := make(chan capturedRequestRecord, 1)
	server := newCapturingServer(t, http.StatusOK,
		`{"message":{"role":"assistant","content":"`+reply+`"},"done":true,"model":"`+model+`"}`,
		requests)
	defer server.Close()

	evidence := m109Evidence()
	provider := newRealProvider(apiKey, server.URL, model, server.Client(), 5*time.Second)

	text, err := provider.GenerateExplanation(evidence)
	if err != nil {
		t.Fatalf("GenerateExplanation returned error: %v", err)
	}
	if text != reply {
		t.Fatalf("GenerateExplanation text = %q, want %q", text, reply)
	}

	req := waitForRequest(t, requests)
	if req.method != http.MethodPost {
		t.Errorf("request method = %q, want %q", req.method, http.MethodPost)
	}
	if req.path != "/api/chat" {
		t.Errorf("request path = %q, want %q", req.path, "/api/chat")
	}
	if got, want := req.auth, "Bearer "+apiKey; got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}

	var body chatRequestBody
	if err := json.Unmarshal(req.body, &body); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	if body.Model != model {
		t.Errorf("request model = %q, want %q", body.Model, model)
	}
	if body.Stream {
		t.Error("request stream = true, want false")
	}
	if len(body.Messages) != 1 {
		t.Fatalf("request messages length = %d, want 1", len(body.Messages))
	}
	if body.Messages[0].Role != "user" {
		t.Errorf("messages[0].role = %q, want %q", body.Messages[0].Role, "user")
	}

	content := body.Messages[0].Content
	if want := mustJSON(t, BuildAnomalyPayload(evidence)); !strings.Contains(content, want) {
		t.Errorf("prompt does not contain the payload JSON\nprompt:\n%s\nwant substring:\n%s", content, want)
	}
	for _, key := range []string{
		"meter_id", "consumption_change_pct", "voltage_change_pct",
		"current_change_pct", "power_factor_change_pct", "has_operational_event",
		"anomaly_score", "classification", "severity",
	} {
		if !strings.Contains(content, key) {
			t.Errorf("prompt is missing payload key %q", key)
		}
	}
	for _, point := range []string{
		"1. qué está ocurriendo",
		"2. cuáles son las posibles causas",
		"3. qué evidencia respalda cada hipótesis",
		"4. qué debería revisar un operador",
		"5. qué acciones recomienda",
	} {
		if !strings.Contains(content, point) {
			t.Errorf("prompt is missing numbered point %q", point)
		}
	}
	if !strings.Contains(content, "No inventes datos que no estén presentes.") {
		t.Error("prompt is missing the final sentence")
	}
}

// TestRealProviderOmitsAuthorizationWhenKeyIsEmpty proves a local Ollama needs
// no credentials: with an empty key the Authorization header must be absent.
func TestRealProviderOmitsAuthorizationWhenKeyIsEmpty(t *testing.T) {
	requests := make(chan capturedRequestRecord, 1)
	server := newCapturingServer(t, http.StatusOK,
		`{"message":{"role":"assistant","content":"ok"},"done":true}`, requests)
	defer server.Close()

	provider := newRealProvider("", server.URL, "gpt-oss:20b", server.Client(), 5*time.Second)
	if _, err := provider.GenerateExplanation(models.Evidence{}); err != nil {
		t.Fatalf("GenerateExplanation returned error: %v", err)
	}
	if auth := waitForRequest(t, requests).auth; auth != "" {
		t.Fatalf("Authorization header = %q, want it absent when the API key is empty", auth)
	}
}

// TestRealProviderErrorPaths proves every failure returns an error and an empty
// string, so a failing LLM never fabricates narration.
func TestRealProviderErrorPaths(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		responseBody string
	}{
		{name: "non-2xx status", status: http.StatusInternalServerError, responseBody: `{"error":"boom"}`},
		{name: "body is not JSON", status: http.StatusOK, responseBody: "<html>not json</html>"},
		{name: "empty message content", status: http.StatusOK, responseBody: `{"message":{"role":"assistant","content":""},"done":true}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan capturedRequestRecord, 1)
			server := newCapturingServer(t, tc.status, tc.responseBody, requests)
			defer server.Close()

			provider := newRealProvider("sk-test-key", server.URL, "gpt-oss:20b", server.Client(), 5*time.Second)
			text, err := provider.GenerateExplanation(models.Evidence{})
			if err == nil {
				t.Fatal("GenerateExplanation must return an error")
			}
			if text != "" {
				t.Fatalf("GenerateExplanation text = %q, want it empty on error", text)
			}
		})
	}
}

// TestRealProviderBaseURLNormalization proves both accepted host forms and an
// "/api" base URL reach exactly one "/api/chat" path.
func TestRealProviderBaseURLNormalization(t *testing.T) {
	cases := []struct {
		name   string
		suffix string
	}{
		{name: "no trailing slash", suffix: ""},
		{name: "trailing slash", suffix: "/"},
		{name: "ends with /api", suffix: "/api"},
		{name: "ends with /api and trailing slash", suffix: "/api/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan capturedRequestRecord, 1)
			server := newCapturingServer(t, http.StatusOK,
				`{"message":{"role":"assistant","content":"ok"},"done":true}`, requests)
			defer server.Close()

			base := server.URL + tc.suffix
			provider := newRealProvider("", base, "gpt-oss:20b", server.Client(), 5*time.Second)
			if _, err := provider.GenerateExplanation(models.Evidence{}); err != nil {
				t.Fatalf("GenerateExplanation returned error: %v", err)
			}
			if path := waitForRequest(t, requests).path; path != "/api/chat" {
				t.Fatalf("base URL %q produced path %q, want %q", base, path, "/api/chat")
			}
		})
	}
}

// TestPromptRendersM109PayloadVerbatim pins the product prompt wording and logs
// the exact string sent for the M-109 example payload.
func TestPromptRendersM109PayloadVerbatim(t *testing.T) {
	prompt, err := buildPrompt(BuildAnomalyPayload(m109Evidence()))
	if err != nil {
		t.Fatalf("buildPrompt returned error: %v", err)
	}
	t.Logf("M-109 prompt:\n%s", prompt)

	const m109JSON = `{"meter_id":"M-109","consumption_change_pct":125.3,"voltage_change_pct":-2.7,"current_change_pct":111.2,"power_factor_change_pct":-18.2,"has_operational_event":false,"anomaly_score":0.97,"classification":"REAL_ANOMALY","severity":"HIGH"}`
	if !strings.Contains(prompt, m109JSON) {
		t.Errorf("prompt does not embed the M-109 payload JSON")
	}
	const finalSentence = "No inventes datos que no estén presentes."
	if !strings.HasSuffix(prompt, finalSentence) {
		t.Errorf("prompt does not end with %q", finalSentence)
	}
}

// TestNewProviderWithoutKeyReturnsMock proves the default path: an empty or
// absent API key must select the deterministic mock so the offline pipeline and
// the dataset acceptance test keep working with no credentials configured.
func TestNewProviderWithoutKeyReturnsMock(t *testing.T) {
	provider := NewProvider("", "https://ollama.com", "gpt-oss:20b")
	if provider == nil {
		t.Fatal("NewProvider returned nil for an empty key")
	}
	if _, isReal := provider.(*RealProvider); isReal {
		t.Fatal("empty key must select the mock provider, got the real provider")
	}

	// The deterministic mock narrates every classification without failing.
	types := []models.AnomalyType{
		models.AnomalyReal,
		models.AnomalyDataQuality,
		models.AnomalyExplainable,
		models.AnomalyFalsePositive,
		"",
	}
	for _, typ := range types {
		evidence := models.Evidence{
			Type:    typ,
			Anomaly: models.AnomalyCandidate{MeterID: "M-000"},
		}
		text, err := provider.GenerateExplanation(evidence)
		if err != nil {
			t.Fatalf("mock provider must not fail for type %q: %v", typ, err)
		}
		if text == "" {
			t.Fatalf("mock provider returned empty narration for type %q", typ)
		}
	}
}

// TestNewProviderWithKeyReturnsRealProviderWithoutNetwork proves that a
// non-empty key selects the real provider and that construction is offline: the
// closed server would fail any accidental request.
func TestNewProviderWithKeyReturnsRealProviderWithoutNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("NewProvider must not perform network access at construction time")
	}))
	server.Close()

	provider := NewProvider("sk-test-key", server.URL, "gpt-oss:20b")
	if provider == nil {
		t.Fatal("NewProvider returned nil for a non-empty key")
	}
	if _, ok := provider.(*RealProvider); !ok {
		t.Fatalf("non-empty key must select the real provider, got %T", provider)
	}
}

// TestNewProviderNeverPanicsAndNeedsNoNetwork guards the factory contract used
// at process start-up: selecting a provider is pure and cannot panic, no matter
// how the API key is shaped.
func TestNewProviderNeverPanicsAndNeedsNoNetwork(t *testing.T) {
	keys := []string{"", " ", "\t", "sk-test-key", "key-with-unicode-🔑"}
	for _, key := range keys {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("NewProvider(%q) panicked: %v", key, r)
				}
			}()
			if provider := NewProvider(key, "https://ollama.com", "gpt-oss:20b"); provider == nil {
				t.Fatalf("NewProvider(%q) returned nil", key)
			}
		}()
	}
}
