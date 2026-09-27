# Feature: spanish-analysis-texts

**Branch:** `feat/spanish-analysis-texts`
**Base:** `ffaca34` (tip of `fix/non-blocking-startup`)
**Created:** 2026-09-27

## Problem

The analysis surface shows English text to the user. The investigation established
that the English the user sees does **not** come from the LLM:

| Field | Origin | Language |
| --- | --- | --- |
| `reason` | `internal/analysis/evidenceBuilder.go:39-49` (Go literals) | English prose |
| `recommended_action` | `internal/analysis/evidenceBuilder.go:71-79` (Go literals) | English prose |
| `data_quality.reason` | `internal/analysis/detector.go:166,169` (Go literals) | English prose |
| `correlated_events[].description` | `data/events.csv:2-5` (dataset) | English data |
| `llm_analysis` | LLM, `internal/ai/provider.go` | **already Spanish** |
| API error bodies | `handlers/ai.go`, `handlers/endpoints.go` | English |

The Ollama prompt (`internal/ai/provider.go:32-44`) is written entirely in Spanish
but contains **no explicit instruction about the response language**, and the chat
request sends no `system` message, `temperature` or `format`. The Spanish
`llm_analysis` is therefore the model mirroring the prompt's language, not a
guarantee of the code.

## Decision (user, 2026-09-27)

Translate everything user-visible to Spanish **in the backend**. The user chose
this option over "only harden the LLM prompt" precisely because the prompt cannot
affect the deterministic Go prose that the user is actually seeing.

## Scope

In scope:

1. Deterministic analysis prose: `internal/analysis/evidenceBuilder.go`
   (format strings at `:39,41,43,45,47,49`, the `"no event recorded"` literal at
   `:55`, the event join format at `:60`, and the recommendations at
   `:71,73,75,77,79`).
2. Detector data-quality prose: `internal/analysis/detector.go:166,169`
   (feeds `data_quality.reason`).
3. Deterministic mock narrative: `internal/analysis/llm.go:30,33,36,39,42`
   (visible whenever `LLM_API_KEY` is empty, i.e. offline/demo runs).
4. API error bodies surfaced to the frontend:
   `internal/api/handlers/ai.go:83,84,170` and
   `internal/api/handlers/endpoints.go:207`. The frontend renders
   `error.message` through `getErrorMessage` (observed in
   `AiReanalysis.tsx`), so these ARE user-visible.
   **Trap to avoid:** `internal/api/handlers/ai.go:80` is NOT an error body. It is
   the `"completed"` status token and it is a non-goal; it must stay
   byte-identical. This document originally listed that line in error scope by
   mistake, corrected 2026-09-27 before WU2 was delegated.
5. An explicit response-language instruction in the prompt
   (`internal/ai/provider.go:32-44`), turning incidental Spanish into a contract,
   plus a test asserting the instruction is present.
6. Dataset descriptions: `data/events.csv:2-5`. These reach the user twice: as
   `correlated_events[].description` and embedded inside the deterministic
   `reason` via the event join at `evidenceBuilder.go:60`.
7. `docs/endpoints.md` example values, so the documented examples match what the
   API now returns.

## Non-goals

- **Never translate the contract tokens.** `type` (`REAL_ANOMALY`,
  `EXPLAINABLE_ANOMALY`, `FALSE_POSITIVE`, `DATA_QUALITY`), `severity` (`LOW`,
  `MEDIUM`, `HIGH`), `status` (`explained`, `unexplained`) and the
  `"completed"` token in `analysisResultDTO` are machine values the frontend maps
  to its own labels. `docs/endpoints.md` documents them as real pipeline values.
- No change to any response shape, status code, route or JSON key. Only the
  string contents change.
- No change to identifiers, timestamp formats or numeric fields.
- No frontend work (different repository).
- No restructure of `evidenceBuilder.go`; keep the same formatted-string
  approach, only the words change.

## Checks

