package services

import (
    "context"
    "encoding/json"
    "io"
    "os"

    "github.com/neuralium/ai-energy/internal/domain/models"
)

// EventLoader reads operational events from a JSON file.
// The JSON format is an array of objects matching models.Event.
func NewEventLoader(path string) *EventLoader {
    return &EventLoader{path: path}
}

type EventLoader struct{ path string }

func (el *EventLoader) Load(ctx context.Context) ([]models.Event, error) {
    f, err := os.Open(el.path)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    data, err := io.ReadAll(f)
    if err != nil {
        return nil, err
    }
    var evts []models.Event
    if err := json.Unmarshal(data, &evts); err != nil {
        return nil, err
    }
    // Unmarshal already populates time.Time values if the JSON uses RFC3339 format.
    return evts, nil
}
