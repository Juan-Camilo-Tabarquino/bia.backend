package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neuralium/ai-energy/internal/analysis"
	"github.com/neuralium/ai-energy/internal/api"
	"github.com/neuralium/ai-energy/internal/api/handlers"
	"github.com/neuralium/ai-energy/internal/data/csv"
	"github.com/neuralium/ai-energy/internal/data/memory"
	"github.com/neuralium/ai-energy/internal/domain/models"
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

// The hermetic fixture below pins the minimum-readings rule end to end. It is
// deliberately separate from testReadingsCSV: TestDashboardSummaryEndpoint
// asserts that shared fixture has exactly two meters, so it must not grow a
// third one.
const (
	// singleReadingMeter has exactly ONE electrically inconsistent reading.
	singleReadingMeter = "U-1"
	// gappedNormalMeter stays inside every sane range and produces no gap.
	gappedNormalMeter = "N-1"
)

// insufficientReadingsReason is the exact user-visible reason the summary must
// publish. It is spelled out here rather than referencing the analysis constant
// on purpose: this end-to-end test pins the literal HTTP contract instead of
// mirroring the implementation.
const insufficientReadingsReason = "no hay suficiente información para validar: se requieren al menos 2 lecturas"

// singleReadingReadingsCSV builds one meter with exactly ONE electrically
// inconsistent reading (voltage above the sane [209, 231] band, which is what
// electricalInconsistency detects) plus one normal meter with several consistent
// readings. Before the minimum-readings guard, the single reading took the
// data-quality branch, carried a NaN baseline into the anomaly DTO and made
// json.Marshal reject the payload, so /api/anomalies answered 200 with an EMPTY
// body.
func singleReadingReadingsCSV() string {
	var b strings.Builder
	b.WriteString("meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor,status\n")
	base := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	fmt.Fprintf(&b, "%s,%s,12.00,240.00,45.00,0.95,OK\n",
		singleReadingMeter, base.Format("2006-01-02 15:04:05"))
	for hour := 1; hour <= 4; hour++ {
		ts := base.Add(time.Duration(hour) * time.Hour).Format("2006-01-02 15:04:05")
		fmt.Fprintf(&b, "%s,%s,10.00,220.00,45.00,0.95,OK\n", gappedNormalMeter, ts)
	}
	return b.String()
}

// newEnvironmentWithReadings builds a router-backed test server over a caller
// supplied readings CSV and the shared empty events file, then runs the real
// pipeline once. It mirrors newTestEnvironmentWithLLM so the hermetic fixture
// above exercises exactly the same production wiring.
func newEnvironmentWithReadings(t *testing.T, readingsCSV string) (*httptest.Server, *analysis.Orchestrator) {
	t.Helper()
	dir := t.TempDir()
	readingsPath := filepath.Join(dir, "readings.csv")
	eventsPath := filepath.Join(dir, "events.csv")
	if err := os.WriteFile(readingsPath, []byte(readingsCSV), 0o600); err != nil {
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

// failingLLM is a deterministic LLMClient whose explanation call always fails,
// mirroring a provider that is not configured or is unreachable. The
// orchestrator leaves LLMText empty in that case.
type failingLLM struct{}

func (failingLLM) GenerateExplanation(models.Evidence) (string, error) {
	return "", errors.New("llm provider unavailable")
}

// newTestEnvironment writes the generated CSVs to a temp dir, builds a real
// orchestrator over them with the deterministic mock LLM, runs the pipeline once
// and returns the router-backed test server plus the orchestrator.
func newTestEnvironment(t *testing.T) (*httptest.Server, *analysis.Orchestrator) {
	t.Helper()
	return newTestEnvironmentWithLLM(t, analysis.NewMockLLM())
}

// newTestEnvironmentWithLLM is newTestEnvironment with an injectable LLM client,
// so tests can exercise the serialization contract for both a populated and an
// empty LLMText without touching the deterministic pipeline.
func newTestEnvironmentWithLLM(t *testing.T, llm analysis.LLMClient) (*httptest.Server, *analysis.Orchestrator) {
	t.Helper()
	redirectAnalysisLog(t)
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
		llm,
		analysis.NewEvidenceBuilder(),
	)
	if err := orchestrator.Run(); err != nil {
		t.Fatalf("orchestrator run: %v", err)
	}

	server := httptest.NewServer(api.NewRouter(orchestrator))
	t.Cleanup(server.Close)
	return server, orchestrator
}

// newDetectOnlyEnvironment builds a router-backed server over the hermetic
// fixture and runs only the deterministic stage, so the injected LLM is never
// called before the test does. It lets a gated LLM observe an asynchronous run
// parked in the middle of the pipeline.
func newDetectOnlyEnvironment(t *testing.T, llm analysis.LLMClient) *httptest.Server {
	t.Helper()
	redirectAnalysisLog(t)
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
		llm,
		analysis.NewEvidenceBuilder(),
	)
	if err := orchestrator.Detect(); err != nil {
		t.Fatalf("orchestrator detect: %v", err)
	}

	server := httptest.NewServer(api.NewRouter(orchestrator))
	t.Cleanup(server.Close)
	return server
}

