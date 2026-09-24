package services

import "github.com/neuralium/ai-energy/internal/data"

// NewCSVLoader provides a services‑level alias for the CSV loader.
func NewCSVLoader(path string) *data.CSVLoader {
    return data.NewCSVLoader(path)
}
