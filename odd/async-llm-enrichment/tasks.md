# Feature: async-llm-enrichment

**Branch:** `fix/non-blocking-startup`
**Base (branch point):** `5abb2b6` (main)
**Created:** 2026-09-27

## Problem

The frontend receives `ERR_CONNECTION_REFUSED` from `http://localhost:3001/api/health`
while the backend is starting.

Root cause is two-fold, both in the startup path:

1. `cmd/api/main.go:45-52` runs `orchestrator.Run()` **before**
   `http.ListenAndServe`. The listening socket does not exist until the whole
   pipeline, including the LLM calls, has finished.
2. `internal/analysis/orchestrator.go:88-101` runs the LLM enrichment loop
   **while holding `o.mu.Lock()`**. `Evidence()` takes `o.mu.RLock()`, so every
   read endpoint (`/api/anomalies`, `/api/reports`, `/api/dashboard/summary`,
   `/api/ai/analysis/{id}`) blocks for the duration of the enrichment.

Measured dead window before the port accepts connections, with the real Ollama
provider and the shipped dataset (4 anomalies):

| Provider | Launch -> port listening | Evidence |
| --- | --- | --- |
| real (Ollama `gpt-oss:20b`) | **69 s** | log: `23:39:51 Loaded 4032 readings` / `23:40:59 Server starting on port 3009` |
| deterministic mock | **0.57 s** | verification run of `async` proof-of-concept |

Naively moving `Run()` into a goroutine is **not** a sufficient fix: the port
would open but every read request would still stall on the write lock, turning
`connection refused` into request timeout.

## Design decision

Chosen by the user (2026-09-27), option "deterministic first, LLM in background":

- Split the pipeline into a fast deterministic stage and a slow narrative stage.
- Publish the deterministic evidence **first**, under a short lock.
- Run LLM enrichment **outside** the lock, in the background, updating evidence
  progressively under short locks.
- `GET /api/health` keeps its current contract: 200 `{"status":"ok"}`, available
  immediately.
- `GET /api/anomalies` returns the **complete, correct** deterministic payload
  from the first moment; `llm_analysis` may be `""` until enrichment finishes
  (~68 s on the shipped dataset).
- No `503 warming` state is introduced (explicitly rejected).

## Checks

- **TDD mode:** strict RED -> GREEN -> REFACTOR
- **TDD source:** explicit user choice, 2026-09-27
- **Runner:** `go test ./...`
- **Race detector:** verified available on this machine
  (`go test -race ./internal/analysis/` compiles and runs, gcc/clang present).

## Delivery strategy

`ask-on-risk` (default). Forecast authored changed lines (additions + deletions):

| File | Estimate |
| --- | --- |
| `internal/analysis/orchestrator.go` | ~95 |
| `internal/analysis/orchestrator_test.go` | ~80 |
| `cmd/api/main.go` | ~18 |
| `internal/api/handlers/ai.go` | ~23 |
| `docs/endpoints.md` | ~14 |
| **Total** | **~230** |

The forecast exceeded the ~400 authored-line budget on WU1 (407 authored lines:
139 fix + 268 tests), so the `ask-on-risk` strategy was applied at that point.

**Resolved strategy: `single-pr` (decided by the user, 2026-09-27).** Rationale
recorded: 66% of the diff is tests, the fix itself is ~139 lines, and splitting
the concurrency test away from the lock change would make the change
unreviewable. One PR, one idea. Projected total ~444 authored lines. Do not ask
again for this feature.

Running authored-line count:

| Work unit | Authored lines | Cumulative |
| --- | --- | --- |
| WU1 | 407 | 407 |

## Route declaration

| Work unit | Route | Trigger evidence |
| --- | --- | --- |
| WU1 | delegated (`gentle-ai-worker`) | Writer trigger: 3 non-trivial files (`orchestrator.go`, `orchestrator_test.go`, `main.go`) |
| WU2 | delegated (same writer thread, sequential) | Writer trigger: 1 file, but tightly coupled to WU1's new API |
| WU3 | delegated (same writer thread, sequential) | Preparation trigger: `docs/endpoints.md` is a large doc that must be read to place the note correctly |
| WU4 | delegated (`gentle-ai-verify`) | Verification rule: executes commands |

