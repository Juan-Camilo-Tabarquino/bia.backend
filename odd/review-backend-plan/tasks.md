# review-backend-plan

## Status

This file was **never executed as its own feature**. It is a list of past reviewer
findings converted into improvement tasks, prepared on its own but never run as a
work-unit sequence. Re-verified against the current repository (2026-09-27), its
ten items are now either satisfied elsewhere, superseded, or genuinely open. The
three open items are also indexed in `odd/known-issues/tasks.md`; the rest are
closed below with the evidence.

## Findings and Improvement Tasks

- [x] Add explicit Go version requirement and module tidy steps in documentation.
      Satisfied in `README.md`: `- **Go version:** 1.22 or newer` (line 10) and
      the `go mod tidy` setup step (line 17).
- [ ] **OPEN** — Define a structured logging framework (e.g., zerolog) and integrate across all layers.
      Still missing: the code uses the standard `log` package, and the former
      `internal/logger` (zerolog) was removed in `repo-hygiene`.
- [x] Externalize CSV file paths and LLM API keys to environment variables with a config loader.
      Satisfied in `internal/config/config.go`: `Load()` binds `READINGS_CSV`,
      `EVENTS_CSV`, `LLM_API_KEY`, `LLM_BASE_URL` and `LLM_MODEL` via `BindEnv`.
- [x] Write unit tests for CSV parsing edge cases (invalid timestamps, missing columns).
      Satisfied in `internal/data/csv/loader_test.go`:
      `TestLoadReadingsInvalidTimestamp`, `TestLoadReadingsMissingColumns`,
      `TestLoadReadingsMalformedNumeric`, `TestLoadEventsInvalidTimestamp`,
      `TestLoadEventsMissingColumns`.
- [x] Implement a mock LLM client for deterministic testing of the AI integration layer.
      Satisfied: `NewMockLLM` in `internal/analysis/llm.go`, selected as the
      no-key fallback by `NewProvider` in `internal/ai/provider.go`.
- [x] Choose a concrete HTTP router (chi recommended) and document routing setup.
      Satisfied differently (superseded): the project deliberately uses the
      standard library router, `net/http.ServeMux`, and documents that choice in
      `docs/routing.md:3-4`. No chi dependency exists; the recommendation was not
      adopted.
- [ ] **OPEN** — Generate an OpenAPI (Swagger) specification for the REST API.
      Still missing: no OpenAPI/Swagger artifact exists in the repository; the
      HTTP contract is hand-written in `docs/endpoints.md`.
- [x] Add CSV schema validation with clear error messages before ingest.
      Satisfied in `internal/data/csv/loader.go`: missing or malformed columns
      return errors such as `missing required column '%s' in readings CSV`,
      `missing required column '%s' in readings row %d` and
      `invalid timestamp in readings row %d`.
- [ ] **OPEN** — Create a CI workflow (GitHub Actions) to run `go test`, lint, and build.
      Still missing: no `.github/` directory and no workflow file exists, so none
      of those steps run automatically.
- [x] Document fallback behavior when LLM API key is absent (use mock client).
      Satisfied in `docs/architecture.md` ("Sin `LLM_API_KEY` el backend funciona
      igual: usa un proveedor determinístico local", line 69) and its §7.

Each task should be tracked in the ODD `todo` list for subagent execution.
