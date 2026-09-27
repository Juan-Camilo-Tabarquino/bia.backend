package analysis_test

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/neuralium/ai-energy/internal/analysis"
	"github.com/neuralium/ai-energy/internal/data/csv"
	"github.com/neuralium/ai-energy/internal/data/memory"
	"github.com/neuralium/ai-energy/internal/domain/models"
)

// blockingLLM is a test double whose GenerateExplanation blocks until the test
// releases it. It lets the test hold the orchestrator in the middle of the
// enrichment loop while it probes Evidence() from another goroutine.
type blockingLLM struct {
	entered chan struct{} // closed the first time GenerateExplanation is called
	release chan struct{} // closed by the test to let every call return
	once    sync.Once
}

func newBlockingLLM() *blockingLLM {
	return &blockingLLM{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (b *blockingLLM) GenerateExplanation(models.Evidence) (string, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return "stub narrative", nil
}

// perCallGatedLLM releases one call at a time so the test can interleave
// Evidence() reads with the enrichment writes deterministically.
type perCallGatedLLM struct {
	proceed chan struct{}
	entered chan struct{}
}

func (g *perCallGatedLLM) GenerateExplanation(models.Evidence) (string, error) {
	<-g.proceed
	g.entered <- struct{}{}
	return "stub narrative", nil
}

// TestEvidenceNotBlockedDuringEnrichment is the governing concurrency test for
// the async-llm-enrichment feature. It proves that Evidence() is not blocked
// while LLM enrichment is in flight: the LLM stub parks the enrichment loop
// mid-flight, and Evidence() must still return promptly.
//
// On the pre-fix code the enrichment loop runs while holding o.mu.Lock(), and
// Evidence() takes o.mu.RLock(), so Evidence() blocks for the whole
// enrichment and this test fails on the assertion deadline.
func TestEvidenceNotBlockedDuringEnrichment(t *testing.T) {
	// The real dataset is used so the deterministic stage produces a non-empty
	// evidence slice and the enrichment loop actually runs.
	readingsPath := filepath.Join("..", "..", "data", "readings.csv")
	eventsPath := filepath.Join("..", "..", "data", "events.csv")

	llm := newBlockingLLM()
	orch := analysis.NewOrchestrator(
		csv.NewLoader(readingsPath, eventsPath),
		memory.NewReadingRepo(),
		memory.NewEventRepo(),
		analysis.NewQualityChecker(),
		analysis.NewBaselineCalculator(),
		analysis.NewAnomalyDetector(),
		analysis.NewEventCorrelator(),
		analysis.NewClassifier(),
		analysis.NewScorer(),
		llm,
		analysis.NewEvidenceBuilder(),
	)

	// Always release the LLM stub so a failed assertion cannot leak the Run
	// goroutine or hang the test binary.
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(llm.release) }) }
	defer release()

	runDone := make(chan error, 1)
	go func() { runDone <- orch.Run() }()

	// Wait until enrichment has started, so Run is parked inside the LLM loop.
	select {
	case <-llm.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("LLM enrichment never started")
	}

	// Evidence() must return while enrichment is still running. The stub stays
	// blocked until release() below, so the only way this can complete is for
	// Evidence() to take a lock that is not held for the duration of the loop.
	evidenceDone := make(chan struct{})
	go func() {
		_ = orch.Evidence()
		close(evidenceDone)
	}()

	select {
	case <-evidenceDone:
		// Correct: readers are not blocked by enrichment.
	case <-time.After(2 * time.Second):
		t.Fatal("Evidence() was blocked while LLM enrichment was running")
	}

	release()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned an error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not complete after the LLM stub was released")
	}
}

