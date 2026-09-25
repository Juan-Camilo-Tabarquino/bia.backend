package ai

import (
    "errors"
    "testing"

    "github.com/neuralium/ai-energy/internal/domain/models"
)

// TestNewProviderWithoutKeyReturnsMock proves the default path: an empty or
// absent API key must select the deterministic mock so the offline pipeline and
// the dataset acceptance test keep working with no credentials configured.
func TestNewProviderWithoutKeyReturnsMock(t *testing.T) {
    provider := NewProvider("")
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

// TestNewProviderWithKeyReturnsRealProvider proves that a non-empty key selects
// the real provider instead of the mock.
func TestNewProviderWithKeyReturnsRealProvider(t *testing.T) {
    provider := NewProvider("sk-test-key")
    if provider == nil {
        t.Fatal("NewProvider returned nil for a non-empty key")
    }
    if _, ok := provider.(*RealProvider); !ok {
        t.Fatalf("non-empty key must select the real provider, got %T", provider)
    }
}

// TestRealProviderFailsAtCallTime ensures the real provider is honest: it
// constructs without any network access and fails only when a call is actually
// attempted, with a wrapped sentinel error and no fabricated text.
func TestRealProviderFailsAtCallTime(t *testing.T) {
    real := NewRealProvider("sk-test-key")
    if real == nil {
        t.Fatal("NewRealProvider returned nil")
    }

    text, err := real.GenerateExplanation(models.Evidence{
        Anomaly: models.AnomalyCandidate{MeterID: "M-104"},
    })
    if err == nil {
        t.Fatal("real provider must return an error until a vendor client is wired")
    }
    if !errors.Is(err, ErrProviderNotImplemented) {
        t.Fatalf("error must wrap ErrProviderNotImplemented, got %v", err)
    }
    if text != "" {
        t.Fatalf("real provider must not fabricate narration, got %q", text)
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
            if provider := NewProvider(key); provider == nil {
                t.Fatalf("NewProvider(%q) returned nil", key)
            }
        }()
    }
}
