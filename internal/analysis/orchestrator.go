package analysis

import (
	"log"
	"sync"

	"github.com/neuralium/ai-energy/internal/data/csv"
	"github.com/neuralium/ai-energy/internal/data/memory"
	"github.com/neuralium/ai-energy/internal/domain/models"
)

// Orchestrator ties all stages together.
// It loads data from CSVs, runs the deterministic pipeline and holds the
// generated evidence for API consumption.
type Orchestrator struct {
	Loader      *csv.Loader
	ReadingRepo *memory.ReadingRepo
	EventRepo   *memory.EventRepo

	QualityChecker  QualityChecker
	BaselineCalc    BaselineCalc
	Detector        Detector
	Correlator      Correlator
	Classifier      Classifier
	Scorer          Scorer
	LLM             LLMClient
	EvidenceBuilder EvidenceBuilder

	// mu protects evidence/loaded/generation. It is held only for short
	// mutations and brief reads: Detect publishes the deterministic snapshot
	// under a short write lock and Enrich writes each LLM narrative under a
	// short write lock per item. It is deliberately NOT held across the slow LLM
	// calls, so a concurrent Evidence() read is never blocked for the duration of
	// the enrichment.
	mu       sync.RWMutex
	evidence []models.Evidence
	loaded   bool
	// generation is a monotonically increasing counter bumped every time Detect
	// publishes a new evidence snapshot. It identifies the snapshot as a whole,
	// not an individual item: two consecutive snapshots may contain the same
	// anomaly identity (meter id and timestamp) with different deterministic
	// fields, so item identity alone cannot tell Enrich that the data it narrated
	// has been superseded. A changed generation is the proof that a republish
	// happened. Protected by mu.
	generation uint64
}

func NewOrchestrator(loader *csv.Loader, rrepo *memory.ReadingRepo, erepo *memory.EventRepo,
	qc QualityChecker, bc BaselineCalc, det Detector, co Correlator, cl Classifier, sc Scorer, llm LLMClient, eb EvidenceBuilder) *Orchestrator {
	return &Orchestrator{Loader: loader, ReadingRepo: rrepo, EventRepo: erepo,
		QualityChecker: qc, BaselineCalc: bc, Detector: det, Correlator: co,
		Classifier: cl, Scorer: sc, LLM: llm, EvidenceBuilder: eb}
}

// Detect runs the fast, deterministic stage of the pipeline: it loads the
// source files, populates the append-only repositories, runs every
// deterministic stage and publishes the resulting evidence snapshot.
//
// Detect is the startup-critical half of the pipeline. It must stay fast so the
// API port can open immediately; the slow narrative work lives in Enrich. The
// expensive deterministic computation happens before the lock, and only the
// final publication of the evidence slice is guarded, so the write lock is held
// for an instant rather than for the whole pipeline.
//
// Detect is idempotent with respect to the in-memory repositories: the reading
// and event repositories are append-only, so storing the loaded dataset on
// every run would duplicate every reading and event. The repositories are
// populated once and kept as the read model behind the meter endpoints; every
// call still reloads the files and recomputes the evidence, which is what
// POST /api/ai/analyze relies on.
func (o *Orchestrator) Detect() error {
	// Load from disk on every run so a re-run has fresh source data.
	readings, err := o.Loader.LoadReadings()
	if err != nil {
		return err
	}
	log.Printf("Loaded %d readings from %s", len(readings), o.Loader.ReadingsPath)
	events, err := o.Loader.LoadEvents()
	if err != nil {
		return err
	}
	log.Printf("Loaded %d events from %s", len(events), o.Loader.EventsPath)

	// Deterministic pipeline. It reads only local values, so it runs outside
	// the lock: the LLM never participates in detection, classification or
	// scoring; it only narrates the result later, in Enrich.
	good := o.QualityChecker.Check(readings)
	baseline := o.BaselineCalc.Calculate(good)
	candidates := o.Detector.Detect(good, baseline)
	correlated := o.Correlator.Correlate(candidates, events)
	classified := o.Classifier.Classify(correlated)
	scored := o.Scorer.Score(classified)
	built := o.EvidenceBuilder.Build(scored)

	o.mu.Lock()
	// Populate the append-only repositories exactly once, no matter how many
	// times the pipeline runs.
	if !o.loaded {
		o.ReadingRepo.AddMany(readings)
		o.EventRepo.AddMany(events)
		o.loaded = true
	}
	// Publish the complete deterministic snapshot and mark it as a new
	// generation. Hold the write lock only for these assignments: readers must
	// never wait on the LLM stage below. Bumping the generation inside the same
	// critical section as the publish is what makes it a reliable snapshot
	// marker: any Enrich loop that captured an older generation is thereby known
	// to be narrating superseded data, no matter how similar the two snapshots
	// look item by item.
	o.evidence = built
	o.generation++
	o.mu.Unlock()
	return nil
}

