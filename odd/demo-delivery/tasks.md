# Backend features delivered for the demo

**Status: DONE — merged into `main` by PR #7.** Per-meter asynchronous analysis (`310fc6d`), the flow login
(`3d2fed1`), the API surface for the dashboard and the meters cards (`dce7964`), the consumption-determinism fix
(`effd9b8`), and the documentation plus the `.env.example` (`0218e19`, `3c705a5`).

**Why this record exists.** This repository's own ODD trail stopped at `baseline-min-two-readings`, so the last
three features had no record here. The detailed process records live in the **frontend** repository's `odd/tasks/`
(`per-meter-analysis-progress.md`, `auth-flow.md`, `demo-polish.md`) because the work was coordinated across both
repos, and the canonical contract is `docs/endpoints.md`. This file closes the provenance gap without duplicating
either.

## What changed

### 1. Per-meter asynchronous analysis with real stage progress

- `POST /api/ai/analyze` takes `{"meter_id"}` and answers **202** with an `analysisId`; the pipeline runs in a
  goroutine. A second POST while a run for that meter is in flight returns the **existing** id instead of
  starting a second LLM call. A missing or unknown meter is a 400.
- `GET /api/ai/analysis/{id}` reports `status`, `stage`, `progress`, the timestamps, that meter's anomalies, the
  platform counters and, on failure, a readable reason. `anomalies` is always an array, never null.
- The seven stages are reported for real, and `progress.done` is derived from the frozen stage order, so it only
  ever counts stages that settled. One LLM call has no partial progress, so nothing fakes a percentage.
- Guardrails: a 90 s watchdog, a `recover()` that turns a panic into a failed run, and a terminal-state check on
  every mutation so a late completion cannot resurrect a run the watchdog failed.
- **The LLM no longer runs at start-up.** Only `Detect()` runs before `ListenAndServe`; the narrative is produced
  on demand. This deliberately **reversed** the decision recorded in `odd/async-llm-enrichment/tasks.md`.
- Terminal runs are appended to `data/analyses.json`: a write-only audit artifact, never a read path for the API.

### 2. Flow login (JWT)

`data/users.csv` holds the single demo user (the password only as a SHA-256), and `POST /api/auth/login` issues an
HS256 token built with the standard library alone — no dependency was added. The signing secret comes from
`JWT_SECRET` with a documented development fallback. The backend **only issues** the token and validates it on no
other route: that is the requested scope for a flow demo, and `docs/architecture.md` §9 records it as deliberate.

### 3. API surface for the dashboard and the meters cards

- The summary gained `total_consumption` and a real `lastRun` as an RFC3339 UTC timestamp, replacing the literal
  `"latest"` placeholder that was never a timestamp.
- `GET /api/meters` returns objects (`id`, `consumption`, `status`, `readings_count`, `last_reading_at`) instead
  of a bare array of ids, sorted by id. This is a **deliberate breaking change**: it is the only way to feed
  per-meter consumption to the cards without one request per meter, and it removed a recorded N+1 advisory.

## Verification, and what is still open

Two independent read-only verifications. The one over the analysis work recomputed the HS256 signature bit for bit
and confirmed the implementation is correct; the one over the demo work proved the `total_consumption` defect
below. Two defects were fixed as a result.

**Known limits, deliberately left unfixed and recorded here so they are not rediscovered as surprises:**

1. The watchdog is 90 s while a single LLM call can take up to 60 s, so a meter with two or more anomalies could
   be marked `failed` while it is healthy. The bundled dataset has one anomaly per meter, so it does not fire.
2. `analysisStore` never evicts: records accumulate for the lifetime of the process.
3. The audit log re-reads and rewrites the whole file on every terminal record under the global mutex, and a read
   error makes the next append overwrite the previous history.
4. The summary reads its fields under separate lock acquisitions, so a `Detect` landing between them can mix
   generations across fields. Each field is internally consistent; the summary as a whole is not atomic.
5. The race detector cannot run in this environment (`-race requires cgo`, and no `gcc`), so the "race-free" claim
   rests on inspection of the locks and on the suite without `-race`.
6. Two clauses of the frozen contract have no test: the 400 for a missing or unknown `meter_id`, and the in-flight
   dedupe. Both are implemented and were verified by inspection.

## Pointers

- Contract: `docs/endpoints.md`. Architecture: `docs/architecture.md` §9.
- Cross-repo process records: the frontend repository's `odd/tasks/` (`per-meter-analysis-progress.md`,
  `auth-flow.md`, `demo-polish.md`).
- The audit-log path is overridable from tests only, through `handlers.SetAnalysisLogPath`.
