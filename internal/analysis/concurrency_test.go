package analysis_test

import (
	"fmt"
	"os"
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

// identityGatedLLM is a gated LLM double that parks the enrichment loop inside
// its first GenerateExplanation call and reports the exact evidence it was
// handed. The narrative it returns encodes that evidence's identity, so a
// narrative that lands on a different anomaly is detectable by exact string
// comparison instead of by guessing.
type identityGatedLLM struct {
	firstCall chan models.Evidence // buffered(1): evidence of the first call
	release   chan struct{}        // closed by the test to unblock every call
	once      sync.Once
}

func newIdentityGatedLLM() *identityGatedLLM {
	return &identityGatedLLM{
		firstCall: make(chan models.Evidence, 1),
		release:   make(chan struct{}),
	}
}

func (l *identityGatedLLM) GenerateExplanation(e models.Evidence) (string, error) {
	l.once.Do(func() { l.firstCall <- e })
	<-l.release
	return narrativeForEvidence(e), nil
}

// narrativeForEvidence encodes the anomaly identity into the narrative itself,
// which is what makes a cross-anomaly write observable.
func narrativeForEvidence(e models.Evidence) string {
	return "narrative for " + evidenceIdentity(e)
}

// evidenceIdentity is the composite id the API already uses for an anomaly:
// meter id plus detection timestamp.
func evidenceIdentity(e models.Evidence) string {
	return fmt.Sprintf("%s@%s", e.Anomaly.MeterID, e.Anomaly.Timestamp.Format(time.RFC3339))
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// snapshotGatedLLM parks only its FIRST GenerateExplanation call and encodes the
// full deterministic signature of the evidence it was handed into the narrative
// it returns. Later calls are not gated, so the test can drive a second Enrich
// to completion while the first one is still parked on its opening call.
type snapshotGatedLLM struct {
	firstCall chan models.Evidence // buffered(1): evidence of the first call
	release   chan struct{}        // closed by the test to unblock the first call
	once      sync.Once
}

func newSnapshotGatedLLM() *snapshotGatedLLM {
	return &snapshotGatedLLM{
		firstCall: make(chan models.Evidence, 1),
		release:   make(chan struct{}),
	}
}

func (l *snapshotGatedLLM) GenerateExplanation(e models.Evidence) (string, error) {
	parked := false
	l.once.Do(func() {
		parked = true
		l.firstCall <- e
	})
	if parked {
		<-l.release
	}
	return narrativeFromEvidenceSignature(e), nil
}

// narrativeFromEvidenceSignature encodes the anomaly identity AND the
// deterministic fields that distinguish two generations of the same anomaly.
// The narrative only equals the one recomputed from the evidence it sits on when
// it was generated from that exact deterministic payload, which is what makes a
// write from a superseded snapshot observable.
func narrativeFromEvidenceSignature(e models.Evidence) string {
	return fmt.Sprintf("%s consumption=%.3f delta=%.6f explanation=%q",
		evidenceIdentity(e), e.Anomaly.Raw.Consumption, e.Anomaly.Delta, e.Explanation)
}

// TestSupersededSnapshotDoesNotOverwriteSameAnomaly is the regression test for
// the second half of R3-001. TestCrossAnomalyNarrative only covers the case
// where the republished snapshot has DIFFERENT anomaly identities at every
// index; an identity-only write-back guard already rejects that. This test
// covers the case the identity guard cannot see: two Detect generations publish
// the SAME anomaly identity (same meter id and timestamp) with CHANGED
// deterministic fields, so an identity-only guard accepts the stale narrative.
//
// The first Enrich is parked on its opening LLM call. While it is parked the
// source CSV is rewritten so a second Detect publishes the same anomaly with a
// different consumption/delta/explanation, and that fresh snapshot is enriched
// to completion. Only then is the parked loop released. Its narrative was
// generated from a superseded snapshot and must be discarded instead of
// overwriting the newer narrative that shares its anomaly identity.
func TestSupersededSnapshotDoesNotOverwriteSameAnomaly(t *testing.T) {
	dir := t.TempDir()
	readingsPath := filepath.Join(dir, "readings.csv")
	eventsPath := filepath.Join(dir, "events.csv")
	writeTestFile(t, eventsPath, "event_id,event_type,start_time,end_time,description\n")

	const header = "meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor,status\n"
	// Every generation describes one meter with exactly one sustained anomaly at
	// 2026-09-01 02:00:00. Only the spike magnitude changes, so the meter id and
	// the timestamp - the composite anomaly id - stay identical while the
	// deterministic delta and explanation change.
	writeGeneration := func(spikeKWh float64) {
		t.Helper()
		writeTestFile(t, readingsPath, header+
			"M-SAME,2026-09-01 00:00:00,10,220,45,0.95,OK\n"+
			"M-SAME,2026-09-01 01:00:00,10,220,45,0.95,OK\n"+
			fmt.Sprintf("M-SAME,2026-09-01 02:00:00,%.0f,220,45,0.95,OK\n", spikeKWh))
	}

	llm := newSnapshotGatedLLM()
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

	writeGeneration(30)
	if err := orch.Detect(); err != nil {
		t.Fatalf("Detect returned an error: %v", err)
	}
	old := orch.Evidence()
	if len(old) != 1 {
		t.Fatalf("test setup: expected generation 1 to publish exactly 1 anomaly, got %d", len(old))
	}

	// Always release the stub so a failed assertion cannot leak the Enrich
	// goroutine or hang the test binary.
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(llm.release) }) }
	defer release()

	enrichDone := make(chan struct{})
	go func() {
		defer close(enrichDone)
		orch.Enrich()
	}()

	// Park generation 1's enrichment inside its first LLM call.
	var parked models.Evidence
	select {
	case parked = <-llm.firstCall:
	case <-time.After(10 * time.Second):
		t.Fatal("LLM enrichment never entered its first call")
	}

	// Rewrite the source and republish, exactly as a concurrent
	// POST /api/ai/analyze -> Run -> Detect would. The new generation keeps the
	// same anomaly identity but carries different deterministic fields.
	writeGeneration(40)
	if err := orch.Detect(); err != nil {
		t.Fatalf("second Detect returned an error: %v", err)
	}
	fresh := orch.Evidence()
	if len(fresh) != len(old) {
		t.Fatalf("test setup: expected both generations to publish the same item count, got %d and %d", len(old), len(fresh))
	}
	if evidenceIdentity(parked) != evidenceIdentity(fresh[0]) {
		t.Fatalf("test setup: expected the same anomaly identity across generations, got %s and %s",
			evidenceIdentity(parked), evidenceIdentity(fresh[0]))
	}
	if parked.Anomaly.Delta == fresh[0].Anomaly.Delta {
		t.Fatalf("test setup: expected the deterministic fields to change across generations, both have delta %.6f",
			fresh[0].Anomaly.Delta)
	}

	// Enrich the fresh snapshot to completion while generation 1 stays parked.
	orch.Enrich()
	if got := orch.Evidence()[0].LLMText; got != narrativeFromEvidenceSignature(fresh[0]) {
		t.Fatalf("test setup: fresh enrichment published %q, want %q", got, narrativeFromEvidenceSignature(fresh[0]))
	}

	// Release generation 1: its write-back must be abandoned because the
	// snapshot it narrates is no longer the published one.
	release()
	select {
	case <-enrichDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Enrich did not complete")
	}

	final := orch.Evidence()
	if len(final) != 1 {
		t.Fatalf("expected exactly 1 published anomaly, got %d", len(final))
	}
	if final[0].LLMText == "" {
		t.Fatal("the fresh narrative was lost")
	}
	if want := narrativeFromEvidenceSignature(final[0]); final[0].LLMText != want {
		t.Errorf("evidence[0] (%s) carries a narrative generated from a superseded snapshot: got %q, want %q",
			evidenceIdentity(final[0]), final[0].LLMText, want)
	}
}

