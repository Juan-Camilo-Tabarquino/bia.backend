package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/neuralium/ai-energy/internal/analysis"
	"github.com/neuralium/ai-energy/internal/domain/models"
)

// defaultRequestTimeout bounds a single Ollama chat call so a hung endpoint can
// never block the pipeline forever.
const defaultRequestTimeout = 60 * time.Second

// maxErrorBodyBytes caps how much of a non-2xx response body is quoted in the
// returned error, keeping the message short and out of the log-noise range.
const maxErrorBodyBytes = 512

// explanationPromptTemplate is the exact prompt sent to the LLM.
//
// This wording is a product requirement: the Spanish text, the numbering, the
// blank lines and the closing sentence must be sent verbatim and must not be
// paraphrased, reformatted or "improved". {JSON} is replaced with the JSON
// produced by AnomalyPayload.JSON, never with a hand-rolled object.
const explanationPromptTemplate = `Analiza la siguiente anomalía eléctrica.

Datos:
{JSON}

Explica:
1. qué está ocurriendo
2. cuáles son las posibles causas
3. qué evidencia respalda cada hipótesis
4. qué debería revisar un operador
5. qué acciones recomienda

No inventes datos que no estén presentes.`

// HTTPDoer is the minimal HTTP surface the provider needs. It exists so tests
// can inject the httptest server client instead of reaching the network.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// chatRequest is the request body of the native Ollama /api/chat endpoint.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatMessage is one entry of the Ollama chat conversation.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the single non-streaming object returned when stream is
// false. Only the fields this provider consumes are modelled.
type chatResponse struct {
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
}

// RealProvider is the configuration-selected implementation of the narration
// contract the analysis pipeline consumes (analysis.LLMClient).
//
// It calls the native Ollama chat API. Construction is pure and offline: it
// stores configuration only, performs no network access and cannot panic, so a
// misconfigured key or endpoint never breaks process start-up.
//
// A failure never changes classification, severity, confidence or the
// deterministic reason/recommendation text: it only means LLMText stays empty,
// which the orchestrator already tolerates.
type RealProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  HTTPDoer
	timeout time.Duration
}

// NewRealProvider builds the real provider from configuration. It does not
// validate the key or contact any endpoint.
func NewRealProvider(apiKey, baseURL, model string) *RealProvider {
	return newRealProvider(apiKey, baseURL, model, &http.Client{}, defaultRequestTimeout)
}

// newRealProvider is the injectable constructor used by tests to supply the HTTP
// client and timeout. Production code goes through NewRealProvider.
func newRealProvider(apiKey, baseURL, model string, client HTTPDoer, timeout time.Duration) *RealProvider {
	return &RealProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  client,
		timeout: timeout,
	}
}

// GenerateExplanation implements analysis.LLMClient. It renders the product
// prompt from the anomaly payload, calls POST {baseURL}/api/chat with
// stream=false and returns message.content. Every failure is wrapped with
// actionable context and never panics; a failure only leaves LLMText empty.
func (p *RealProvider) GenerateExplanation(evidence models.Evidence) (string, error) {
	prompt, err := buildPrompt(BuildAnomalyPayload(evidence))
	if err != nil {
		return "", err
	}

	endpoint, err := chatEndpoint(p.baseURL)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(chatRequest{
		Model:    p.model,
		Messages: []chatMessage{{Role: "user", Content: prompt}},
		Stream:   false,
	})
	if err != nil {
		return "", fmt.Errorf("ai: encode Ollama chat request: %w", err)
	}

	timeout := p.timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai: build Ollama chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// A local Ollama needs no credentials; a remote one does. Only send the
	// header when a key is configured, and never log the key value itself.
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: call Ollama chat API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return "", fmt.Errorf("ai: Ollama chat API returned status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(preview)))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ai: read Ollama chat response body: %w", err)
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("ai: decode Ollama chat response as JSON: %w", err)
	}
	if strings.TrimSpace(parsed.Message.Content) == "" {
		return "", errors.New("ai: Ollama chat response has an empty message.content")
	}
	return parsed.Message.Content, nil
}

// buildPrompt renders the product prompt for one anomaly payload. The JSON is
// produced by BuildAnomalyPayload/AnomalyPayload.JSON, never by hand.
func buildPrompt(payload AnomalyPayload) (string, error) {
	raw, err := payload.JSON()
	if err != nil {
		return "", fmt.Errorf("ai: encode anomaly payload as JSON: %w", err)
	}
	return strings.Replace(explanationPromptTemplate, "{JSON}", string(raw), 1), nil
}

// chatEndpoint normalises baseURL and appends the native chat path.
//
// Normalisation handles both accepted host forms: "https://host" and
// "https://host/" must both end in a single "/api/chat", so trailing slashes
// and surrounding whitespace are trimmed first. A base URL that already ends in
// "/api" (for example "https://host/api") then receives only "/chat", which
// prevents the doubled "/api/api/chat" path.
func chatEndpoint(baseURL string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", errors.New("ai: LLM base URL is empty")
	}
	if strings.HasSuffix(base, "/api") {
		return base + "/chat", nil
	}
	return base + "/api/chat", nil
}

// NewProvider selects the LLM implementation from configuration. A non-empty
// API key selects the real provider; an empty or absent key keeps the
// deterministic mock, which is the default so the offline pipeline and the
// dataset acceptance test run with no credentials configured.
func NewProvider(apiKey, baseURL, model string) analysis.LLMClient {
	if apiKey == "" {
		return analysis.NewMockLLM()
	}
	return NewRealProvider(apiKey, baseURL, model)
}
