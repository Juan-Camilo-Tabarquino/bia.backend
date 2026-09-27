# Feature: baseline-min-two-readings

**Branch:** `fix/baseline-min-two-readings`
**Base:** `210f727` (tip of `main`)
**Created:** 2026-09-27

## Problem

`internal/analysis/baseline.go:67` computes `variance := varSum / float64(cnt-1)`
with no guard for `cnt == 1`. With a single reading per meter, `diff` is
identically `0`, so `varSum == 0` and `variance` is `0/0`, i.e. **`NaN`** — never
`+Inf` (the wording in the originating backlog item is imprecise).

The `NaN` is not merely a degraded comparison. It travels into
`models.Baseline.StdDev`, is carried by every candidate
(`detector.go:74` `Baseline: b`), and reaches the `baseline.stddev` field of the
`/api/anomalies` DTO and the raw `models.Evidence` serialized by `/api/reports`.
`json.Marshal` rejects `NaN` with `json: unsupported value: NaN`, and **both**
handlers discard the encode error (`handlers/endpoints.go:206` `writeJSON`,
`router.go:112` `_ =`), so the response is a `200` with an empty body.

Reachability: `qualityChecker.Check` (`quality.go:31-45`) only drops readings with
negative consumption or a non-`ok` status. A meter with exactly one surviving
reading is therefore reachable from real input. The shipped dataset never
triggers it (12 meters, `data/readings.csv` gives every meter 336 readings, all
`status=OK`), and no test covered it.

Consumers of `models.Baseline`: `handlers/endpoints.go:150-157` (`BaselineDTO`),
`router.go:106-116` (`/api/reports`, raw struct), `detector.go:82` (sigma term of
the spike rule). The drop rule (`detector.go:84`) does **not** use `StdDev` —
another imprecision in the backlog.

## Decisions (user, 2026-09-27)

1. **Hardening rule.** `Calculate` requires **at least 2 readings per meter**.
   Below that it does not compute a baseline at all, so a non-finite `StdDev`
   becomes structurally impossible rather than merely unlikely.
2. **Surface for the warning.** The meter must **not** appear in
   `/api/anomalies` (no fabricated anomaly, no new `AnomalyType`/`AnomalyKind`).
   The warning is exposed as a count of unvalidated meters in
   `/api/dashboard/summary`.
   *Rejected alternatives:* `StdDev = 0` with the meter still evaluated (the
   sigma term degenerates to `r.Consumption > b.Mean`, producing false spikes);
   a new `INSUFFICIENT_DATA` type (contract growth with no requirement behind it);
   total silence (the user asked for a visible "not enough information" signal).
3. **Documentation scope.** Full alignment: the docs that describe this change,
   the four documented drift defects (B1-B4), and the stale ODD bookkeeping.

## Scope

In scope:

1. `internal/analysis/baseline.go` — the `cnt < 2` guard.
2. The unvalidated-meter signal: analysis-side computation, orchestrator storage
   and accessor, and the `/api/dashboard/summary` field.
3. Tests that pin the new behavior, including a case that would have produced a
   `NaN` response body before the fix.
4. `docs/architecture.md` (§5.2, §9, §11, §13) and `docs/endpoints.md` (summary)
   descriptions of the new rule.
5. Drift B3/B4: `docs/routing.md` signature, `docs/endpoints.md` anomaly field
   list and stale `Source:` line references.
6. Drift B1/B2 under the chosen policy (user, 2026-09-27): mark
   `openspec/specs/backend_implementation.sdd.yaml` as historical/abandoned, and
   correct `docs/backend-implementation-plan.md` to the real `/api` prefix with a
   note that `/api/v1` was the original intent.
7. `odd/known-issues/tasks.md`: close C1 with the corrected facts, and fix the
   stale bookkeeping in `odd/review-backend-plan`, `odd/create-backend-sdd-spec`
   and `odd/spanish-analysis-texts`.

Out of scope (reported, not implemented):

- The `b.Mean == 0` skip at `detector.go:58`: a meter whose readings all have
  zero consumption is also silently dropped, but it is a **different** rule and
  was not authorized. Flagged for a separate decision.
- C2 (`evidenceBuilder.go:43,45` sign-unaware templates, plus the third instance
  at `llm.go:30`) and C3 (`/api/reports` raw domain shape): both remain open.
- Group A of the backlog: verified as discardable, closed by documentation only.
- The three genuinely open items inherited from `odd/review-backend-plan`
  (structured logging, OpenAPI spec, CI workflow): indexed, not implemented.

## Work units

### WU1 — Baseline guard and the unvalidated-meter signal (code + tests)

- `internal/analysis/baseline.go`: skip meters with `cnt < 2`.
- Analysis package: compute the unvalidated meters (meters present after the
  quality check whose baseline was not produced) and expose the user-visible
  reason string.
- `internal/analysis/orchestrator.go`: compute once per `Detect`, store under the
  existing mutex, expose a copying accessor.
