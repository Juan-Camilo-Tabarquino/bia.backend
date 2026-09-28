package handlers

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// analysisLogRecord is one terminal entry of the write-only audit log. Its field
// names mirror the HTTP contract where the concepts coincide, but the log is an
// independent artifact: it is never read back into the analysis id space.
type analysisLogRecord struct {
	AnalysisID   string               `json:"analysisId"`
	MeterID      string               `json:"meter_id"`
	Status       string               `json:"status"`
	StartedAt    string               `json:"started_at"`
	FinishedAt   *string              `json:"finished_at"`
	Error        *string              `json:"error"`
	AnomalyCount int                  `json:"anomaly_count"`
	Platform     *analysisPlatformDTO `json:"platform"`
}

// analysisLogPath is where terminal analysis records are appended. It defaults
// to the repository-relative path the product documents and can be redirected
// by tests through SetAnalysisLogPath.
//
// The log is write-only for the API: the in-memory analysis store remains the
// single source of truth for every analysis id, and this file is never read
// back into the id space. An id therefore does not survive a process restart,
// which is the documented behaviour.
var analysisLogPath = filepath.Join("data", "analyses.json")

// SetAnalysisLogPath redirects the write-only audit log. It exists as a test
// seam so the suite never writes into the repository's data directory; the
// production binary never calls it.
func SetAnalysisLogPath(path string) {
	// Guarding the assignment with the same mutex that serialises appends keeps
	// the path read in appendAnalysisLogLocked race-free.
	storeMutex.Lock()
	analysisLogPath = path
	storeMutex.Unlock()
}

// appendAnalysisLogLocked appends one terminal record to the audit log with an
// atomic replace: the whole file is rewritten to a temporary sibling and then
// renamed over the destination, so a crash mid-write can never leave a
// truncated log.
//
// The caller must hold storeMutex, which serialises every append. The append is
// best-effort by design: the audit log is a write-only side artifact, so a
// filesystem failure must never turn a completed analysis into a failed
// request.
func appendAnalysisLogLocked(record analysisLogRecord) {
	path := analysisLogPath
	existing := readAnalysisLogLocked(path)
	existing = append(existing, record)

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		log.Printf("analysis audit log: marshal %s: %v", path, err)
		return
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("analysis audit log: create dir for %s: %v", path, err)
			return
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("analysis audit log: write %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("analysis audit log: rename %s: %v", tmp, err)
	}
}

// readAnalysisLogLocked loads the existing audit entries so an append can
// preserve them. The caller holds storeMutex. A missing or unreadable file is
// treated as an empty log: this read serves the append only and is never the
// source of truth for an analysis id.
func readAnalysisLogLocked(path string) []analysisLogRecord {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var existing []analysisLogRecord
	if err := json.Unmarshal(data, &existing); err != nil {
		log.Printf("analysis audit log: ignoring unreadable %s: %v", path, err)
		return nil
	}
	return existing
}