// datasetReadingsPath and datasetEventsPath point at the real repository dataset,
// so the ordering and statistical-evidence assertions run over the same data the
// product ships rather than a synthetic fixture.
func datasetReadingsPath() string { return filepath.Join("..", "..", "data", "readings.csv") }
func datasetEventsPath() string   { return filepath.Join("..", "..", "data", "events.csv") }

// newDatasetEnvironment builds an orchestrator over the real dataset and
// returns the router-backed test server plus the orchestrator.
func newDatasetEnvironment(t *testing.T) (*httptest.Server, *analysis.Orchestrator) {
	t.Helper()
	orchestrator := analysis.NewOrchestrator(
		csv.NewLoader(datasetReadingsPath(), datasetEventsPath()),
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

// anomalyBody mirrors the API anomaly DTO decoded by the tests. The statistical
// fields (priority, baseline, the four per-signal changes, the correlated
// events and the data-quality flag) are decoded alongside the original ones, so
// the tests can prove the published evidence, not only its presence as a key.
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
	Priority          int     `json:"priority"`
	Baseline          struct {
		Mean            float64 `json:"mean"`
		StdDev          float64 `json:"stddev"`
		Count           int     `json:"count"`
		VoltageMean     float64 `json:"voltage_mean"`
		CurrentMean     float64 `json:"current_mean"`
		PowerFactorMean float64 `json:"power_factor_mean"`
	} `json:"baseline"`
	ConsumptionChangePct float64 `json:"consumption_change_pct"`
	VoltageChangePct     float64 `json:"voltage_change_pct"`
	CurrentChangePct     float64 `json:"current_change_pct"`
	PowerFactorChangePct float64 `json:"power_factor_change_pct"`
	CorrelatedEvents     []struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		Start       string `json:"start"`
		End         string `json:"end"`
		Description string `json:"description"`
	} `json:"correlated_events"`
	DataQuality struct {
		Flagged bool   `json:"flagged"`
		Reason  string `json:"reason"`
	} `json:"data_quality"`
	LLMAnalysis string `json:"llm_analysis"`
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

// redirectAnalysisLog points the write-only analysis audit log at a per-test
// temporary file, so the suite never writes into the repository's data
// directory.
func redirectAnalysisLog(t *testing.T) {
	t.Helper()
	handlers.SetAnalysisLogPath(filepath.Join(t.TempDir(), "analyses.json"))
}

// analysisCreatedBody is the 202 body of POST /api/ai/analyze.
type analysisCreatedBody struct {
	AnalysisID string `json:"analysisId"`
	MeterID    string `json:"meter_id"`
	Status     string `json:"status"`
}

// analysisBody mirrors the frozen GET /api/ai/analysis/{id} contract.
type analysisBody struct {
	AnalysisID string `json:"analysisId"`
	MeterID    string `json:"meter_id"`
	Status     string `json:"status"`
	Stage      string `json:"stage"`
	Progress   struct {
		Done  int `json:"done"`
		Total int `json:"total"`
	} `json:"progress"`
	StartedAt  string        `json:"started_at"`
	FinishedAt *string       `json:"finished_at"`
	Anomalies  []anomalyBody `json:"anomalies"`
	Platform   *struct {
		TotalAnomalies int `json:"total_anomalies"`
		HighPriority   int `json:"high_priority"`
	} `json:"platform"`
	Error *string `json:"error"`
}

// postAnalyze starts an asynchronous analysis run for meterID and decodes the
// 202 body. Any other status fails the test.
func postAnalyze(t *testing.T, url, meterID string) analysisCreatedBody {
	t.Helper()
	body := strings.NewReader(fmt.Sprintf(`{"meter_id":%q}`, meterID))
	resp, err := http.Post(url, "application/json", body)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body for %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST %s: expected 202, got %d (body %s)", url, resp.StatusCode, raw)
	}
	var created analysisCreatedBody
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode %s: %v (body %s)", url, err, raw)
	}
	return created
}

