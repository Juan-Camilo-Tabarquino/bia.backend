package csv

import (
    "encoding/csv"
    "os"
    "strconv"
    "fmt"
    "time"

    "github.com/neuralium/ai-energy/internal/domain/models"
)

// Loader reads readings and events from CSV files.
// The CSV format matches the sample files in assets/.
// The struct holds absolute or relative file paths.

type Loader struct {
    ReadingsPath string
    EventsPath   string
}

func NewLoader(readings, events string) *Loader {
    return &Loader{ReadingsPath: readings, EventsPath: events}
}

func (l *Loader) LoadReadings() ([]models.Reading, error) {
    f, err := os.Open(l.ReadingsPath)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    r := csv.NewReader(f)
    r.TrimLeadingSpace = true
    recs, err := r.ReadAll()
    if err != nil {
        return nil, err
    }
    var out []models.Reading
    // Validate header contains required columns for readings
    header := recs[0]
    colIndex := make(map[string]int)
    for idx, name := range header {
        colIndex[name] = idx
    }
    requiredReadings := []string{"meter_id", "timestamp", "consumption", "voltage", "current", "power_factor"}
    for _, col := range requiredReadings {
        if _, ok := colIndex[col]; !ok {
            return nil, fmt.Errorf("missing required column '%s' in readings CSV", col)
        }
    }
    // Process data rows
    for i, rec := range recs {
        // Skip header row
        if i == 0 {
            continue
        }
        // Validate column count (must have at least the number of required columns)
        if len(rec) < len(header) {
            return nil, fmt.Errorf("missing columns in readings row %d", i+1)
        }
        // Helper to get column by name safely
        get := func(name string) string { return rec[colIndex[name]] }
        // Parse timestamp
        ts, err := time.Parse("2006-01-02 15:04:05", get("timestamp"))
        if err != nil {
            return nil, fmt.Errorf("invalid timestamp in readings row %d: %w", i+1, err)
        }
        // Parse numeric fields
        kwh, err := strconv.ParseFloat(get("consumption"), 64)
        if err != nil {
            return nil, fmt.Errorf("invalid consumption value in readings row %d: %w", i+1, err)
        }
        volt, err := strconv.ParseFloat(get("voltage"), 64)
        if err != nil {
            return nil, fmt.Errorf("invalid voltage value in readings row %d: %w", i+1, err)
        }
        cur, err := strconv.ParseFloat(get("current"), 64)
        if err != nil {
            return nil, fmt.Errorf("invalid current value in readings row %d: %w", i+1, err)
        }
        pf, err := strconv.ParseFloat(get("power_factor"), 64)
        if err != nil {
            return nil, fmt.Errorf("invalid power factor value in readings row %d: %w", i+1, err)
        }
        // Status column may be optional; use index if exists
        out = append(out, models.Reading{MeterID: models.MeterID(get("meter_id")), Timestamp: ts, Consumption: kwh, Voltage: volt, Current: cur, PowerFactor: pf})
    }
    return out, nil
}

func (l *Loader) LoadEvents() ([]models.Event, error) {
    f, err := os.Open(l.EventsPath)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    r := csv.NewReader(f)
    r.TrimLeadingSpace = true
    recs, err := r.ReadAll()
    if err != nil {
        return nil, err
    }
    var out []models.Event
    // Validate header contains required columns for events
    header := recs[0]
    colIndex := make(map[string]int)
    for idx, name := range header {
        colIndex[name] = idx
    }
    requiredEvents := []string{"event_id", "event_type", "start_time", "end_time", "description"}
    for _, col := range requiredEvents {
        if _, ok := colIndex[col]; !ok {
            return nil, fmt.Errorf("missing required column '%s' in events CSV", col)
        }
    }
    // Process data rows
    for i, rec := range recs {
        if i == 0 {
            continue
        }
        if len(rec) < len(header) {
            return nil, fmt.Errorf("missing columns in events row %d", i+1)
        }
        get := func(name string) string { return rec[colIndex[name]] }
        start, err := time.Parse(time.RFC3339, get("start_time"))
        if err != nil {
            return nil, fmt.Errorf("invalid start timestamp in events row %d: %w", i+1, err)
        }
        end, err := time.Parse(time.RFC3339, get("end_time"))
        if err != nil {
            return nil, fmt.Errorf("invalid end timestamp in events row %d: %w", i+1, err)
        }
        out = append(out, models.Event{ID: get("event_id"), Type: models.EventType(get("event_type")), Start: start, End: end, Description: get("description")})
    }
    return out, nil
}
