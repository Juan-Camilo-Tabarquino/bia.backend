# review-backend-plan

## Findings and Improvement Tasks

- [ ] Add explicit Go version requirement and module tidy steps in documentation.
- [ ] Define a structured logging framework (e.g., zerolog) and integrate across all layers.
- [ ] Externalize CSV file paths and LLM API keys to environment variables with a config loader.
- [ ] Write unit tests for CSV parsing edge cases (invalid timestamps, missing columns).
- [ ] Implement a mock LLM client for deterministic testing of the AI integration layer.
- [ ] Choose a concrete HTTP router (chi recommended) and document routing setup.
- [ ] Generate an OpenAPI (Swagger) specification for the REST API.
- [ ] Add CSV schema validation with clear error messages before ingest.
- [ ] Create a CI workflow (GitHub Actions) to run `go test`, lint, and build.
- [ ] Document fallback behavior when LLM API key is absent (use mock client).

Each task should be tracked in the ODD `todo` list for subagent execution.
