package analysis

import (
    "context"
    "github.com/neuralium/ai-energy/internal/data/csv"
    "github.com/neuralium/ai-energy/internal/data/memory"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

// Orchestrator ties all stages together.
// It loads data from CSVs, runs the deterministic pipeline and holds the
// generated evidence for API consumption.
type Orchestrator struct {
    Loader *csv.Loader
    ReadingRepo *memory.ReadingRepo
    EventRepo *memory.EventRepo

    QualityChecker QualityChecker
    BaselineCalc   BaselineCalc
    Detector       Detector
    Correlator     Correlator
    Classifier     Classifier
    Scorer Scorer
    LLM LLMClient
    EvidenceBuilder EvidenceBuilder
    evidence []models.Evidence
}

func NewOrchestrator(loader *csv.Loader, rrepo *memory.ReadingRepo, erepo *memory.EventRepo,
    qc QualityChecker, bc BaselineCalc, det Detector, co Correlator, cl Classifier, sc Scorer, llm LLMClient, eb EvidenceBuilder) *Orchestrator {
    return &Orchestrator{Loader: loader, ReadingRepo: rrepo, EventRepo: erepo,
        QualityChecker: qc, BaselineCalc: bc, Detector: det, Correlator: co,
        Classifier: cl, Scorer: sc, LLM: llm, EvidenceBuilder: eb}
}

// Run executes the whole pipeline. It loads the files, processes them
// and stores the resulting evidence internally.
func (o *Orchestrator) Run() error {
    // Load
    readings, err := o.Loader.LoadReadings()
    if err != nil {return err}
    events, err := o.Loader.LoadEvents()
    if err != nil {return err}

    // Store
    o.ReadingRepo.AddMany(readings)
    o.EventRepo.AddMany(events)

    // Context placeholder (required import)
    ctx := context.Background()
    _ = ctx

    // Pipeline
    good := o.QualityChecker.Check(readings)
    baseline := o.BaselineCalc.Calculate(good)
    candidates := o.Detector.Detect(good, baseline)
    correlated := o.Correlator.Correlate(candidates, events)
    // Classification step (result unused currently but kept for future)
    _ = o.Classifier.Classify(correlated)
    // Build evidence from candidates and correlations
    o.evidence = o.EvidenceBuilder.Build(candidates, correlated)
    if o.LLM != nil {
        for i, e := range o.evidence {
            txt, _ := o.LLM.GenerateExplanation(e)
            e.LLMText = txt
            o.evidence[i] = e
        }
    }
    return nil
}

// Evidence returns the last run's evidence. In a real implementation you
// would probably stream the results or store them in a database.
func (o *Orchestrator) Evidence() []models.Evidence {return o.evidence}
