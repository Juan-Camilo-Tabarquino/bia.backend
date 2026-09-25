package csv

import (
    "encoding/csv"
    "os"
    "testing"
)

func TestLoadReadingsInvalidTimestamp(t *testing.T) {
    tmp, err := os.CreateTemp("", "readings_invalid_ts_*.csv")
    if err != nil { t.Fatalf("temp file: %v", err) }
    defer os.Remove(tmp.Name())
    w := csv.NewWriter(tmp)
    // header
    w.Write([]string{"meter_id","timestamp","consumption","voltage","current","power_factor","status"})
    // row with invalid timestamp
    w.Write([]string{"M-1","invalid_ts","10","230","5","0.95","OK"})
    w.Flush()
    tmp.Close()

    loader := NewLoader(tmp.Name(), "")
    _, err = loader.LoadReadings()
    if err == nil {
        t.Fatalf("expected error for invalid timestamp, got nil")
    }
}

func TestLoadReadingsMissingColumns(t *testing.T) {
    tmp, err := os.CreateTemp("", "readings_missing_cols_*.csv")
    if err != nil { t.Fatalf("temp file: %v", err) }
    defer os.Remove(tmp.Name())
    w := csv.NewWriter(tmp)
    // header with all columns
    w.Write([]string{"meter_id","timestamp","consumption","voltage","current","power_factor","status"})
    // row missing required columns (voltage and every column after it)
    w.Write([]string{"M-2","2023-01-01 12:00:00","10"})
    w.Flush()
    tmp.Close()

    loader := NewLoader(tmp.Name(), "")
    _, err = loader.LoadReadings()
    if err == nil {
        t.Fatalf("expected error for missing required column, got nil")
    }
}

func TestLoadReadingsMalformedNumeric(t *testing.T) {
    tmp, err := os.CreateTemp("", "readings_malformed_num_*.csv")
    if err != nil { t.Fatalf("temp file: %v", err) }
    defer os.Remove(tmp.Name())
    w := csv.NewWriter(tmp)
    w.Write([]string{"meter_id","timestamp","consumption","voltage","current","power_factor","status"})
    // consumption non-numeric
    w.Write([]string{"M-3","2023-01-01 12:00:00","not_a_number","230","5","0.95","OK"})
    w.Flush()
    tmp.Close()

    loader := NewLoader(tmp.Name(), "")
    _, err = loader.LoadReadings()
    if err == nil {
        t.Fatalf("expected error for malformed numeric, got nil")
    }
}

func TestLoadEventsInvalidTimestamp(t *testing.T) {
    tmp, err := os.CreateTemp("", "events_invalid_ts_*.csv")
    if err != nil { t.Fatalf("temp file: %v", err) }
    defer os.Remove(tmp.Name())
    w := csv.NewWriter(tmp)
    w.Write([]string{"event_id","event_type","start_time","end_time","description"})
    // invalid start timestamp
    w.Write([]string{"e1","PRODUCTION_LINE","invalid","2023-01-01T14:00:00Z","Desc"})
    w.Flush()
    tmp.Close()

    loader := NewLoader("", tmp.Name())
    _, err = loader.LoadEvents()
    if err == nil {
        t.Fatalf("expected error for invalid event timestamp, got nil")
    }
}

func TestLoadEventsMissingColumns(t *testing.T) {
    tmp, err := os.CreateTemp("", "events_missing_cols_*.csv")
    if err != nil { t.Fatalf("temp file: %v", err) }
    defer os.Remove(tmp.Name())
    w := csv.NewWriter(tmp)
    w.Write([]string{"event_id","event_type","start_time","end_time","description"})
    // missing description column
    w.Write([]string{"e2","MAINTENANCE","2023-01-01T12:00:00Z","2023-01-01T13:00:00Z"})
    w.Flush()
    tmp.Close()

    loader := NewLoader("", tmp.Name())
    _, err = loader.LoadEvents()
    if err == nil {
        t.Fatalf("expected error for missing columns in events, got nil")
    }
}

func TestLoadReadingsRealDataset(t *testing.T) {
    // `go test` runs with the package directory as the working directory, so the
    // repository asset is reached three levels up from internal/data/csv.
    loader := NewLoader("../../../assets/readings.csv", "")
    readings, err := loader.LoadReadings()
    if err != nil { t.Fatalf("unexpected error loading assets/readings.csv: %v", err) }
    if len(readings) != 4032 {
        t.Fatalf("expected 4032 readings, got %d", len(readings))
    }
    meters := make(map[string]bool)
    for _, r := range readings {
        meters[string(r.MeterID)] = true
        if r.Status != "OK" {
            t.Fatalf("expected status \"OK\" for meter %s at %s, got %q", r.MeterID, r.Timestamp, r.Status)
        }
    }
    if len(meters) != 12 {
        t.Fatalf("expected 12 distinct meter IDs, got %d", len(meters))
    }
    // Spot-check a known meter from the dataset.
    found := false
    for _, r := range readings {
        if r.MeterID == "M-101" {
            found = true
            if r.Status != "OK" {
                t.Fatalf("expected status \"OK\" for M-101, got %q", r.Status)
            }
            break
        }
    }
    if !found {
        t.Fatalf("expected to find meter M-101 in assets/readings.csv")
    }
}

func TestLoadReadingsValid(t *testing.T) {
    tmp, err := os.CreateTemp("", "readings_valid_*.csv")
    if err != nil { t.Fatalf("temp file: %v", err) }
    defer os.Remove(tmp.Name())
    w := csv.NewWriter(tmp)
    w.Write([]string{"meter_id","timestamp","consumption","voltage","current","power_factor","status"})
    w.Write([]string{"M-4","2023-01-01 12:00:00","10.5","230","5","0.95","OK"})
    w.Flush()
    tmp.Close()

    loader := NewLoader(tmp.Name(), "")
    readings, err := loader.LoadReadings()
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if len(readings) != 1 { t.Fatalf("expected 1 reading, got %d", len(readings)) }
    if string(readings[0].MeterID) != "M-4" { t.Fatalf("unexpected MeterID: %s", readings[0].MeterID) }
    if readings[0].Consumption != 10.5 { t.Fatalf("unexpected consumption: %f", readings[0].Consumption) }
    // check timestamp parsed correctly
    if readings[0].Timestamp.Format("2006-01-02 15:04:05") != "2023-01-01 12:00:00" {
        t.Fatalf("timestamp mismatch: %s", readings[0].Timestamp)
    }
}

func TestLoadEventsValid(t *testing.T) {
    tmp, err := os.CreateTemp("", "events_valid_*.csv")
    if err != nil { t.Fatalf("temp file: %v", err) }
    defer os.Remove(tmp.Name())
    w := csv.NewWriter(tmp)
    w.Write([]string{"event_id","event_type","start_time","end_time","description"})
    w.Write([]string{"e3","SHUTDOWN","2023-01-01T12:00:00Z","2023-01-01T13:00:00Z","Desc"})
    w.Flush()
    tmp.Close()

    loader := NewLoader("", tmp.Name())
    events, err := loader.LoadEvents()
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if len(events) != 1 { t.Fatalf("expected 1 event, got %d", len(events)) }
    if events[0].ID != "e3" { t.Fatalf("unexpected ID: %s", events[0].ID) }
    if events[0].Description != "Desc" { t.Fatalf("unexpected description: %s", events[0].Description) }
}