- **TDD mode:** strict RED -> GREEN -> REFACTOR
- **TDD source:** the user's explicit choice earlier in this same session
  (recorded for `async-llm-enrichment`), carried forward. No project or session
  configuration file states a different mode, and no conflicting source exists.
  If the user wants a different mode for this feature, it overrides this.
- **Runner:** `go test ./...`
- **Race detector:** available on this machine (verified in this session).

Strict TDD fits this feature naturally: the RED is a test asserting the Spanish
text, observed failing against the English literals, then the literals change and
it passes.

## Delivery strategy

`single-pr`, carried from the user's explicit choice in this session. Forecast
authored changed lines: the literals are few but the tests that assert them must
move with them.

| Area | Estimate |
| --- | --- |
| `internal/analysis/evidenceBuilder.go` | ~30 |
| `internal/analysis/detector.go` | ~10 |
| `internal/analysis/llm.go` | ~15 |
| API error bodies (2 files) | ~15 |
| `internal/ai/provider.go` (prompt + test) | ~30 |
| `data/events.csv` | ~10 |
| `docs/endpoints.md` | ~30 |
| Test updates across packages | ~80 |
| **Total** | **~220** |

Under the ~400 budget, so no chain strategy is needed. Running count updated
after each work-unit commit.

## Route declaration

| Work unit | Route | Trigger evidence |
| --- | --- | --- |
| WU1 analysis prose | delegated (`gentle-ai-worker`) | Writer trigger: 3+ files with tests |
| WU2 API errors | delegated (same writer thread) | Writer trigger: 2 files with tests |
| WU3 prompt instruction | delegated (same writer thread) | Preparation trigger: prompt + its test |
| WU4 dataset | parent inline OR delegated | Single data file; decide when reached |
| WU5 docs | delegated (same writer thread) | Preparation trigger: large doc |
| WU6 verification | delegated (`gentle-ai-verify`) | Verification rule |

One writer thread only; no parallel writers in this worktree.

## Tasks

- [ ] **WU1 — Spanish for the deterministic analysis prose.** Translate the
      literals in `evidenceBuilder.go` and `detector.go`. RED first: a test that
      asserts the Spanish output and fails against the English literals. Update
      every existing test that asserts the old English text. Commit.
- [ ] **WU2 — Spanish for the API error bodies.** Translate the error strings in
      `handlers/ai.go` (the not-found messages) and `handlers/endpoints.go`,
      with tests asserting the new Spanish bodies and the same status codes.
      Do NOT touch the `"completed"` token in `analysisResultDTO`. Commit.
- [ ] **WU3 — Explicit response-language instruction.** Add the instruction to
      `explanationPromptTemplate` and a test asserting the prompt carries it.
      Commit.
- [ ] **WU4 — Spanish dataset descriptions.** Translate the four `description`
      values in `data/events.csv`, keeping the CSV schema and the event types
      untouched. Confirm nothing outside the dataset depends on the exact English
      strings. Commit.
- [ ] **WU5 — Align the documented examples.** Update the `reason` and
      `recommended_action` example values in `docs/endpoints.md` so the doc shows
      what the API now returns. Commit.
- [ ] **WU6 — Closure verification.** `go test ./...`, `go test -race ./...`, and
      a live run confirming that `/api/anomalies` returns Spanish `reason`,
      `recommended_action`, `data_quality.reason` and
      `correlated_events[].description`, that `llm_analysis` is Spanish, and that
      the contract tokens are still the English machine values. Evidence only.

## Acceptance evidence

- No English prose remains in any user-visible field of the anomaly/analysis
  surface.
- `type`, `severity`, `status` and `"completed"` are byte-identical to before.
- Full suite and race detector green.
- The prompt contains an explicit Spanish instruction and a test enforces it.
- A live request shows Spanish text end to end.

## Follow-up out of scope

- The wider documentation drift found earlier (`openspec/` naming chi and
  `/api/v1`, `docs/routing.md:35` stale signature, `docs/endpoints.md`
  under-documenting the anomaly DTO) remains a separate feature.
- The frontend's own labels for the machine tokens are that repository's concern.
