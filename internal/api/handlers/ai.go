package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neuralium/ai-energy/internal/analysis"
)

// analysisStore holds the live record of every analysis run, keyed by the
// generated analysis id. Unlike the old whole-platform snapshot map, a record is
// mutable while its run is in flight: the goroutine started by
// POST /api/ai/analyze advances its status, stage and progress in place. A
// single mutex guards the map and every record in it, so concurrent readers for
// the same id never observe a half-written record and never race the writer.
var (
	analysisStore = make(map[string]*analysisRecord)
	storeMutex    sync.Mutex
)

// Analysis run statuses published in the status field. "queued" is set before
// the goroutine starts, "running" once the pipeline reports a stage.
const (
	statusQueued    = "queued"
	statusRunning   = "running"
	statusCompleted = "completed"
	statusFailed    = "failed"
)

// Stage ids that are not pipeline stages but are still published in the stage
// field: the initial queued marker and the two terminal markers.
const (
	stageQueued    = "queued"
	stageCompleted = "completed"
	stageFailed    = "failed"
)

// progressTotal is the number of pipeline stages the client renders: the five
// deterministic stages plus the two narrative stages.
const progressTotal = 7

// analysisWatchdog bounds one analysis run. The provider already caps each LLM
// call at 60 s; this is the whole-run ceiling, so a stuck pipeline always ends
// in a terminal "failed" record and the client's spinner can never hang.
const analysisWatchdog = 90 * time.Second

// analysisStages is the frozen pipeline order. The index of a stage is exactly
// the number of stages that have already settled when that stage is the current
// one, so progress.done is derived from it and never invented.
var analysisStages = []string{
	analysis.StageLecturas,
	analysis.StageBaseline,
	analysis.StageDeteccion,
	analysis.StageCorrelacion,
	analysis.StageEventos,
	analysis.StageExplicacion,
	analysis.StageRecomendacion,
}

func stageIndex(stage string) int {
	for i, s := range analysisStages {
		if s == stage {
			return i
		}
	}
	return 0
}

// analysisRecord is the mutable, in-memory state of one analysis run. It is
// created as queued, advanced by the run's goroutine and read by the GET
// handler, always under storeMutex.
//
// Once a record reaches a terminal status (completed or failed) it is never
// written again, so a result body built from it stays stable.
//
// A record holds only the requested meter's anomaly DTOs, mapped through the
// shared anomalyDTOs helper, plus the whole-platform counts produced by the same
// run.
type analysisRecord struct {
	meterID      string
	status       string
	stage        string
	progressDone int
	startedAt    time.Time
	finishedAt   *time.Time
	err          string
	anomalies    []AnomalyDTO
	platform     *analysisPlatformDTO
}

// analysisProgressDTO is the progress block of the analysis result.
type analysisProgressDTO struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// analysisPlatformDTO is the whole-platform count produced by the same run as
// the per-meter anomalies. TotalAnomalies counts every evidence item;
// HighPriority counts those whose severity is HIGH.
type analysisPlatformDTO struct {
	TotalAnomalies int `json:"total_anomalies"`
	HighPriority   int `json:"high_priority"`
}

// analysisCreatedDTO is the 202 body of POST /api/ai/analyze.
type analysisCreatedDTO struct {
	AnalysisID string `json:"analysisId"`
	MeterID    string `json:"meter_id"`
	Status     string `json:"status"`
}

// analysisResultDTO is the body returned by GET /api/ai/analysis/{id}. The
// shape is frozen: the frontend client is written against it. Platform is only
// present once the run completes, and Error is null unless the run failed.
type analysisResultDTO struct {
	AnalysisID string               `json:"analysisId"`
	MeterID    string               `json:"meter_id"`
	Status     string               `json:"status"`
	Stage      string               `json:"stage"`
	Progress   analysisProgressDTO  `json:"progress"`
	StartedAt  string               `json:"started_at"`
	FinishedAt *string              `json:"finished_at"`
	Anomalies  []AnomalyDTO         `json:"anomalies"`
	Platform   *analysisPlatformDTO `json:"platform,omitempty"`
	Error      *string              `json:"error"`
}