- `internal/api/handlers/ai.go`: `DashboardSummary` gains
  `unvalidatedMeters {count, meters, reason}`; `meters` is a `[]string` that is
  never `null`.
- Tests: single-reading meter produces no baseline and no `NaN`; the meter
  produces no anomaly; the summary reports it; the summary key is present and
  well-formed on the real dataset.

### WU2 — Documentation for this change

- `docs/architecture.md` §5.2 (the minimum-readings rule), §9 (summary shape),
  §11 (the new test), §13 (the single-reading debt bullet is now closed).
- `docs/endpoints.md`: the `/api/dashboard/summary` section.
- `odd/known-issues/tasks.md`: C1 marked resolved with the corrected facts and
  the commit evidence.

### WU3 — Drift B3 and B4

- `docs/routing.md:35` signature; `docs/routing.md:74-76` health-handler location.
- `docs/endpoints.md`: the eight missing anomaly fields with their types, and the
  stale `Source:` line references.
- `docs/architecture.md` §13: the documentation bullets for the two files above
  stop being true once they are corrected.

### WU4 — Drift B1/B2 under the chosen policy

- `openspec/specs/backend_implementation.sdd.yaml`: historical/abandoned warning;
  content and `status:` field deliberately untouched.
- `openspec/config.yaml`: dead test-evidence paths replaced by real test files.
- `docs/backend-implementation-plan.md:159-163`: real `/api` prefix, corrected
  fifth-route label (anomaly id, not `meter_id`), and the original-intent note.
- `docs/architecture.md` §13: the two remaining documentation bullets and the
  group heading they would otherwise leave behind empty.

### WU5 — ODD bookkeeping

- `odd/review-backend-plan/tasks.md`, `odd/create-backend-sdd-spec/tasks.md`,
  `odd/spanish-analysis-texts/tasks.md`: bookkeeping aligned with the verified
  repository state.
- `odd/known-issues/tasks.md`: group A closed with the recovered verdicts, every
  resolved item linked to its commit, and the debts that only `docs/architecture.md`
  knew about (C4-C7) indexed so the two documents finally agree.

## Verification plan

- `go build ./...`, `go vet ./...`, `go test ./... -count=1`, `go test -race ./...`.
- `internal/analysis/requirements_dataset_test.go` is the regression net for the
  four benchmark meters and must stay green before and after: the guard does not
  fire on the shipped dataset, so no benchmark result may move.
- The new test must fail against the pre-fix code (a `NaN` `StdDev` reaching
  `json.Marshal`). Recorded as evidence below.

## Execution log

| Work unit | Commit | Checks | Evidence |
| --- | --- | --- | --- |
| WU1 | `3051333` | `go build ./...`, `go vet ./...`, `go test ./... -count=1` (8/8 packages ok) | RED reproduced against the pre-fix code in a throwaway copy with the guard deleted: `StdDev:NaN` and `GET /api/anomalies returned an empty body` |
| WU2 | `c65ab53` | every claim re-read against `git show 3051333` | §5.2/§9/§11 rewritten, §13 single-reading debt deleted, C1 closed in the backlog |
| WU3 | `75a4741` | every `Source:` reference re-read at its target line | routing.md signature and health location; endpoints.md from 10 to 18 documented fields |
| WU4 | `e713860` | spec content diffed to prove it is unchanged; `grep -rn "api/v1"` | historical warning, plan prefix corrected, §13 documentation group removed |
| WU5 | `0b99064` | `grep -rn "_pendiente_" odd/` returns nothing | group A closed, C4-C7 indexed, suggested order rewritten |

## Review and residual risk

The code change was independently verified by a separate read-only verifier, which
reproduced the pre-fix failure by deleting the guard in a copy of the repository
outside the working tree, and reported the change verified and non-blocking. Its
residual findings became items C4 and C5 of the backlog and two bullets of
`docs/architecture.md` §13: non-finite input still reaches the JSON encoders
because `strconv.ParseFloat` accepts `NaN`/`Inf`, and both handlers discard the
`json.Marshal` error, so a non-finite value answers `200` with an empty body
instead of failing loudly. Neither is fixed here.

`go test -race ./...` could NOT be run on this host: the race detector requires
cgo and no C compiler is installed (`go env CGO_ENABLED` is `0`, `gcc` is absent
from `PATH`). Race safety therefore rests on the lock discipline being a copy of
the existing `Evidence()` pattern, not on a measured result. This is the one
unmet check of the feature.

Native review: `gentle_review` inspect scoped the candidate to
`sha256:764df626a3fb5027e934107b4f99654ebb25a8ca383ab5d1f8eafa2f4cea3b6b`
(10 paths, risk `medium`, 532 changed lines) and offered an ordinary START. The
START returned a **candidate-scoped consent decline**, so no lineage was created
and no mutation was performed. The candidate therefore stands unreviewed pending a
human decision; delivery (commit is on the branch, but push, PR and merge) remains
an ordinary repository decision.
