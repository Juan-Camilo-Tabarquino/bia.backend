package ai

import (
    "context"
    "fmt"
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