// waitForAnalysis polls GET /api/ai/analysis/{id} until the run reaches a
// terminal status and returns the final body. The plain mock LLM finishes
// instantly; the timeout only guards against a regression that would leave the
// record non-terminal.
func waitForAnalysis(t *testing.T, url, id string) analysisBody {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var body analysisBody
		getJSON(t, url, &body)
		switch body.Status {
		case "completed":
			return body
		case "failed":
			t.Fatalf("analysis %s failed: %v (body %+v)", id, body.Error, body)
		}
		if time.Now().After(deadline) {
			t.Fatalf("analysis %s did not reach a terminal status in time (last status %q, stage %q)", id, body.Status, body.Stage)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// gatedLLM parks its GenerateExplanation call until the test releases it, so a
// test can observe an asynchronous analysis run in a real intermediate state.
type gatedLLM struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newGatedLLM() *gatedLLM {
	return &gatedLLM{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *gatedLLM) GenerateExplanation(models.Evidence) (string, error) {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return "narrativa de prueba", nil
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
	const want = `{"error":"medidor UNKNOWN no encontrado"}`
	if got := strings.TrimSpace(body); got != want {
		t.Fatalf("expected error body %s, got %s", want, got)
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
	// The DTO now carries a slice (correlated_events), so it is no longer
	// comparable with ==; compare the two bodies by value instead.
	if !reflect.DeepEqual(detail, listed) {
		t.Fatalf("detail does not match the list entry:\nlist   %+v\ndetail %+v", listed, detail)
	}
}

// TestAnomaliesListOrderAndStatisticalEvidence runs the list and detail
// endpoints over the real dataset and proves two contracts T26/T27
// require: the list is ordered by the deterministic priority (1 = most urgent)
// with a stable tie-break, and every element publishes the full statistical
// evidence (priority, baseline, the four per-signal changes, the correlated
// events and the data-quality flag). The detail endpoint must deep-equal its
// list element field for field.
func TestAnomaliesListOrderAndStatisticalEvidence(t *testing.T) {
	server, _ := newDatasetEnvironment(t)

	status, raw := getStatus(t, server.URL+"/api/anomalies")
	if status != http.StatusOK {
		t.Fatalf("GET /api/anomalies: expected 200, got %d (body %s)", status, raw)
	}
	t.Logf("GET /api/anomalies (repo dataset) -> %d %s", status, strings.TrimSpace(raw))

	var list []anomalyBody
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("decode list: %v (body %s)", err, raw)
	}

	// The deterministic priority order: M-109 (1) before M-112 (2) before
	// M-104 (3) before M-106 (4).
	wantOrder := []struct {
		meter    string
		priority int
	}{
		{meter: "M-109", priority: 1},
		{meter: "M-112", priority: 2},
		{meter: "M-104", priority: 3},
		{meter: "M-106", priority: 4},
	}
	if len(list) != len(wantOrder) {
		t.Fatalf("expected %d anomalies, got %d (body %s)", len(wantOrder), len(list), raw)
	}
	for i, want := range wantOrder {
		got := list[i]
		if got.MeterID != want.meter {
			t.Errorf("list[%d]: expected meter %s, got %s", i, want.meter, got.MeterID)
		}
		if got.Priority != want.priority {
			t.Errorf("list[%d] (%s): expected priority %d, got %d", i, want.meter, want.priority, got.Priority)
		}
		if i > 0 && list[i-1].Priority > got.Priority {
			t.Errorf("list is not ordered by priority ascending: %d before %d", list[i-1].Priority, got.Priority)
		}
	}

	// Every element must expose the statistical-evidence keys, and
	// correlated_events must always be a JSON array (never null).
	var rawList []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rawList); err != nil {
		t.Fatalf("decode raw list: %v (body %s)", err, raw)
	}
	requiredTop := []string{
		"priority", "baseline",
		"consumption_change_pct", "voltage_change_pct", "current_change_pct", "power_factor_change_pct",
		"correlated_events", "data_quality",
	}
	requiredBaseline := []string{"mean", "stddev", "count", "voltage_mean", "current_mean", "power_factor_mean"}
	for _, elem := range rawList {
		var meter string
		if err := json.Unmarshal(elem["meter_id"], &meter); err != nil {
			t.Fatalf("decode meter_id from raw element: %v", err)
		}
		for _, key := range requiredTop {
			if _, ok := elem[key]; !ok {
				t.Errorf("meter %s: missing JSON field %q", meter, key)
			}
		}
		if string(elem["correlated_events"]) == "null" {
			t.Errorf("meter %s: correlated_events must be an array, got null", meter)
		}
		var baseline map[string]json.RawMessage
		if err := json.Unmarshal(elem["baseline"], &baseline); err != nil {
			t.Errorf("meter %s: decode baseline: %v", meter, err)
			continue
		}
		for _, key := range requiredBaseline {
			if _, ok := baseline[key]; !ok {
				t.Errorf("meter %s: missing baseline field %q", meter, key)
			}
		}
	}

	byMeter := make(map[string]anomalyBody, len(list))
	for _, a := range list {
		byMeter[a.MeterID] = a
	}
	for meter, a := range byMeter {
		if a.Baseline.Mean <= 0 || a.Baseline.Count <= 0 {
			t.Errorf("meter %s: baseline must carry the real statistics, got %+v", meter, a.Baseline)
		}
		if a.Baseline.VoltageMean <= 0 || a.Baseline.CurrentMean <= 0 || a.Baseline.PowerFactorMean <= 0 {
			t.Errorf("meter %s: electrical baseline means must be populated, got %+v", meter, a.Baseline)
		}
	}
	if got := byMeter["M-109"].ConsumptionChangePct; got <= 0 {
		t.Errorf("M-109 is a consumption spike: expected consumption_change_pct > 0, got %v", got)
	}

	// Correlated events: a real event explains M-104 and M-106, while M-109 is a
	// real anomaly with no explaining event and must publish an empty array.
	for _, meter := range []string{"M-104", "M-106"} {
		if len(byMeter[meter].CorrelatedEvents) == 0 {
			t.Errorf("meter %s is explained by an operational event: expected non-empty correlated_events", meter)
		}
	}
	if events := byMeter["M-109"].CorrelatedEvents; len(events) != 0 {
		t.Errorf("M-109 has no explaining event: expected empty correlated_events, got %+v", events)
	}

	// data_quality.flagged is true only for the DATA_QUALITY anomaly (M-112).
	for meter, a := range byMeter {
		wantFlagged := meter == "M-112"
		if a.DataQuality.Flagged != wantFlagged {
			t.Errorf("meter %s: data_quality.flagged = %v, want %v", meter, a.DataQuality.Flagged, wantFlagged)
		}
		if wantFlagged && a.DataQuality.Reason == "" {
			t.Errorf("meter %s: a flagged data-quality anomaly must carry a reason", meter)
		}
	}

	// The detail endpoint must publish exactly the same body as its list
	// element, field for field.
	for _, listed := range list {
		detailStatus, detailRaw := getStatus(t, server.URL+"/api/anomalies/"+listed.ID)
		if detailStatus != http.StatusOK {
			t.Fatalf("GET /api/anomalies/%s: expected 200, got %d (body %s)", listed.ID, detailStatus, detailRaw)
		}
		var detail anomalyBody
		if err := json.Unmarshal([]byte(detailRaw), &detail); err != nil {
			t.Fatalf("decode detail %s: %v (body %s)", listed.ID, err, detailRaw)
		}
		if !reflect.DeepEqual(detail, listed) {
			t.Errorf("detail for %s does not match its list element:\nlist   %+v\ndetail %+v", listed.ID, listed, detail)
		}
	}
}

// TestAnomaliesExposeLLMAnalysisAlongsideDeterministicFields proves the HTTP
// contract exposes the LLM narrative as an ADDITIONAL field: llm_analysis
// carries models.Evidence.LLMText while reason keeps carrying the deterministic
// explanation, so the two never collapse into one and the deterministic values
// stay authoritative.
func TestAnomaliesExposeLLMAnalysisAlongsideDeterministicFields(t *testing.T) {
	server, orchestrator := newTestEnvironment(t)

	evidence := orchestrator.Evidence()
	if len(evidence) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(evidence))
	}
	wantLLM := evidence[0].LLMText
	if wantLLM == "" {
		t.Fatal("mock LLM must populate LLMText for this test to be meaningful")
	}

	status, raw := getStatus(t, server.URL+"/api/anomalies")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d (body %s)", status, raw)
	}
	t.Logf("GET /api/anomalies (LLM configured) -> %d %s", status, strings.TrimSpace(raw))
	if !strings.Contains(raw, `"llm_analysis"`) {
		t.Fatalf("expected the serialized anomaly to contain llm_analysis, got %s", raw)
	}

	var list []anomalyBody
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("decode list: %v (body %s)", err, raw)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(list))
	}
	got := list[0]

	if got.LLMAnalysis != wantLLM {
		t.Fatalf("llm_analysis must carry models.Evidence.LLMText: got %q want %q", got.LLMAnalysis, wantLLM)
	}
	if got.Reason != evidence[0].Explanation {
		t.Fatalf("reason must keep the deterministic explanation: got %q want %q", got.Reason, evidence[0].Explanation)
	}
	if got.RecommendedAction != evidence[0].Recommendation {
		t.Fatalf("recommended_action must keep the deterministic recommendation: got %q want %q", got.RecommendedAction, evidence[0].Recommendation)
	}
	if got.LLMAnalysis == got.Reason {
		t.Fatalf("llm_analysis and reason must be different fields with different content, both were %q", got.Reason)
	}
}

