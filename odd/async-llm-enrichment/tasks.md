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
| Work unit | Authored lines | Cumulative |
| --- | --- | --- |
| WU1 | 407 | 407 |
| WU2 | 0 (cancelled after investigation) | 407 |
| WU3 (docs) | 34 | 441 |

The scope shrank materially when WU2 was cancelled, so the `single-pr` decision
still stands and no chain is needed. Do not ask again for this feature.

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
- [ ] **WU2 — CANCELLED after investigation (no code written).** The original
      intent was to make `POST /api/ai/analyze` non-blocking. Evidence collected
      from the frontend repository `bia.frontend` shows that the synchronous
      contract is a **deliberate, documented UX decision**, not an accident:

      - `src/components/anomalies/AiReanalysis.tsx:33-42` states that the POST
        "is synchronous: in a single request it re-runs the deterministic
        pipeline and then the LLM narrative, so it can legitimately stay pending
        for about a minute" and that `GET /api/ai/analysis/{id}` "always answers
        `"status":"completed"`, so there is no pending state to poll".
      - The same component renders an explicit latency disclosure to the user
        (`LATENCY_HELPER`, "Puede tardar alrededor de un minuto") and a running
        status message while the request is in flight.
      - The component consumes `topAnomaly.llm_analysis` directly as the main
        content of its result block, and it never polls.

      Making the POST return early would therefore render "Análisis completado"
      over an empty narrative — a worse defect than the one it would fix. The
      decision (user, 2026-09-27) is to leave `POST /api/ai/analyze` synchronous.
      If asynchrony is ever wanted, it is a coordinated two-repository change
      (the frontend must poll `status: "queued"` → `"completed"`), not a backend
      only change.

      Residual risk accepted and recorded: a proxy or network timeout between
      frontend and backend could cut a ~68 s request. Not observed locally.
- [x] **WU3 — Document the warm-up contract.** DONE. Added a
      "Startup warm-up and the LLM narrative window" section to
      `docs/endpoints.md`, placed after the route summary and before the
      per-endpoint reference so a reader sees it before any individual
      endpoint. `docs/endpoints.md` +34/-0.

      Parent verification: the worker's load-bearing claim was that
      `llm_analysis` carries `omitempty`, which is what makes the "the key may
      be absent rather than present as `''`" wording correct. Confirmed
      independently at `internal/api/handlers/endpoints.go:71`:
      ``LLMAnalysis string `json:"llm_analysis,omitempty"` ``. Parent spot
      check re-ran `git status --porcelain` and `git diff --stat`: exactly the
      two expected files, no third one.

      Review authority: the documentation candidate was reviewed natively and
      closed on its own at tier `low` with reason `non_executable_only`, no
      lenses required (lineage `review-9682fa058ba5aa19`), then acknowledged
      with `authority: burned`.
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

## RDD candidate outcome (WU1, commit `c3e75de`)

`gentle_review {"operation":"assess"}` over the committed range, with the branch
point as `baseRef` and `committedOnly: true`, **failed**: `risk: "unassessable"`,
reason `schema-incompatible`, `changedPaths: 0`, `changedLines: 0`,
`candidate: null`. It was reproduced twice, the second time with the documented
minimal input, so the optional `writerModelId` field is not the cause.

Per the reviewed contract, a failed assessment is treated exactly like high
risk: `writerSelfVerification: true` and `independentVerifier: true`. WU4 must
therefore include a separate independent verifier run (`gentle-ai-verify`); the
writer report alone is not the verification of record for this candidate.

The failure is a known upstream defect tracked as
`Gentleman-Programming/gentle-ai#4791` (open, no published fix on the 3.7.0
stable pairing; a field-for-field identical occurrence is already recorded in
that thread). The user authorized reporting it. The occurrence comment was
prepared and passed its final privacy scan, but could not be published: the
`POST` to the issue comments endpoint returned `HTTP 401 Requires
authentication`, and this machine holds no write credential (no `gh` CLI, no
token). A permission failure ends further GitHub mutation, with no blind retry
and no substitute command, so consumer state was preserved and the work resumed
under the documented fallback. The prepared comment is recoverable from Engram.

## Non-goals

- No `503 warming` state, no new readiness endpoint.
- No change to `POST /api/ai/analyze`: it stays synchronous by decision (see
  WU2).
- No fix of the wider documentation drift in this feature (`openspec/` still
  names chi and `/api/v1`, `docs/routing.md:35` has a stale `NewRouter`
  signature, `docs/endpoints.md` under-documents the anomaly DTO). Tracked as a
  separate future feature.

## Out-of-scope discovery for follow-up

- The team's earlier note that the race detector was unavailable ("no gcc") is
  stale: it works on this machine.