// meterDTO is the body returned by GET /api/meters/{meterId}.
type meterDTO struct {
	ID            string `json:"id"`
	MeterID       string `json:"meter_id"`
	Name          string `json:"name"`
	Location      string `json:"location"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	ReadingsCount int    `json:"readings_count"`
	LastReadingAt string `json:"last_reading_at"`
}

// AnalyzePOST starts an asynchronous, per-meter analysis run.
//
// The request body carries the target meter. A missing or unknown meter is a
// 400. For a known meter the handler mints an id, records it as "queued" and
// answers 202 immediately, while the pipeline runs in a goroutine. If a run for
// the same meter is already queued or running, the existing id is returned with
// 202 and no second LLM call is started.
func AnalyzePOST(orchestrator *analysis.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MeterID string `json:"meter_id"`
		}
		// A missing or malformed body simply leaves MeterID empty, which the
		// validation below rejects with the same user-visible 400.
		_ = json.NewDecoder(r.Body).Decode(&req)
		meterID := strings.TrimSpace(req.MeterID)
		if meterID == "" {
			writeJSONError(w, http.StatusBadRequest, "meter_id es obligatorio")
			return
		}
		if !meterExists(orchestrator, meterID) {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("medidor %s no encontrado", meterID))
			return
		}

		storeMutex.Lock()
		// Return the in-flight run for this meter instead of starting a second
		// one, so pressing the button twice never issues a second LLM call.
		for id, rec := range analysisStore {
			if rec.meterID == meterID && !isTerminal(rec) {
				status := rec.status
				storeMutex.Unlock()
				writeJSON(w, http.StatusAccepted, analysisCreatedDTO{
					AnalysisID: id, MeterID: meterID, Status: status,
				})
				return
			}
		}
		analysisID := uuid.New().String()
		analysisStore[analysisID] = &analysisRecord{
			meterID:   meterID,
			status:    statusQueued,
			stage:     stageQueued,
			startedAt: time.Now().UTC(),
		}
		storeMutex.Unlock()

		go runMeterAnalysis(orchestrator, analysisID, meterID)

		writeJSON(w, http.StatusAccepted, analysisCreatedDTO{
			AnalysisID: analysisID, MeterID: meterID, Status: statusQueued,
		})
	}
}

// meterExists reports whether the meter is known to the reading repository. The
// repository is populated by the synchronous Detect at start-up, so an unknown
// meter is a client error rather than an empty analysis.
func meterExists(orchestrator *analysis.Orchestrator, meterID string) bool {
	for _, id := range orchestrator.ReadingRepo.AllMeterIDs() {
		if id == meterID {
			return true
		}
	}
	return false
}

// isTerminal reports whether a run can no longer change. Terminal records are
// never overwritten, so a late completion cannot resurrect a run the watchdog
// already failed.
func isTerminal(rec *analysisRecord) bool {
	return rec.status == statusCompleted || rec.status == statusFailed
}

// runMeterAnalysis executes one asynchronous run and always leaves a terminal
// record behind.
func runMeterAnalysis(orchestrator *analysis.Orchestrator, id, meterID string) {
	// The watchdog marks the run failed even while the goroutine is still parked
	// inside a provider call, which is the only way the client can never wait
	// forever. It is stopped as soon as the goroutine returns.
	watchdog := time.AfterFunc(analysisWatchdog, func() {
		failAnalysis(id, fmt.Sprintf("el análisis superó el tiempo máximo de %s", analysisWatchdog))
	})
	defer watchdog.Stop()
	// A panic anywhere in the pipeline must surface as a failed run rather than
	// as a silent goroutine death that leaves the record non-terminal forever.
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("analysis %s panicked: %v", id, recovered)
			failAnalysis(id, fmt.Sprintf("error interno del análisis: %v", recovered))
		}
	}()

	setAnalysisRunning(id)
	report := func(stage string) { setAnalysisStage(id, stage) }

	result, err := orchestrator.RunMeter(meterID, report)
	if err != nil {
		failAnalysis(id, err.Error())
		return
	}
	completeAnalysis(id, result)
}

// setAnalysisRunning flips a freshly created record to running.
func setAnalysisRunning(id string) {
	storeMutex.Lock()
	if rec := analysisStore[id]; rec != nil && !isTerminal(rec) {
		rec.status = statusRunning
	}
	storeMutex.Unlock()
}

// setAnalysisStage records the current pipeline stage and derives progress.done
// from the frozen stage order, so done only ever reflects stages that settled.
func setAnalysisStage(id, stage string) {
	storeMutex.Lock()
	if rec := analysisStore[id]; rec != nil && !isTerminal(rec) {
		rec.status = statusRunning
		rec.stage = stage
		rec.progressDone = stageIndex(stage)
	}
	storeMutex.Unlock()
}

// completeAnalysis stores the finished result and appends the terminal record to
// the write-only audit log. A record already marked failed by the watchdog is
// left untouched: terminal states are never overwritten.
func completeAnalysis(id string, result analysis.MeterAnalysis) {
	platform := &analysisPlatformDTO{
		TotalAnomalies: result.TotalAnomalies,
		HighPriority:   result.HighPriority,
	}
	now := time.Now().UTC()

	storeMutex.Lock()
	rec := analysisStore[id]
	if rec == nil || isTerminal(rec) {
		storeMutex.Unlock()
		return
	}
	rec.status = statusCompleted
	rec.stage = stageCompleted
	rec.progressDone = progressTotal
	rec.finishedAt = &now
	rec.err = ""
	rec.anomalies = anomalyDTOs(result.Evidence)
	rec.platform = platform
	appendAnalysisLogLocked(analysisLogRecord{
		AnalysisID:   id,
		MeterID:      rec.meterID,
		Status:       rec.status,
		StartedAt:    rec.startedAt.UTC().Format(time.RFC3339),
		FinishedAt:   formatTimePtr(rec.finishedAt),
		AnomalyCount: len(rec.anomalies),
		Platform:     platform,
	})
	storeMutex.Unlock()
}

// failAnalysis marks the run failed with a readable reason and appends the
// terminal record to the audit log. A completed record is never downgraded.
func failAnalysis(id, message string) {
	now := time.Now().UTC()

	storeMutex.Lock()
	rec := analysisStore[id]
	if rec == nil || isTerminal(rec) {
		storeMutex.Unlock()
		return
	}
	rec.status = statusFailed
	rec.stage = stageFailed
	rec.finishedAt = &now
	rec.err = message
	appendAnalysisLogLocked(analysisLogRecord{
		AnalysisID:   id,
		MeterID:      rec.meterID,
		Status:       rec.status,
		StartedAt:    rec.startedAt.UTC().Format(time.RFC3339),
		FinishedAt:   formatTimePtr(rec.finishedAt),
		Error:        &message,
		AnomalyCount: len(rec.anomalies),
		Platform:     rec.platform,
	})
	storeMutex.Unlock()
}

// formatTimePtr formats an optional timestamp as RFC3339, or nil so the JSON
// field is null rather than the zero time.
func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// GET /api/ai/analysis/{id} – retrieve the live state of a previous analysis.
func AnalysisGET(w http.ResponseWriter, r *http.Request) {
	// Normalize path: the router mounts everything under /api.
	path := strings.TrimPrefix(r.URL.Path, "/api")
	// Expected pattern: /ai/analysis/{id}
	prefix := "/ai/analysis/"
	if !strings.HasPrefix(path, prefix) {
		writeJSONError(w, http.StatusNotFound, "análisis no encontrado")
		return
	}
	id := strings.TrimPrefix(path, prefix)
	storeMutex.Lock()
	rec, ok := analysisStore[id]
	var dto analysisResultDTO
	if ok {
		dto = analysisResultFor(id, rec)
	}
	storeMutex.Unlock()
	if !ok {
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("análisis %s no encontrado", id))
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// analysisResultFor builds the frozen GET body from a live record. The caller
// must hold storeMutex. Anomalies is always an array, and never null.
func analysisResultFor(id string, rec *analysisRecord) analysisResultDTO {
	anomalies := rec.anomalies
	if anomalies == nil {
		anomalies = []AnomalyDTO{}
	}
	dto := analysisResultDTO{
		AnalysisID: id,
		MeterID:    rec.meterID,
		Status:     rec.status,
		Stage:      rec.stage,
		Progress:   analysisProgressDTO{Done: rec.progressDone, Total: progressTotal},
		StartedAt:  rec.startedAt.UTC().Format(time.RFC3339),
		FinishedAt: formatTimePtr(rec.finishedAt),
		Anomalies:  anomalies,
		Platform:   rec.platform,
	}
	if rec.err != "" {
		message := rec.err
		dto.Error = &message
	}
	return dto
}

// GET /api/dashboard/summary – provide a high‑level summary for the UI.
func DashboardSummary(orchestrator *analysis.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gaps := orchestrator.DataGaps()
		// meterIDs stays non-nil even with no gaps, so the JSON field is [] and
		// never null and clients never have to branch on a missing array.
		meterIDs := make([]string, 0, len(gaps))
		for _, gap := range gaps {
			meterIDs = append(meterIDs, gap.MeterID)
		}
		summary := map[string]any{
			"health":    "ok",
			"meters":    len(orchestrator.ReadingRepo.AllMeterIDs()),
			"anomalies": len(orchestrator.Evidence()),
			"lastRun":   "latest", // placeholder
			// unvalidatedMeters reports the meters that survived the quality
			// check but could not be validated (too few readings for a baseline).
			// The key is ALWAYS present, even with a count of 0, so the response
			// shape is stable. These meters are deliberately absent from
			// /api/anomalies: a data gap is not an anomaly.
			"unvalidatedMeters": map[string]any{
				"count":  len(gaps),
				"meters": meterIDs,
				"reason": analysis.InsufficientReadingsReason,
			},
		}
		writeJSON(w, http.StatusOK, summary)
	}
}

// GET /api/meters/{meterId} – return real, derivable meter metadata.
//
// The values come from the reading repository. This project has no data source
// for a meter's name or location, so Name and Location are always returned as
// empty strings rather than invented values.
func MeterDetail(orchestrator *analysis.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api")
		prefix := "/meters/"
		if !strings.HasPrefix(path, prefix) {
			writeJSONError(w, http.StatusNotFound, "medidor no encontrado")
			return
		}
		id := strings.TrimPrefix(path, prefix)
		if idx := strings.Index(id, "/"); idx != -1 {
			id = id[:idx]
		}
		readings := orchestrator.ReadingRepo.ReadingsFor([]string{id}, nil, nil)
		if len(readings) == 0 {
			writeJSONError(w, http.StatusNotFound, fmt.Sprintf("medidor %s no encontrado", id))
			return
		}
		sort.Slice(readings, func(i, j int) bool {
			return readings[i].Timestamp.Before(readings[j].Timestamp)
		})
		// A meter is healthy only when every reading carries an OK status.
		status := "OK"
		for _, reading := range readings {
			if !strings.EqualFold(strings.TrimSpace(reading.Status), "OK") {
				status = "DEGRADED"
				break
			}
		}
		resp := meterDTO{
			ID:            id,
			MeterID:       id,
			Name:          "", // no meter-name source exists in this project
			Location:      "", // no meter-location source exists in this project
			Status:        status,
			CreatedAt:     readings[0].Timestamp.UTC().Format(time.RFC3339),
			ReadingsCount: len(readings),
			LastReadingAt: readings[len(readings)-1].Timestamp.UTC().Format(time.RFC3339),
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// GET /api/anomalies/{id} – returns the full anomaly detail for the stable id
// published by the list endpoint. Every field comes from the real evidence.
func AnomalyDetailByID(orchestrator *analysis.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api")
		prefix := "/anomalies/"
		if !strings.HasPrefix(path, prefix) {
			writeJSONError(w, http.StatusNotFound, "anomalía no encontrada")
			return
		}
		id := strings.TrimPrefix(path, prefix)
		for _, ev := range orchestrator.Evidence() {
			if AnomalyID(ev) == id {
				writeJSON(w, http.StatusOK, newAnomalyDTO(ev))
				return
			}
		}
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("anomalía %s no encontrada", id))
	}
}
