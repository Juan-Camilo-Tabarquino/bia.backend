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

	// mu protects evidence/loaded. It is held only for short mutations and
	// brief reads: Detect publishes the deterministic snapshot under a short
	// write lock and Enrich writes each LLM narrative under a short write lock
	// per item. It is deliberately NOT held across the slow LLM calls, so a
	// concurrent Evidence() read is never blocked for the duration of the
	// enrichment.
	mu       sync.RWMutex
	evidence []models.Evidence
	loaded   bool
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
	// Publish the complete deterministic snapshot. Hold the write lock only for
	// this assignment: readers must never wait on the LLM stage below.
	o.evidence = built
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
// The write-back re-checks the snapshot identity under the write lock. A
// concurrent reanalysis (POST /api/ai/analyze -> Run -> Detect) can republish a
// completely different evidence slice while this loop is still running; the
// index alone is not proof that the item is still the one the narrative was
// generated for, and an index-only guard would attach one meter's narrative to
// another meter's anomaly. sameAnomaly supplies that proof.
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
	// holding any lock. The returned slice is a copy, so the loop cannot race a
	// concurrent reader while it iterates.
	snapshot := make([]models.Evidence, len(o.evidence))
	copy(snapshot, o.evidence)
	o.mu.RUnlock()

	for i := range snapshot {
		txt, err := o.LLM.GenerateExplanation(snapshot[i])
		if err != nil {
			continue
		}
		// Publish this item's narrative under a short write lock. The bound and
		// the identity check keep the write safe if Detect republished the
		// snapshot while this loop was running: the narrative was generated for
		// snapshot[i], so it may only be written onto the same anomaly that still
		// lives at index i. When the republished item differs, the narrative is
		// dropped rather than attached to the wrong meter.
		o.mu.Lock()
		if i < len(o.evidence) && sameAnomaly(o.evidence[i], snapshot[i]) {
			o.evidence[i].LLMText = txt
		}
		o.mu.Unlock()
	}
}

// sameAnomaly reports whether two evidence records describe the same anomaly.
//
// The identity is the pair the API already uses as the composite anomaly id:
// the meter id and the detection timestamp. Enrich uses it to make sure a
// narrative is only ever published onto the anomaly it was generated for, even
// when a concurrent Detect replaced the evidence slice mid-loop.
func sameAnomaly(a, b models.Evidence) bool {
	return a.Anomaly.MeterID == b.Anomaly.MeterID && a.Anomaly.Timestamp.Equal(b.Anomaly.Timestamp)
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
