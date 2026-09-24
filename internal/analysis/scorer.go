package analysis

import "github.com/neuralium/ai-energy/internal/domain/models"

// Scorer could adjust priority further or compute risk scores.
// For now it simply passes the evidence through.

type Scorer interface {
	Score(evidence []models.Evidence) []models.Evidence
}

// identityScorer preserves scores.

type identityScorer struct{}

func NewScorer() Scorer {return &identityScorer{}}

func (s *identityScorer) Score(evidence []models.Evidence) []models.Evidence {return evidence}
