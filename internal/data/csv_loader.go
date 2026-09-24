package data

import (
    "context"
    "encoding/csv"
    "io"
    "os"
    "strconv"
    "time"

    "github.com/neuralium/ai-energy/internal/domain/models"
)

// CSVLoader implements the DataLoader interface.
// It expects the CSV to have header row: meter_id,timestamp,consumption,voltage,current,power_factor
// Timestamp must be RFC3339.

type CSVLoader struct {
    path string
}

func NewCSVLoader(path string) *CSVLoader {
    return &CSVLoader{path: path}
}

func (l *CSVLoader) Load(ctx context.Context) ([]models.Reading, error) {
    f, err := os.Open(l.path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    r := csv.NewReader(f)
    // skip header
    if _, err = r.Read(); err != nil {
        return nil, err
    }

    var res []models.Reading
    for {
        rec, err := r.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, err
        }
        if len(rec) < 6 {
            continue // skip malformatted
        }
        ts, _ := time.Parse(time.RFC3339, rec[1])
        cons, _ := strconv.ParseFloat(rec[2], 64)
        volt, _ := strconv.ParseFloat(rec[3], 64)
        curr, _ := strconv.ParseFloat(rec[4], 64)
        pf, _ := strconv.ParseFloat(rec[5], 64)
        res = append(res, models.Reading{MeterID: models.MeterID(rec[0]), Timestamp: ts, Consumption: cons, Voltage: volt, Current: curr, PowerFactor: pf})
    }
    return res, nil
}