// Enrich runs the slow, narrative LLM stage over the deterministic evidence
// published by Detect.
//
// The write lock is deliberately NOT held across the LLM loop. The LLM may take
// tens of seconds per item, and Evidence() reads the same lock, so holding it
// here would turn a fast startup into a request stall on every read endpoint:
// the connection would be accepted but the handler would block on the mutex.
//
// Instead the evidence is snapshotted under a brief read lock, every LLM call
// happens outside any lock, and each item's narrative is written back under a
// short write lock. Concurrent readers therefore observe the LLM text appearing
// progressively instead of waiting for the whole loop to finish.
//
// The write-back is guarded by the published snapshot generation, not by the
// item's identity. A concurrent reanalysis (POST /api/ai/analyze -> Run ->
// Detect) can republish evidence while this loop is still running, and the new
// snapshot may carry the very same anomaly identity (meter id and timestamp) at
// the same index while describing different deterministic values. An
// identity-only guard would accept that case and attach a narrative generated
// from the old numbers to a payload that no longer matches it. Comparing
// generations rejects it: a republish always bumps the counter, so any write
// from a superseded snapshot is dropped. The invariant is "same published
// snapshot", not "same anomaly id".
//
// Do not "optimize" this by collapsing the loop back under a single lock: that
// reintroduces the startup stall this split exists to remove.
func (o *Orchestrator) Enrich() {
	o.mu.RLock()
	if o.LLM == nil || len(o.evidence) == 0 {
		o.mu.RUnlock()
		return
	}
	// Snapshot the evidence so the slow LLM calls read stable inputs without
	// holding any lock, and capture the generation that identifies the snapshot
	// the copy came from. The returned slice is a copy, so the loop cannot race a
	// concurrent reader while it iterates.
	snapshot := make([]models.Evidence, len(o.evidence))
	copy(snapshot, o.evidence)
	snapshotGeneration := o.generation
	o.mu.RUnlock()

	for i := range snapshot {
		// Early exit: once Detect has published a new snapshot, every remaining
		// narrative in this loop would belong to the superseded one, so there is no
		// point generating them. This only stops wasted LLM work; the write-back
		// below is the correctness guard, because a republish can also happen
		// during the LLM call itself.
		o.mu.RLock()
		superseded := o.generation != snapshotGeneration
		o.mu.RUnlock()
		if superseded {
			return
		}

		txt, err := o.LLM.GenerateExplanation(snapshot[i])
		if err != nil {
			continue
		}
		// Publish this item's narrative under a short write lock, but only while
		// the snapshot it was generated from is still the published one. The
		// generation check closes the window opened by the LLM call above: if a
		// reanalysis republished evidence in the meantime, this narrative describes
		// superseded numbers and is discarded.
		//
		// When the generation still matches, no republish happened since the
		// snapshot was copied, so index i provably still holds the very item this
		// narrative was generated for. No separate anomaly-identity check is needed
		// or wanted here; the generation is the strictly stronger invariant.
		o.mu.Lock()
		if o.generation == snapshotGeneration {
			o.evidence[i].LLMText = txt
		}
		o.mu.Unlock()
	}
}

// Run executes the whole pipeline as the deterministic Detect stage followed by
// the narrative Enrich stage. It preserves the original post-conditions: when
// Run returns, the evidence is published and carries the LLM text. The existing
// call sites keep their exact observable semantics.
func (o *Orchestrator) Run() error {
	if err := o.Detect(); err != nil {
		return err
	}
	o.Enrich()
	return nil
}

// Evidence returns a copy of the last run's evidence so callers cannot mutate
// the orchestrator's internal state or observe a partially written slice.
func (o *Orchestrator) Evidence() []models.Evidence {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]models.Evidence, len(o.evidence))
	copy(out, o.evidence)
	return out
}