// TestDetectDoesNotCallLLM proves the deterministic stage is fully independent
// of the LLM: Detect returns and publishes evidence while the blocking LLM
// double is never even entered. This is the property that keeps startup fast.
func TestDetectDoesNotCallLLM(t *testing.T) {
	readingsPath := filepath.Join("..", "..", "data", "readings.csv")
	eventsPath := filepath.Join("..", "..", "data", "events.csv")

	llm := newBlockingLLM()
	// No release is needed: Detect must not call the LLM, so the stub is never
	// entered and never parked.
	orch := analysis.NewOrchestrator(
		csv.NewLoader(readingsPath, eventsPath),
		memory.NewReadingRepo(),
		memory.NewEventRepo(),
		analysis.NewQualityChecker(),
		analysis.NewBaselineCalculator(),
		analysis.NewAnomalyDetector(),
		analysis.NewEventCorrelator(),
		analysis.NewClassifier(),
		analysis.NewScorer(),
		llm,
		analysis.NewEvidenceBuilder(),
	)

	done := make(chan error, 1)
	go func() { done <- orch.Detect() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Detect returned an error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Detect blocked on the LLM; the deterministic stage must not call it")
	}

	select {
	case <-llm.entered:
		t.Fatal("Detect must not invoke the LLM")
	default:
	}

	evidence := orch.Evidence()
	if len(evidence) == 0 {
		t.Fatal("Detect published no evidence")
	}
	for _, e := range evidence {
		if e.LLMText != "" {
			t.Fatalf("Detect must not populate LLM text, got %q", e.LLMText)
		}
	}
}

// TestEnrichPublishesProgressively interleaves Evidence() reads with the
// enrichment writes and asserts each item's narrative appears after its own
// call, under the race detector.
func TestEnrichPublishesProgressively(t *testing.T) {
	readingsPath := filepath.Join("..", "..", "data", "readings.csv")
	eventsPath := filepath.Join("..", "..", "data", "events.csv")

	llm := &perCallGatedLLM{proceed: make(chan struct{}), entered: make(chan struct{})}
	orch := analysis.NewOrchestrator(
		csv.NewLoader(readingsPath, eventsPath),
		memory.NewReadingRepo(),
		memory.NewEventRepo(),
		analysis.NewQualityChecker(),
		analysis.NewBaselineCalculator(),
		analysis.NewAnomalyDetector(),
		analysis.NewEventCorrelator(),
		analysis.NewClassifier(),
		analysis.NewScorer(),
		llm,
		analysis.NewEvidenceBuilder(),
	)
	if err := orch.Detect(); err != nil {
		t.Fatalf("Detect returned an error: %v", err)
	}
	published := orch.Evidence()
	if len(published) == 0 {
		t.Fatal("Detect published no evidence")
	}

	enrichDone := make(chan struct{})
	go func() {
		defer close(enrichDone)
		orch.Enrich()
	}()

	for i := range published {
		llm.proceed <- struct{}{} // allow the next LLM call
		<-llm.entered             // the call is about to return

		deadline := time.Now().Add(2 * time.Second)
		for {
			evidence := orch.Evidence()
			if len(evidence) > i && evidence[i].LLMText != "" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("LLM text for item %d was never published", i)
			}
			time.Sleep(time.Millisecond)
		}
	}

	select {
	case <-enrichDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Enrich did not complete")
	}
}

// TestEnrichToleratesNilLLM proves Enrich is a no-op when no provider is
// configured, so startup with the deterministic mock never panics.
func TestEnrichToleratesNilLLM(t *testing.T) {
	readingsPath := filepath.Join("..", "..", "data", "readings.csv")
	eventsPath := filepath.Join("..", "..", "data", "events.csv")

	orch := analysis.NewOrchestrator(
		csv.NewLoader(readingsPath, eventsPath),
		memory.NewReadingRepo(),
		memory.NewEventRepo(),
		analysis.NewQualityChecker(),
		analysis.NewBaselineCalculator(),
		analysis.NewAnomalyDetector(),
		analysis.NewEventCorrelator(),
		analysis.NewClassifier(),
		analysis.NewScorer(),
		nil,
		analysis.NewEvidenceBuilder(),
	)
	if err := orch.Detect(); err != nil {
		t.Fatalf("Detect returned an error: %v", err)
	}

	orch.Enrich() // must not panic with a nil LLM

	evidence := orch.Evidence()
	if len(evidence) == 0 {
		t.Fatal("expected evidence to survive a nil-LLM Enrich")
	}
	for _, e := range evidence {
		if e.LLMText != "" {
			t.Fatalf("nil LLM unexpectedly produced text: %q", e.LLMText)
		}
	}
}