// TestAnomaliesOmitLLMAnalysisWhenLLMUnavailable proves the omitempty contract
// end to end: when the provider call fails, LLMText stays empty and the key is
// absent from the JSON, while the deterministic reason remains present.
func TestAnomaliesOmitLLMAnalysisWhenLLMUnavailable(t *testing.T) {
	server, orchestrator := newTestEnvironmentWithLLM(t, failingLLM{})

	for _, ev := range orchestrator.Evidence() {
		if ev.LLMText != "" {
			t.Fatalf("expected empty LLMText when the LLM call fails, got %q", ev.LLMText)
		}
		if ev.Explanation == "" {
			t.Fatalf("deterministic explanation must stay populated, got %+v", ev)
		}
	}

	status, raw := getStatus(t, server.URL+"/api/anomalies")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d (body %s)", status, raw)
	}
	t.Logf("GET /api/anomalies (LLM unavailable) -> %d %s", status, strings.TrimSpace(raw))
	if strings.Contains(raw, "llm_analysis") {
		t.Fatalf("llm_analysis must be absent when LLMText is empty, got %s", raw)
	}
	if !strings.Contains(raw, `"reason"`) {
		t.Fatalf("deterministic reason must remain present, got %s", raw)
	}
}

func TestAnomalyDetailUnknownReturnsNotFound(t *testing.T) {
	server, _ := newTestEnvironment(t)

	status, body := getStatus(t, server.URL+"/api/anomalies/does-not-exist")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown anomaly id, got %d", status)
	}
	const want = `{"error":"anomalía does-not-exist no encontrada"}`
	if got := strings.TrimSpace(body); got != want {
		t.Fatalf("expected error body %s, got %s", want, got)
	}
}

