package memory

import (
    "github.com/neuralium/ai-energy/internal/domain/models"
    "time"
)

// ReadingRepo stores readings in memory.
// It is a simple map from meter ID to slice of readings.
// The methods are not thread‑safe – they are meant for a single‑thread
// pipeline that runs on start‑up.

type ReadingRepo struct {
    store map[string][]models.Reading
}

func NewReadingRepo() *ReadingRepo {
    return &ReadingRepo{store: make(map[string][]models.Reading)}
}

func (r *ReadingRepo) AddMany(readings []models.Reading) {
    for _, rl := range readings {
        key := string(rl.MeterID)
        r.store[key] = append(r.store[key], rl)
    }
}

func (r *ReadingRepo) ReadingsFor(meterIDs []string, from, to *time.Time) []models.Reading {
    var out []models.Reading
    for _, id := range meterIDs {
        for _, rl := range r.store[id] {
            if from != nil && rl.Timestamp.Before(*from) {
                continue
            }
            if to != nil && rl.Timestamp.After(*to) {
                continue
            }
            out = append(out, rl)
        }
    }
    return out
}

func (r *ReadingRepo) AllMeterIDs() []string {
    ids := make([]string, 0, len(r.store))
    for id := range r.store {
        ids = append(ids, id)
    }
    return ids
}

// EventRepo stores events.
type EventRepo struct{ events []models.Event }

func NewEventRepo() *EventRepo { return &EventRepo{events: []models.Event{}} }

func (e *EventRepo) AddMany(ev []models.Event) { e.events = append(e.events, ev...) }

func (e *EventRepo) All() []models.Event { return e.events }
