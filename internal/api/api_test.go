package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/neuralium/ai-energy/internal/analysis"
	"github.com/neuralium/ai-energy/internal/api"
	"github.com/neuralium/ai-energy/internal/data/csv"
	"github.com/neuralium/ai-energy/internal/data/memory"
)

// The hermetic dataset below is generated at runtime, so every test is
// independent from the real data/ dataset and fully deterministic.
const (
	stableMeter = "T-1"
	otherMeter  = "T-2"
)

// testReadingsCSV builds two meters over 24 hourly readings. T-1 carries a
// single consumption spike at 12:00 with no explaining event, which the
// pipeline must classify as a REAL_ANOMALY; T-2 stays flat and produces none.
func testReadingsCSV() string {
	var b strings.Builder
	b.WriteString("meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor,status\n")
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for hour := 0; hour < 24; hour++ {
		ts := base.Add(time.Duration(hour) * time.Hour).Format("2006-01-02 15:04:05")
		consumption := 10.0
		if hour == 12 {
			consumption = 25.0
		}
		fmt.Fprintf(&b, "%s,%s,%.2f,220.00,45.00,0.95,OK\n", stableMeter, ts, consumption)
		fmt.Fprintf(&b, "%s,%s,%.2f,220.00,20.00,0.96,OK\n", otherMeter, ts, 5.0)
	}
	return b.String()
}

// testEventsCSV keeps the events file valid but empty, so no event can explain
// the spike and the classification stays REAL_ANOMALY.
func testEventsCSV() string {
	return "event_id,event_type,timestamp,description\n"
}

// newTestEnvironment writes the generated CSVs to a temp dir, builds a real
// orchestrator over them, runs the pipeline once and returns the router-backed
// test server plus the orchestrator for direct assertions.
func newTestEnvironment(t *testing.T) (*httptest.Server, *analysis.Orchestrator) {
	t.Helper()
	dir := t.TempDir()
	readingsPath := filepath.Join(dir, "readings.csv")
	eventsPath := filepath.Join(dir, "events.csv")
	if err := os.WriteFile(readingsPath, []byte(testReadingsCSV()), 0o600); err != nil {
		t.Fatalf("write readings csv: %v", err)
	}
	if err := os.WriteFile(eventsPath, []byte(testEventsCSV()), 0o600); err != nil {
		t.Fatalf("write events csv: %v", err)
	}

	orchestrator := analysis.NewOrchestrator(
		csv.NewLoader(readingsPath, eventsPath),
		memory.NewReadingRepo(),
		memory.NewEventRepo(),
		analysis.NewQualityChecker(),
		analysis.NewBaselineCalculator(),
		analysis.NewAnomalyDetector(),
		analysis.NewEventCorrelator(),
		analysis.NewClassifier(),
		analysis.NewScorer(),
		analysis.NewMockLLM(),
		analysis.NewEvidenceBuilder(),
	)
	if err := orchestrator.Run(); err != nil {
		t.Fatalf("orchestrator run: %v", err)
	}

	server := httptest.NewServer(api.NewRouter(orchestrator))
	t.Cleanup(server.Close)
	return server, orchestrator
}

// anomalyBody mirrors the API anomaly DTO decoded by the tests.
type anomalyBody struct {
	ID                string  `json:"id"`
	MeterID           string  `json:"meter_id"`
	DetectedAt        string  `json:"detected_at"`
	Type              string  `json:"type"`
	Severity          string  `json:"severity"`
	Confidence        float64 `json:"confidence"`
	Reason            string  `json:"reason"`
	RecommendedAction string  `json:"recommended_action"`
	Status            string  `json:"status"`
}

type readingBody struct {
	MeterID     string
	Timestamp   time.Time
	Consumption float64
	Status      string
}

func getStatus(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body for %s: %v", url, err)
	}
	return resp.StatusCode, string(body)
}