func TestAnalyzeReRunsPipelineAndStoresResult(t *testing.T) {
	server, _ := newTestEnvironment(t)

	created := postAnalyze(t, server.URL+"/api/ai/analyze", stableMeter)
	if created.AnalysisID == "" {
		t.Fatal("expected a non-empty analysisId")
	}
	if created.MeterID != stableMeter {
		t.Fatalf("expected meter_id %q in the create response, got %q", stableMeter, created.MeterID)
	}
	if created.Status != "queued" {
		t.Fatalf("expected the run to be queued on creation, got %q", created.Status)
	}

	result := waitForAnalysis(t, server.URL+"/api/ai/analysis/"+created.AnalysisID, created.AnalysisID)

	if result.AnalysisID != created.AnalysisID {
		t.Fatalf("expected analysisId %q, got %q", created.AnalysisID, result.AnalysisID)
	}
	if result.MeterID != stableMeter {
		t.Fatalf("expected meter_id %q, got %q", stableMeter, result.MeterID)
	}
	if result.Status != "completed" {
		t.Fatalf("expected status completed, got %q", result.Status)
	}
	if result.Stage != "completed" {
		t.Fatalf("expected stage completed, got %q", result.Stage)
	}
	if result.Progress.Total != 7 || result.Progress.Done != 7 {
		t.Fatalf("expected progress 7/7, got %d/%d", result.Progress.Done, result.Progress.Total)
	}
	if result.Error != nil {
		t.Fatalf("expected a nil error on a completed run, got %q", *result.Error)
	}
	if len(result.Anomalies) != 1 {
		t.Fatalf("expected 1 stored anomaly, got %d", len(result.Anomalies))
	}
	if result.Anomalies[0].MeterID != stableMeter {
		t.Fatalf("expected only the requested meter's anomalies, got %+v", result.Anomalies[0])
	}
	if result.Anomalies[0].Severity != "HIGH" || result.Anomalies[0].Type != "REAL_ANOMALY" {
		t.Fatalf("expected the real pipeline values in the stored result, got %+v", result.Anomalies[0])
	}
	// The platform counts cover the whole run, not just the requested meter.
	if result.Platform == nil {
		t.Fatal("expected the platform block on a completed run")
	}
	if result.Platform.TotalAnomalies != 1 || result.Platform.HighPriority != 1 {
		t.Fatalf("expected platform counts 1/1, got %+v", *result.Platform)
	}
}