One writer thread only. No parallel writers in this worktree.

## Tasks

- [x] **WU1 — Non-blocking startup core.** DONE.

      RED, observed against the pre-fix code before implementing, and then
      **independently reproduced by the parent** in an isolated `/tmp` worktree
      built from the branch point (`git archive HEAD`), running only the
      governing test:

      ```
      red_test.go:50: Evidence() was blocked while LLM enrichment was running
      --- FAIL: TestEvidenceNotBlockedDuringEnrichment (2.01s)
      ```

      GREEN: `go test ./internal/analysis/ -run TestEvidenceNotBlocked -v -count=1`
      -> `--- PASS: TestEvidenceNotBlockedDuringEnrichment (0.01s)`.

      Checks: `go test ./... -count=1` PASS (whole suite);
      `go test -race ./internal/analysis/ -count=1` PASS; `gofmt -l` on the
      touched files clean; `go vet ./internal/analysis/ ./cmd/api/` clean.
      Parent spot check re-ran the race check independently: PASS.

      Route: delegated to `gentle-ai-worker` (writer trigger: 3 non-trivial
      files). Authored lines: 407 (139 fix + 268 tests).

      Delivered shape: `Detect()` runs the deterministic pipeline outside the
      lock and publishes under a brief write lock; `Enrich()` snapshots under a
      read lock, calls the LLM with no lock held, and writes each item's
      `LLMText` under a short per-item write lock; `Run()` is `Detect()` +
      `Enrich()` and keeps its signature and post-conditions; `cmd/api/main.go`
      runs `Detect()` synchronously (fatal on error), starts `Enrich()` in a
      goroutine with start/finish logging, and no longer delays
      `ListenAndServe`. `Enrich()` carries an English comment stating the lock
      must not be collapsed back over the LLM loop.
- [ ] **WU2 — `POST /api/ai/analyze` must not block.** The handler currently
      calls `Run()` and would hold the request for ~68 s. Make it publish the
      deterministic snapshot and return immediately, with enrichment continuing
      in the background. Commit.
- [ ] **WU3 — Document the warm-up contract.** In `docs/endpoints.md`, state
      that `llm_analysis` may be `""` for a period after startup while the rest
      of the anomaly payload is already complete and correct. Commit.
- [ ] **WU4 — Closure verification.** `go test ./...`, `go test -race ./...`,
      and a live run measuring launch-to-listening (expect ~1 s, not 69 s) plus
      confirming `/api/anomalies` returns the 4 benchmark cases immediately.
      Evidence only, no commit.

## Acceptance evidence

- Launch-to-listening drops from 69 s to about 1 s with the real provider.
- `/api/anomalies` returns M-104 `EXPLAINABLE_ANOMALY`/`MEDIUM`,
  M-106 `FALSE_POSITIVE`/`LOW`, M-109 `REAL_ANOMALY`/`HIGH`,
  M-112 `DATA_QUALITY`/`HIGH` from the first request.
- No read endpoint is blocked during enrichment (concurrency test, `-race`).
- Full suite green; the 4 benchmark assertions in
  `internal/analysis/requirements_dataset_test.go` still pass.

## Non-goals

- No `503 warming` state, no new readiness endpoint.
- No change to `GET /api/health`'s response contract.
- No fix of the wider documentation drift in this feature (`openspec/` still
  names chi and `/api/v1`, `docs/routing.md:35` has a stale `NewRouter`
  signature, `docs/endpoints.md` under-documents the anomaly DTO). Tracked as a
  separate future feature.

## Out-of-scope discovery for follow-up

- The team's earlier note that the race detector was unavailable ("no gcc") is
  stale: it works on this machine.