// TestCrossAnomalyNarrative is the regression test for R3-001. It reproduces a
// concurrent reanalysis (POST /api/ai/analyze -> Run -> Detect) landing while a
// background Enrich loop is still in flight, and asserts the invariant that a
// non-empty narrative must belong to the anomaly it sits on.
//
// The enrichment loop is parked inside its first LLM call, the evidence slice is
// then republished from a second dataset whose items have different identities
// at every index, and only then is the loop released. On the pre-fix code the
// index-only length guard lets the write-back land, so the narrative generated
// for M-OLD is stored on M-NEW and the invariant below fails.
func TestCrossAnomalyNarrative(t *testing.T) {
	dir := t.TempDir()
	readingsOld := filepath.Join(dir, "readings_old.csv")
	readingsNew := filepath.Join(dir, "readings_new.csv")
	eventsPath := filepath.Join(dir, "events.csv")

	const header = "meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor,status\n"
	// Both datasets have the same shape and therefore produce the same number of
	// evidence items in the same order, but they describe different meters on
	// different days. Item i of the new snapshot is never item i of the old one,
	// so any narrative carried over by index is provably mismatched.
	writeTestFile(t, readingsOld, header+
		"M-OLD,2026-09-01 00:00:00,10,220,45,0.95,OK\n"+
		"M-OLD,2026-09-01 01:00:00,10,220,45,0.95,OK\n"+
		"M-OLD,2026-09-01 02:00:00,100,220,45,0.95,OK\n")
	writeTestFile(t, readingsNew, header+
		"M-NEW,2026-09-02 00:00:00,10,220,45,0.95,OK\n"+
		"M-NEW,2026-09-02 01:00:00,10,220,45,0.95,OK\n"+
		"M-NEW,2026-09-02 02:00:00,100,220,45,0.95,OK\n")
	writeTestFile(t, eventsPath, "event_id,event_type,start_time,end_time,description\n")

	llm := newIdentityGatedLLM()
	orch := analysis.NewOrchestrator(
		csv.NewLoader(readingsOld, eventsPath),
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
	old := orch.Evidence()
	if len(old) == 0 {
		t.Fatal("the first dataset published no evidence")
	}

	// Always release the stub so a failed assertion cannot leak the Enrich
	// goroutine or hang the test binary.
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(llm.release) }) }
	defer release()

	enrichDone := make(chan struct{})
	go func() {
		defer close(enrichDone)
		orch.Enrich()
	}()

	// Park the enrichment loop inside its first LLM call. It is blocked before
	// any write-back, so the evidence is still the old snapshot.
	var parked models.Evidence
	select {
	case parked = <-llm.firstCall:
	case <-time.After(10 * time.Second):
		t.Fatal("LLM enrichment never entered its first call")
	}

	// While the loop is parked, republish a different evidence slice, exactly as
	// a concurrent POST /api/ai/analyze would through Run -> Detect.
	orch.Loader.ReadingsPath = readingsNew
	if err := orch.Detect(); err != nil {
		t.Fatalf("second Detect returned an error: %v", err)
	}
	fresh := orch.Evidence()
	if len(fresh) != len(old) {
		t.Fatalf("test setup: expected both datasets to publish the same item count, got %d and %d", len(old), len(fresh))
	}
	if evidenceIdentity(parked) == evidenceIdentity(fresh[0]) {
		t.Fatalf("test setup: republished item 0 (%s) equals the parked item, so no mismatch is reproducible", evidenceIdentity(fresh[0]))
	}

	// Release the loop: every snapshotted item now attempts its write-back.
	release()
	select {
	case <-enrichDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Enrich did not complete")
	}

	// The invariant: a non-empty narrative must have been generated for the very
	// anomaly it is attached to, never for the one that used to sit at that index.
	final := orch.Evidence()
	for i, e := range final {
		if e.LLMText == "" {
			continue
		}
		if want := narrativeForEvidence(e); e.LLMText != want {
			t.Errorf("evidence[%d] (%s) carries a narrative generated for a different anomaly: got %q, want %q (or empty)",
				i, evidenceIdentity(e), e.LLMText, want)
		}
	}
}