// TestAnalyzeProgressesFromQueuedToCompleted is the single focused test for the
// asynchronous contract: the POST answers 202 with a queued run, the GET exposes
// a real intermediate running stage while the narrative call is parked, and the
// run reaches completed with the meter's anomaly and the platform counts.
func TestAnalyzeProgressesFromQueuedToCompleted(t *testing.T) {
	llm := newGatedLLM()
	server := newDetectOnlyEnvironment(t, llm)

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(llm.release) }) }
	defer release() // never leak the worker goroutine if an assertion fails

	created := postAnalyze(t, server.URL+"/api/ai/analyze", stableMeter)
	if created.Status != "queued" {
		t.Fatalf("expected the POST to report a queued run, got %q", created.Status)
	}

	// Wait until the run is parked inside its narrative call. The explicacion
	// stage is reported immediately before that call, so this is deterministic.
	select {
	case <-llm.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the narrative stage never started")
	}

	var mid analysisBody
	getJSON(t, server.URL+"/api/ai/analysis/"+created.AnalysisID, &mid)
	if mid.Status != "running" {
		t.Fatalf("expected a running status while the narrative is parked, got %q", mid.Status)
	}
	if mid.Stage != "explicacion" {
		t.Fatalf("expected stage explicacion while the narrative is parked, got %q", mid.Stage)
	}
	if mid.Progress.Done != 5 || mid.Progress.Total != 7 {
		t.Fatalf("expected progress 5/7 during the narrative stage, got %d/%d", mid.Progress.Done, mid.Progress.Total)
	}

	release()
	result := waitForAnalysis(t, server.URL+"/api/ai/analysis/"+created.AnalysisID, created.AnalysisID)
	if result.Status != "completed" || result.Stage != "completed" {
		t.Fatalf("expected a completed run, got status %q stage %q", result.Status, result.Stage)
	}
	if result.Progress.Done != 7 || result.Progress.Total != 7 {
		t.Fatalf("expected progress 7/7 on completion, got %d/%d", result.Progress.Done, result.Progress.Total)
	}
	if len(result.Anomalies) != 1 || result.Anomalies[0].MeterID != stableMeter {
		t.Fatalf("expected exactly the meter's anomaly, got %+v", result.Anomalies)
	}
	if result.Platform == nil || result.Platform.TotalAnomalies != 1 || result.Platform.HighPriority != 1 {
		t.Fatalf("expected platform counts 1/1, got %+v", result.Platform)
	}
}

