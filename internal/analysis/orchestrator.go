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

	// mu serialises Run and protects evidence/loaded, so the pipeline can be
	// re-triggered from an HTTP handler while other goroutines read the current
	// evidence.
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

// Run executes the whole pipeline. It loads the files, processes them and
// stores the resulting evidence internally.
//
// Run is idempotent with respect to the in-memory repositories: the reading and
// event repositories are append-only, so storing the loaded dataset on every
// run would duplicate every reading and event. The repositories are populated
// once and kept as the read model behind the meter endpoints; every call still
// reloads the files and recomputes the evidence, which is what
// POST /api/ai/analyze relies on.
func (o *Orchestrator) Run() error {
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

	o.mu.Lock()
	defer o.mu.Unlock()

	// Populate the append-only repositories exactly once, no matter how many
	// times the pipeline runs.
	if !o.loaded {
		o.ReadingRepo.AddMany(readings)
		o.EventRepo.AddMany(events)
		o.loaded = true
	}

	// Deterministic pipeline. The LLM runs strictly after detection,
	// classification and scoring; it only narrates the result.
	good := o.QualityChecker.Check(readings)
	baseline := o.BaselineCalc.Calculate(good)
	candidates := o.Detector.Detect(good, baseline)
	correlated := o.Correlator.Correlate(candidates, events)
	classified := o.Classifier.Classify(correlated)
	scored := o.Scorer.Score(classified)
	o.evidence = o.EvidenceBuilder.Build(scored)
	if o.LLM != nil {
		for i := range o.evidence {
			txt, err := o.LLM.GenerateExplanation(o.evidence[i])
			if err != nil {
				continue
			}
			o.evidence[i].LLMText = txt
		}
	}
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
