package ai

import (
    "context"
    "fmt"
)

// MockLLMProvider is a deterministic mock implementation of the LLMProvider
// interface, useful for tests and environments without an actual LLM service.
type MockLLMProvider struct{}

// NewMockLLMProvider creates a new instance of MockLLMProvider.
func NewMockLLMProvider() *MockLLMProvider { return &MockLLMProvider{} }

// Generate returns a predictable response that prefixes the prompt with
// "[MockLLM] ". This satisfies the ai.LLMProvider contract.
func (m *MockLLMProvider) Generate(ctx context.Context, input LLMInput) (LLMOutput, error) {
    return LLMOutput{Text: fmt.Sprintf("[MockLLM] %s", input.Prompt)}, nil
}
