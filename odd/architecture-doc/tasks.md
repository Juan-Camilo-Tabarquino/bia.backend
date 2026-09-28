# Feature: architecture-doc

**Branch:** `main` (direct, no feature branch — user request: document the default branch)
**Base:** `e6536c7` (main after both PRs merged)
**Created:** 2026-09-27

## Goal

A complete architecture document for the repository: how the backend works, how each
analysis stage works, how the LLM is wired, and whatever else is needed to present
the project to a reader. Requested by the user with `main` as the target branch.

## Facts that shape the deliverable

- `main` was fast-forwarded to `e6536c7` after the user merged PR #2
  (`fix/non-blocking-startup`) and PR #3 (`feat/spanish-analysis-texts`) on GitHub.
  The document therefore describes the FINAL state: non-blocking startup, the
  generation guard, the Spanish copy and the explicit prompt language instruction.
- The whole suite runs green on the merged `main`, verified before writing.

## Decisions (user, 2026-09-27)

- **Language: Spanish.** An explicit override of the English default for
  repository-facing docs. Supporting reasons: the document exists to present the
  project to Spanish-speaking readers, the API's user-visible copy is now Spanish,
  and `odd/*/tasks.md` in this same repository are already written in Spanish.
  Identifiers, file paths, JSON keys and contract tokens stay English inside it.
- **Location: `docs/architecture.md`**, plus a two-line pointer in `README.md` so
  the document is actually discoverable from the repository root.

## Route

| Step | Route | Trigger evidence |
| --- | --- | --- |
| Algorithm extraction | delegated (`gentle-ai-explore`) | Mapping trigger: 6+ analysis files plus the LLM, API and config |
| Authoring the document | parent inline | The parent holds the session context needed to synthesise; a writer would have to be told everything already learned |
| README pointer | parent inline | Two lines, mechanical |
| Verification | delegated (`gentle-ai-verify`) | Verification rule |

One writer thread; the parent is the only writer here.

## Checks

- No TDD: a documentation artifact has no executable behaviour, so strict
  RED->GREEN cannot apply. Verification is a factual readback against the source
  plus a link and format check. This is stated so the absence of tests is a
  decision, not an oversight.
- Runner: `go test ./...` still run, to prove the document's arrival changed no
  behaviour.

## Tasks

- [x] **AD1 — Extract the exact algorithms.** DONE. Delegated read-only extraction
      of every stage with literals and `file:line`. Operational note: the scout
      FINISHED (20 turns, 68 tool calls) but its completion message never reached
      the parent, so the parent briefly and wrongly reported it as still running;
      the result was recovered with `subagent_result`. Worth remembering as a
      harness delivery failure mode.
- [x] **AD2 — Write `docs/architecture.md`.** DONE. 13 sections, 746 lines.
- [x] **AD3 — Add the README pointer.** DONE. Three lines with two links under the
      title, including a note that the document is in Spanish.
- [x] **AD4 — Verify.** DONE. Independent verifier returned
      **`partially-verified`** and was genuinely useful: the entire numeric and
      structural core passed (all ten threshold/formula claims MATCH, the prompt
      quoted in the document is byte-identical to the source, and the verifier ran
      the dataset regression test and reproduced all four benchmark outcomes), but
      it found **five real errors**, all in the soft descriptive parts:

      | # | Error | Truth |
      | --- | --- | --- |
      | A | claimed `config.yaml` may live in the parent directory | `SetConfigFile` bypasses Viper's search paths, so only the working directory is read |
      | B | attributed `/api/v1` to `openspec/` | that token lives in `docs/backend-implementation-plan.md`, never in openspec |
      | C | described `docs/` as English | `requerimientos.md` and this document are Spanish |
      | D | startup arrow showed `ListenAndServe` before `Enrich` | the goroutine is launched before it; §8 already had it right |
      | E | two cross-references pointed at §11 | the referenced material is in §13 |

      All five were corrected. A sixth defect was introduced by the correction
      itself (a duplicated clause) and was caught and fixed before committing.

      The verifier could not confirm the timing claims (no server was started),
      the historical 69-second figure, or the race detector's functionality; those
      remain unverified in the document rather than asserted as fact.

## Acceptance evidence

- Every numeric threshold and formula in the document matches the source, verified
  against `file:line`.
- No invented behaviour and no stale claim: the document describes `e6536c7`, not
  an earlier state.
- The README links to the document, and the link resolves.
- `go test ./...` still green.

## Non-goals

- No code change of any kind.
- No translation of the existing English docs (`README.md` body,
  `docs/endpoints.md`, `docs/routing.md`, `docs/backend-implementation-plan.md`).
- No fix of the previously identified documentation drift (`openspec/` naming chi
  and `/api/v1`, `docs/routing.md:35` stale signature, `docs/endpoints.md`
  under-documenting the anomaly DTO). Still a separate feature.
  **Corrección posterior:** ese drift se arregló después (commits `e713860` y
  `75a4741`); la lista se conserva como registro.
