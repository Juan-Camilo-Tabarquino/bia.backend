package memory

import (
    "reflect"
    "sort"
    "testing"
    "time"

    "github.com/neuralium/ai-energy/internal/domain/models"
)

func TestReadingRepo_AllMeterIDs(t *testing.T) {
    repo := NewReadingRepo()
    now := time.Now()
    readings := []models.Reading{
        {MeterID: "m1", Timestamp: now.Add(-2 * time.Hour)},
        {MeterID: "m1", Timestamp: now.Add(-1 * time.Hour)},
        {MeterID: "m2", Timestamp: now.Add(-30 * time.Minute)},
    }
    repo.AddMany(readings)

    ids := repo.AllMeterIDs()
    // order not important, sort for comparison
    sort.Strings(ids)
    expected := []string{"m1", "m2"}
    sort.Strings(expected)
    if !reflect.DeepEqual(ids, expected) {
        t.Fatalf("AllMeterIDs = %v, expected %v", ids, expected)
    }
}

func TestReadingRepo_ReadingsFor_TimeBounds(t *testing.T) {
    repo := NewReadingRepo()
    now := time.Now()
    r1 := models.Reading{MeterID: "m1", Timestamp: now.Add(-2 * time.Hour)}
    r2 := models.Reading{MeterID: "m1", Timestamp: now.Add(-1 * time.Hour)}
    r3 := models.Reading{MeterID: "m2", Timestamp: now.Add(-30 * time.Minute)}
    repo.AddMany([]models.Reading{r1, r2, r3})

    // No bounds: should return all three readings for both meters
    all := repo.ReadingsFor([]string{"m1", "m2"}, nil, nil)
    if len(all) != 3 {
        t.Fatalf("expected 3 readings, got %d", len(all))
    }

    // From bound: after first reading, should return r2 and r3
    from := now.Add(-90 * time.Minute) // 1.5h ago
    subsetFrom := repo.ReadingsFor([]string{"m1", "m2"}, &from, nil)
    if len(subsetFrom) != 2 {
        t.Fatalf("expected 2 readings with from bound, got %d", len(subsetFrom))
    }
    // Verify expected timestamps are present
    have := map[time.Time]bool{}
    for _, r := range subsetFrom {
        have[r.Timestamp] = true
    }
    if !(have[r2.Timestamp] && have[r3.Timestamp]) {
        t.Fatalf("ReadingsFor with from bound returned unexpected readings: %v", subsetFrom)
    }

    // To bound: up to 45 minutes ago, should exclude r3
    to := now.Add(-45 * time.Minute)
    subsetTo := repo.ReadingsFor([]string{"m1", "m2"}, nil, &to)
    if len(subsetTo) != 2 {
        t.Fatalf("expected 2 readings with to bound, got %d", len(subsetTo))
    }
    have = map[time.Time]bool{}
    for _, r := range subsetTo {
        have[r.Timestamp] = true
    }
    if !(have[r1.Timestamp] && have[r2.Timestamp]) {
        t.Fatalf("ReadingsFor with to bound returned unexpected readings: %v", subsetTo)
    }

    // Both bounds: should return only r2
    subsetBoth := repo.ReadingsFor([]string{"m1", "m2"}, &from, &to)
    if len(subsetBoth) != 1 || subsetBoth[0].Timestamp != r2.Timestamp {
        t.Fatalf("expected only r2 with both bounds, got %v", subsetBoth)
    }
}
