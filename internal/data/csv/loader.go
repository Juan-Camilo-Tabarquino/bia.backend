package csv

import (
    "encoding/csv"
    "os"
    "strconv"
    "fmt"
    "strings"
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
    // Each row may carry a different number of fields than the header so that a
    // row missing only the optional "status" column is still readable below.
    r.FieldsPerRecord = -1
    recs, err := r.ReadAll()
    if err != nil {
        return nil, err
    }
    var out []models.Reading
    // Validate header contains required columns for readings (allow both short and unit‑suffixed names)
    header := recs[0]
    // Build a map from header name to its index
    rawIdx := make(map[string]int)
    for idx, name := range header {
        rawIdx[name] = idx
    }
    // Logical column names we need and possible aliases present in the CSV
    colAliases := map[string][]string{
        "meter_id":      {"meter_id"},
        "timestamp":    {"timestamp"},
        "consumption":  {"consumption", "consumption_kwh"},
        "voltage":      {"voltage", "voltage_v"},
        "current":      {"current", "current_a"},
        "power_factor": {"power_factor", "power_factor"},
    }
    // Resolve each logical column to the actual header index
    colIndex := make(map[string]int)
    for logical, alts := range colAliases {
        found := false
        for _, alt := range alts {
            if idx, ok := rawIdx[alt]; ok {
                colIndex[logical] = idx
                found = true
                break
            }
        }
        if !found {
            return nil, fmt.Errorf("missing required column '%s' in readings CSV", logical)
        }
    }
    // The "status" column is optional: when it is absent from the header the
    // parsed readings keep an empty Status instead of failing the load.
    statusIdx, hasStatus := rawIdx["status"]
    // Process data rows
    for i, rec := range recs {
        // Skip header row
        if i == 0 {
            continue
        }
        // Ensure every required column is present in this row. Short rows are
        // tolerated as long as they still carry the required columns, which lets
        // the optional trailing "status" value be omitted.
        for logical, idx := range colIndex {
            if idx >= len(rec) {
                return nil, fmt.Errorf("missing required column '%s' in readings row %d", logical, i+1)
            }
        }
        // Helper to get column by logical name safely
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
        // Populate the optional status value; a short row has no status cell.
        status := ""
        if hasStatus && statusIdx < len(rec) {
            status = strings.TrimSpace(rec[statusIdx])
        }
        // Append the parsed reading
        out = append(out, models.Reading{MeterID: models.MeterID(get("meter_id")), Timestamp: ts, Consumption: kwh, Voltage: volt, Current: cur, PowerFactor: pf, Status: status})
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
    // Flexible header handling – accept both the original schema and the simplified one used in assets.
    header := recs[0]
    rawIdx := make(map[string]int)
    for idx, name := range header {
        rawIdx[name] = idx
    }
    // Logical fields we need and possible aliases.
    colAliases := map[string][]string{
        "id":          {"event_id", "meter_id"}, // use meter_id as fallback identifier
        "type":        {"event_type"},
        "timestamp":   {"event_timestamp", "timestamp", "start_time"},
        "description": {"description"},
    }
    colIndex := make(map[string]int)
    for logical, alts := range colAliases {
        found := false
        for _, alt := range alts {
            if idx, ok := rawIdx[alt]; ok {
                colIndex[logical] = idx
                found = true
                break
            }
        }
        if !found {
            return nil, fmt.Errorf("missing required column '%s' in events CSV", logical)
        }
    }
    // Process data rows (skip header)
    for i, rec := range recs {
        if i == 0 {
            continue
        }
        if len(rec) < len(header) {
            return nil, fmt.Errorf("missing columns in events row %d", i+1)
        }
        get := func(name string) string { return rec[colIndex[name]] }
        // Try formats: full with seconds, without seconds, and RFC3339
        layouts := []string{"2006-01-02 15:04:05", "2006-01-02 15:04", time.RFC3339}
        var ts time.Time
        var parseErr error
        for _, layout := range layouts {
            ts, parseErr = time.Parse(layout, get("timestamp"))
            if parseErr == nil {
                break
            }
        }
        if parseErr != nil {
            return nil, fmt.Errorf("invalid timestamp in events row %d: %w", i+1, parseErr)
        }
        // Use the same timestamp for both start and end (point‑in‑time event)
        out = append(out, models.Event{ID: get("id"), Type: models.EventType(get("type")), Start: ts, End: ts, Description: get("description")})
    }
    return out, nil
}
