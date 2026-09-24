package services

import (
    "context"
    "github.com/neuralium/ai-energy/internal/domain/models"
)

// Orchestrator coordinates the analysis pipeline.
type Orchestrator struct {
    Loader       DataLoader
    Qualifier    DataQualityChecker
    Baseline     BaselineCalculator
    Detector     AnomalyDetector
    Correlator   EventCorrelator
    Classifier   Classifier
    Scorer       Scorer
    Builder      EvidenceBuilder
    LLMProvider  interface{}
}

// Run executes the full pipeline and returns evidence.
func (o *Orchestrator) Run(ctx context.Context, events []models.Event) ([]models.Evidence, error) {
    readings, err := o.Loader.Load(ctx)
    if err != nil {
        return nil, err
    }
    clean, _, err := o.Qualifier.Check(ctx, readings)
    if err != nil {
        return nil, err
    }
    baseline, err := o.Baseline.Calculate(ctx, clean)
    if err != nil {
        return nil, err
    }
    candidates := o.Detector.Detect(ctx, clean, baseline)
    var evidences []models.Evidence
    for _, cand := range candidates {
        corr, err := o.Correlator.Correlate(ctx, cand, events)
        if err != nil {
            return nil, err
        }
        cls, err := o.Classifier.Classify(ctx, corr)
        if err != nil {
            return nil, err
        }
        prio, err := o.Scorer.Score(ctx, cls, corr)
        if err != nil {
            return nil, err
        }
        ev, err := o.Builder.Build(ctx, corr)
        if err != nil {
            return nil, err
        }
        ev.Priority = prio
        evidences = append(evidences, ev)
    }
    return evidences, nil
}