func TestAnalysisUnknownReturnsNotFound(t *testing.T) {
	server, _ := newTestEnvironment(t)

	status, body := getStatus(t, server.URL+"/api/ai/analysis/unknown-analysis-id")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown analysis id, got %d", status)
	}
	const want = `{"error":"análisis unknown-analysis-id no encontrado"}`
	if got := strings.TrimSpace(body); got != want {
		t.Fatalf("expected error body %s, got %s", want, got)
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

// TestAnomaliesAndSummaryForSingleReadingMeter is the end-to-end regression test
// for the baseline minimum-readings guard. The fixture carries one meter with
// exactly one electrically inconsistent reading, which before the guard took the
// data-quality branch of the detector, carried a NaN baseline into the anomaly
// DTO and made the JSON encoder reject the whole payload, so /api/anomalies
// answered 200 with an EMPTY body. After the guard the meter has no baseline at
// all: it is never an anomaly and is reported as unvalidated in the dashboard
// summary instead.
func TestAnomaliesAndSummaryForSingleReadingMeter(t *testing.T) {
	server, _ := newEnvironmentWithReadings(t, singleReadingReadingsCSV())

	// (a) The anomaly list must be a decodable JSON body that does NOT contain
	// the single-reading meter.
	status, raw := getStatus(t, server.URL+"/api/anomalies")
	if status != http.StatusOK {
		t.Fatalf("GET /api/anomalies: expected 200, got %d (body %q)", status, raw)
	}
	if strings.TrimSpace(raw) == "" {
		t.Fatal("GET /api/anomalies returned an empty body: a non-finite baseline reached the JSON encoder")
	}
	var list []anomalyBody
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("decode /api/anomalies: %v (body %q)", err, raw)
	}
	for _, a := range list {
		if a.MeterID == singleReadingMeter {
			t.Errorf("the meter with a single reading must never be reported as an anomaly: %+v", a)
		}
	}

	// (b) The dashboard summary must report that meter as unvalidated.
	var summary struct {
		UnvalidatedMeters struct {
			Count  int      `json:"count"`
			Meters []string `json:"meters"`
			Reason string   `json:"reason"`
		} `json:"unvalidatedMeters"`
	}
	getJSON(t, server.URL+"/api/dashboard/summary", &summary)
	if summary.UnvalidatedMeters.Count != 1 {
		t.Fatalf("expected unvalidatedMeters.count 1, got %d (%+v)",
			summary.UnvalidatedMeters.Count, summary.UnvalidatedMeters)
	}
	if want := []string{singleReadingMeter}; !reflect.DeepEqual(summary.UnvalidatedMeters.Meters, want) {
		t.Fatalf("expected unvalidatedMeters.meters %v, got %v", want, summary.UnvalidatedMeters.Meters)
	}
	if summary.UnvalidatedMeters.Reason != insufficientReadingsReason {
		t.Fatalf("expected unvalidatedMeters.reason %q, got %q",
			insufficientReadingsReason, summary.UnvalidatedMeters.Reason)
	}
}

// TestDashboardSummaryUnvalidatedMetersEmptyShape pins the stable shape of the
// new key on a fixture with no data gaps: the key is ALWAYS present, count is 0,
// and meters serialises as an empty JSON array rather than null.
func TestDashboardSummaryUnvalidatedMetersEmptyShape(t *testing.T) {
	// The shared fixture gives every meter 24 readings, so no meter is
	// unvalidated there.
	server, _ := newTestEnvironment(t)

	status, raw := getStatus(t, server.URL+"/api/dashboard/summary")
	if status != http.StatusOK {
		t.Fatalf("GET /api/dashboard/summary: expected 200, got %d (body %q)", status, raw)
	}
	var summary map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		t.Fatalf("decode summary: %v (body %q)", err, raw)
	}
	rawGaps, ok := summary["unvalidatedMeters"]
	if !ok {
		t.Fatalf("unvalidatedMeters must always be present, got keys %v (body %q)", mapKeys(summary), raw)
	}
	var gaps struct {
		Count  int             `json:"count"`
		Meters json.RawMessage `json:"meters"`
		Reason string          `json:"reason"`
	}
	if err := json.Unmarshal(rawGaps, &gaps); err != nil {
		t.Fatalf("decode unvalidatedMeters: %v (raw %s)", err, rawGaps)
	}
	if gaps.Count != 0 {
		t.Fatalf("expected count 0 on the shared fixture, got %d", gaps.Count)
	}
	if got := string(gaps.Meters); got != "[]" {
		t.Fatalf("unvalidatedMeters.meters must serialise as [] and never null, got %s", got)
	}
	if gaps.Reason != insufficientReadingsReason {
		t.Fatalf("expected reason %q, got %q", insufficientReadingsReason, gaps.Reason)
	}
}

// mapKeys returns the sorted keys of a decoded JSON object, so a failure message
// lists the published fields instead of only their count.
func mapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

	created := postAnalyze(t, server.URL+"/api/ai/analyze", stableMeter)

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

	// The run must still reach a terminal state after all the concurrent reads.
	waitForAnalysis(t, server.URL+"/api/ai/analysis/"+created.AnalysisID, created.AnalysisID)
}