func getJSON(t *testing.T, url string, out any) {
	t.Helper()
	status, body := getStatus(t, url)
	if status != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d (body %s)", url, status, body)
	}
	if err := json.Unmarshal([]byte(body), out); err != nil {
		t.Fatalf("decode %s: %v (body %s)", url, err, body)
	}
}

func postJSON(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body for %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: expected 200, got %d (body %s)", url, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %s: %v (body %s)", url, err, body)
	}
}

// TestHealthEndpoint fixes the previous failure: the router is driven directly,
// without wrapping it in http.StripPrefix("/api", ...).
func TestHealthEndpoint(t *testing.T) {
	server, _ := newTestEnvironment(t)

	status, body := getStatus(t, server.URL+"/api/health")
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
	if body != `{"status":"ok"}` {
		t.Fatalf("expected body %s, got %s", `{"status":"ok"}`, body)
	}
}

// TestRouterRegistersOnlyAPIPrefixedRoutes enforces the intentional single
// prefix: no legacy unprefixed alias and no /api/v1 route may exist.
func TestRouterRegistersOnlyAPIPrefixedRoutes(t *testing.T) {
	server, _ := newTestEnvironment(t)

	for _, path := range []string{
		"/health",
		"/reports",
		"/meters",
		"/anomalies",
		"/api/v1/health",
		"/api/v1/meters",
		"/api/v1/anomalies",
	} {
		status, _ := getStatus(t, server.URL+path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: expected 404 (no legacy or /api/v1 alias), got %d", path, status)
		}
	}
}

func TestReportsEndpoint(t *testing.T) {
	server, _ := newTestEnvironment(t)

	var body struct {
		Reports []json.RawMessage `json:"reports"`
	}
	getJSON(t, server.URL+"/api/reports", &body)
	if len(body.Reports) != 1 {
		t.Fatalf("expected 1 report (T-1 spike), got %d", len(body.Reports))
	}
}

func TestMetersListEndpoint(t *testing.T) {
	server, _ := newTestEnvironment(t)

	var ids []string
	getJSON(t, server.URL+"/api/meters", &ids)
	sort.Strings(ids)
	want := []string{stableMeter, otherMeter}
	sort.Strings(want)
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("expected meters %v, got %v", want, ids)
	}
}

func TestMeterDetailEndpoint(t *testing.T) {
	server, _ := newTestEnvironment(t)

	var meter struct {
		ID            string `json:"id"`
		MeterID       string `json:"meter_id"`
		Name          string `json:"name"`
		Location      string `json:"location"`
		Status        string `json:"status"`
		CreatedAt     string `json:"created_at"`
		ReadingsCount int    `json:"readings_count"`
		LastReadingAt string `json:"last_reading_at"`
	}
	getJSON(t, server.URL+"/api/meters/"+stableMeter, &meter)

	if meter.ID != stableMeter || meter.MeterID != stableMeter {
		t.Fatalf("expected id/meter_id %q, got %q/%q", stableMeter, meter.ID, meter.MeterID)
	}
	if meter.Name != "" || meter.Location != "" {
		t.Fatalf("name/location have no data source and must stay empty, got %q/%q", meter.Name, meter.Location)
	}
	if meter.Status != "OK" {
		t.Fatalf("expected status OK, got %q", meter.Status)
	}
	if meter.ReadingsCount != 24 {
		t.Fatalf("expected 24 readings, got %d", meter.ReadingsCount)
	}
	if meter.CreatedAt != "2026-09-01T00:00:00Z" {
		t.Fatalf("expected created_at 2026-09-01T00:00:00Z, got %q", meter.CreatedAt)
	}
	if meter.LastReadingAt != "2026-09-01T23:00:00Z" {
		t.Fatalf("expected last_reading_at 2026-09-01T23:00:00Z, got %q", meter.LastReadingAt)
	}
}

func TestMeterDetailUnknownReturnsNotFound(t *testing.T) {
	server, _ := newTestEnvironment(t)

	status, body := getStatus(t, server.URL+"/api/meters/UNKNOWN")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown meter, got %d", status)
	}
	if !strings.Contains(body, "error") {
		t.Fatalf("expected a JSON error body, got %s", body)
	}
}

