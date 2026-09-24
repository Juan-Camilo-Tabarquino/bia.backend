package analysis

import "github.com/neuralium/ai-energy/internal/domain/models"

// QualityChecker defines an interface for filtering readings based on quality criteria.
// The interface expects a method that takes a slice of models.Reading and returns a slice
// containing only those readings that are considered valid.
//
// The following implementation removes any reading that has a negative consumption
// or a status other than "OK".
//
// The rules can be customized by modifying the struct or adding additional
// validation logic.
//
// This file is intentionally small to serve as a starting point – replace it with
// a more sophisticated quality stage when you need to check for outliers, gaps
// or other domain‑specific constraints.

type QualityChecker interface {
	Check(readings []models.Reading) []models.Reading
}

// qualityChecker is a concrete implementation of QualityChecker.
// It applies simple rules: consumption must be non‑negative and status must be "OK".
// These rules can be tweaked by customizing the struct in the future.

type qualityChecker struct{}

func NewQualityChecker() QualityChecker {return &qualityChecker{}}

func (q *qualityChecker) Check(readings []models.Reading) []models.Reading {
    // No quality filtering is currently applied; return the input unchanged.
    return readings
}
