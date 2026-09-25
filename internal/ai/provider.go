package ai

import (
    "context"
    "errors"
    "fmt"

    "github.com/neuralium/ai-energy/internal/analysis"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

// LLMProvider abstracts a generic LLM.
type LLMProvider interface {
    Generate(ctx context.Context, input LLMInput) (LLMOutput, error)
}

// LLMInput/Output are lightweight structs.
type LLMInput struct {
    Prompt string
}

type LLMOutput struct {
    Text string
}

// OllamaProviderStub is a placeholder that always returns a fixed message.
func NewOllamaStub() *OllamaProviderStub {
    return &OllamaProviderStub{}
}

type OllamaProviderStub struct{}

func (o *OllamaProviderStub) Generate(ctx context.Context, input LLMInput) (LLMOutput, error) {
    return LLMOutput{Text: fmt.Sprintf("[OllamaStub] %s", input.Prompt)}, nil
}

// ErrProviderNotImplemented is returned when the real provider is selected by
// configuration but no vendor client has been wired in yet. This repository
// deliberately does not pin a vendor API contract, so the provider does not
// fabricate one.
var ErrProviderNotImplemented = errors.New("ai: real LLM provider is not implemented")

// RealProvider is the configuration-selected implementation of the narration
// contract the analysis pipeline consumes (analysis.LLMClient).
//
// Construction is pure and offline: it stores the API key only, performs no
// network access and cannot panic, so a misconfigured key never breaks process
// start-up. Because the deterministic pipeline already classifies and explains
// every anomaly, a failing real provider never changes severity, confidence,
// or the reason/recommendation text; it only means LLMText stays empty.
type RealProvider struct {
    apiKey string
}

// NewRealProvider builds the real provider from a non-empty API key. It does
// not validate the key or contact any endpoint.
func NewRealProvider(apiKey string) *RealProvider {
    return &RealProvider{apiKey: apiKey}
}

// GenerateExplanation implements analysis.LLMClient. It fails at call time (not
// at construction) with a wrapped ErrProviderNotImplemented and never returns
// fabricated narration.
func (p *RealProvider) GenerateExplanation(evidence models.Evidence) (string, error) {
    return "", fmt.Errorf("%w (meter %s)", ErrProviderNotImplemented, evidence.Anomaly.MeterID)
}

// NewProvider selects the LLM implementation from configuration. A non-empty
// API key selects the real provider; an empty or absent key keeps the
// deterministic mock, which is the default so the offline pipeline and the
// dataset acceptance test run with no credentials configured.
func NewProvider(apiKey string) analysis.LLMClient {
    if apiKey == "" {
        return analysis.NewMockLLM()
    }
    return NewRealProvider(apiKey)
}