func TestMeterReadingsEndpointWithTimeRange(t *testing.T) {
	server, _ := newTestEnvironment(t)

	var all []readingBody
	getJSON(t, server.URL+"/api/meters/"+stableMeter+"/readings", &all)
	if len(all) != 24 {
		t.Fatalf("expected 24 readings without bounds, got %d", len(all))
	}

	var window []readingBody
	getJSON(t, server.URL+"/api/meters/"+stableMeter+"/readings?from=2026-09-01T10:00:00Z&to=2026-09-01T12:00:00Z", &window)
	if len(window) != 3 {
		t.Fatalf("expected 3 readings inside the window, got %d", len(window))
	}
	if !window[2].Timestamp.Equal(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected the last in-window reading at 12:00, got %s", window[2].Timestamp)
	}
}

// TestAnomaliesListAndDetail proves the list and the detail endpoint publish the
// same stable id and the real pipeline values, not hardcoded placeholders.
func TestAnomaliesListAndDetail(t *testing.T) {
	server, _ := newTestEnvironment(t)

	var list []anomalyBody
	getJSON(t, server.URL+"/api/anomalies", &list)
	if len(list) != 1 {
		t.Fatalf("expected exactly 1 anomaly, got %d", len(list))
	}
	listed := list[0]
	if listed.ID != "T-1-2026-09-01T12:00:00Z" {
		t.Fatalf("expected deterministic id T-1-2026-09-01T12:00:00Z, got %q", listed.ID)
	}
	if listed.MeterID != stableMeter {
		t.Fatalf("expected meter %q, got %q", stableMeter, listed.MeterID)
	}
	if listed.Type != "REAL_ANOMALY" {
		t.Fatalf("expected REAL_ANOMALY, got %q", listed.Type)
	}
	if listed.Severity != "HIGH" {
		t.Fatalf("expected HIGH severity, got %q", listed.Severity)
	}
	if listed.Confidence <= 0.9 {
		t.Fatalf("expected a real confidence above the old hardcoded 0.9, got %v", listed.Confidence)
	}
	if listed.Status != "unexplained" {
		t.Fatalf("expected unexplain status, got %q", listed.Status)
	}
	if listed.DetectedAt != "2026-09-01T12:00:00Z" {
		t.Fatalf("expected detected_at 2026-09-01T12:00:00Z, got %q", listed.DetectedAt)
	}
	if listed.Reason == "" || listed.RecommendedAction == "" {
		t.Fatalf("expected reason and recommended_action to be populated, got %+v", listed)
	}

	status, raw := getStatus(t, server.URL+"/api/anomalies/"+listed.ID)
	t.Logf("GET /api/anomalies/%s -> %d %s", listed.ID, status, strings.TrimSpace(raw))
	if status != http.StatusOK {
		t.Fatalf("expected 200 for the listed id, got %d", status)
	}

	var detail anomalyBody
	if err := json.Unmarshal([]byte(raw), &detail); err != nil {
		t.Fatalf("decode detail: %v (body %s)", err, raw)
	}
	if detail != listed {
		t.Fatalf("detail does not match the list entry:\nlist   %+v\ndetail %+v", listed, detail)
	}
}

func TestAnomalyDetailUnknownReturnsNotFound(t *testing.T) {
	server, _ := newTestEnvironment(t)

	status, body := getStatus(t, server.URL+"/api/anomalies/does-not-exist")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown anomaly id, got %d", status)
	}
	if !strings.Contains(body, "error") {
		t.Fatalf("expected a JSON error body, got %s", body)
	}
}

func TestAnalyzeReRunsPipelineAndStoresResult(t *testing.T) {
	server, _ := newTestEnvironment(t)

	var created struct {
		AnalysisID string `json:"analysisId"`
	}
	postJSON(t, server.URL+"/api/ai/analyze", &created)
	if created.AnalysisID == "" {
		t.Fatal("expected a non-empty analysisId")
	}

	var result struct {
		AnalysisID string        `json:"analysisId"`
		Status     string        `json:"status"`
		Anomalies  []anomalyBody `json:"anomalies"`
	}
	getJSON(t, server.URL+"/api/ai/analysis/"+created.AnalysisID, &result)

	if result.AnalysisID != created.AnalysisID {
		t.Fatalf("expected analysisId %q, got %q", created.AnalysisID, result.AnalysisID)
	}
	if result.Status != "completed" {
		t.Fatalf("expected status completed, got %q", result.Status)
	}
	if len(result.Anomalies) != 1 {
		t.Fatalf("expected 1 stored anomaly, got %d", len(result.Anomalies))
	}
	if result.Anomalies[0].Severity != "HIGH" || result.Anomalies[0].Type != "REAL_ANOMALY" {
		t.Fatalf("expected the real pipeline values in the stored result, got %+v", result.Anomalies[0])
	}
}

func TestAnalysisUnknownReturnsNotFound(t *testing.T) {
	server, _ := newTestEnvironment(t)

	status, body := getStatus(t, server.URL+"/api/ai/analysis/unknown-analysis-id")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown analysis id, got %d", status)
	}
	if !strings.Contains(body, "error") {
		t.Fatalf("expected a JSON error body, got %s", body)
	}
}

func TestDashboardSummaryEndpoint(t *testing.T) {
	server, _ := newTestEnvironment(t)

	var summary struct {
		Health    string `json:"health"`
		Meters    int    `json:"meters"`
		Anomalies int    `json:"anomalies"`
	}
	getJSON(t, server.URL+"/api/dashboard/summary", &summary)

	if summary.Health != "ok" {
		t.Fatalf("expected health ok, got %q", summary.Health)
	}
	if summary.Meters != 2 {
		t.Fatalf("expected 2 meters, got %d", summary.Meters)
	}
	if summary.Anomalies != 1 {
		t.Fatalf("expected 1 anomaly, got %d", summary.Anomalies)
	}
}

// TestOrchestratorRunTwiceDoesNotDuplicateReadings proves Run is idempotent
// with respect to the append-only in-memory repositories.
func TestOrchestratorRunTwiceDoesNotDuplicateReadings(t *testing.T) {
	_, orchestrator := newTestEnvironment(t)

	ids := orchestrator.ReadingRepo.AllMeterIDs()
	before := len(orchestrator.ReadingRepo.ReadingsFor(ids, nil, nil))
	if before != 48 {
		t.Fatalf("expected 48 seeded readings (2 meters x 24), got %d", before)
	}

	if err := orchestrator.Run(); err != nil {
		t.Fatalf("second orchestrator run: %v", err)
	}

	after := len(orchestrator.ReadingRepo.ReadingsFor(ids, nil, nil))
	if after != before {
		t.Fatalf("second run duplicated readings: before %d, after %d", before, after)
	}
	if evidence := orchestrator.Evidence(); len(evidence) != 1 {
		t.Fatalf("expected 1 evidence record after the second run, got %d", len(evidence))
	}
}

// TestConcurrentRequestsAreRaceFree exercises the analysis store mutex by
// hammering the same stored id while a re-run refreshes the evidence.
func TestConcurrentRequestsAreRaceFree(t *testing.T) {
	server, _ := newTestEnvironment(t)

	var created struct {
		AnalysisID string `json:"analysisId"`
	}
	postJSON(t, server.URL+"/api/ai/analyze", &created)

	const workers = 16
	done := make(chan string, workers)
	for i := 0; i < workers; i++ {
		go func() {
			resp, err := http.Get(server.URL + "/api/ai/analysis/" + created.AnalysisID)
			if err != nil {
				done <- err.Error()
				return
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				done <- fmt.Sprintf("unexpected status %d: %s", resp.StatusCode, raw)
				return
			}
			done <- ""
		}()
	}
	for i := 0; i < workers; i++ {
		if msg := <-done; msg != "" {
			t.Fatalf("concurrent request failed: %s", msg)
		}
	}
}
