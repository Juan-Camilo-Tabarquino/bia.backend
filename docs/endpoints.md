# API Endpoints Documentation

Reference for the real request/response contract of every registered route, as
implemented in `internal/api/router.go` and `internal/api/handlers/*.go`.

For routing, prefixes and middleware conventions see
[`docs/routing.md`](./routing.md).

## Base path: `/api` only

Every route is registered under the single `/api` prefix, and nowhere else.
Unprefixed paths such as `GET /health`, `GET /meters` or `GET /anomalies` return
`404 Not Found` (the mux default plain-text body `404 page not found`), because
only `/api/...` patterns are registered. There is no `/api/v1` alias and no
legacy unprefixed alias.

The Go 1.22 method-pattern syntax (`"POST /api/..."`) is **not** used: patterns
are plain paths, so the documented method is the intended one and the mux itself
does not reject other methods.

## Route summary

| Method | Path | Response body (top level) |
|--------|------|---------------------------|
| GET | `/api/health` | object `{"status":"ok"}` |
| GET | `/api/reports` | object `{"reports":[...]}` |
| GET | `/api/meters` | **bare array** of meter id strings |
| GET | `/api/meters/{meterId}` | object (meter metadata) |
| GET | `/api/meters/{meterId}/readings` | **bare array** of reading objects |
| GET | `/api/anomalies` | **bare array** of anomaly objects |
| GET | `/api/anomalies/{id}` | object (anomaly object) |
| POST | `/api/ai/analyze` | object `{"analysisId":"..."}` |
| GET | `/api/ai/analysis/{id}` | object (analysis result) |
| GET | `/api/dashboard/summary` | object (summary) |

All successful responses are JSON with `Content-Type: application/json`. Error
bodies from the handlers are always JSON: `{"error":"<message>"}`.

---

## GET /api/health

- Parameters: none. Request body: none.
- `200 OK` with the literal body `{"status":"ok"}` (written byte-for-byte, no
  trailing newline).

Source: `internal/api/router.go:106-109`.

## GET /api/reports

- Parameters: none; query parameters are ignored. Request body: none.
- `200 OK`, wrapper object with a single key:

```json
{
  "reports": [
    {
      "Anomaly": { "MeterID": "T-1", "Timestamp": "2026-09-01T12:00:00Z", "Delta": 0.5, "Kind": "CONSUMPTION_SPIKE", "Raw": { "MeterID": "T-1", "Timestamp": "2026-09-01T12:00:00Z", "Consumption": 1.2, "Voltage": 230, "Current": 5, "PowerFactor": 0.95, "Status": "OK" } },
      "Correlation": { "Anomaly": { "...": "same shape" }, "Events": [ { "ID": "e1", "Type": "MAINTENANCE", "Start": "...", "End": "...", "Description": "..." } ], "Explains": false },
      "Priority": 1,
      "explanation": "consumption spike of +50% vs baseline",
      "recommendation": "inspect the installation",
      "llm_text": "",
      "type": "REAL_ANOMALY",
      "severity": "HIGH",
      "confidence": 0.9,
      "status": "unexplained"
    }
  ]
}
```

Notes derived from the code:

- The elements are the domain `models.Evidence` values, **not** the anomaly DTO
  used by the anomaly endpoints, so their field names differ.
- `Anomaly`, `Correlation` and `Priority` have no JSON tags and always serialize
  with those Go field names; nested structs (`AnomalyCandidate`, `Reading`,
  `EventCorrelation`, `Event`) likewise serialize with Go field names.
- `explanation`, `recommendation`, `llm_text`, `type`, `severity`, `confidence`
  and `status` are tagged `omitempty`, so they are absent when empty/zero.
- `Timestamp`, `Start`, `End` are Go `time.Time` values (RFC3339 with
  sub-second precision when present).
- An empty evidence set produces `{"reports":[]}`.

Source: `internal/api/router.go:111-121`, `internal/domain/models/reading.go:82-95`.

## GET /api/meters

- Parameters: none. Request body: none.
- `200 OK` with a **bare JSON array of strings** — the meter identifiers, with no
  wrapper object:

```json
["T-1", "T-2"]
```

Order is map-iteration order (not sorted). With no meters loaded the body is `[]`.

Source: `internal/api/handlers/endpoints.go:22-30` (`AllMeterIDs` returns
`[]string`, `internal/data/memory/repository.go:44`).

## GET /api/meters/{meterId}

- Path parameter: `meterId` (string). Query parameters: none.
- Request body: none.
- `200 OK`, real, derivable meter metadata:

```json
{
  "id": "T-1",
  "meter_id": "T-1",
  "name": "",
  "location": "",
  "status": "OK",
  "created_at": "2026-09-01T00:00:00Z",
  "readings_count": 24,
  "last_reading_at": "2026-09-01T23:00:00Z"
}
```

- `created_at` is the earliest reading timestamp, `last_reading_at` the latest,
  both formatted as UTC RFC3339. `readings_count` is the number of readings.
- `status` is `"OK"` only when every reading of the meter has a status equal to
  `"OK"` (trimmed, case-insensitive); otherwise `"DEGRADED"`.
- `name` and `location` are always empty strings: the project has no data source
  for them, so no values are invented.
- Unknown meter: `404 Not Found` with `{"error":"meter <meterId> not found"}`.

Source: `internal/api/handlers/ai.go:32-41, 105-147`.

## GET /api/meters/{meterId}/readings

- Path parameter: `meterId` (string).
- Query parameters (both optional, RFC3339 timestamps):
  - `from` — returns only readings with `Timestamp >= from` (bound inclusive).
  - `to` — returns only readings with `Timestamp <= to` (bound inclusive).
  - A value that is not parseable by `time.RFC3339` is silently ignored; the
    request is not rejected.
- Request body: none.
- `200 OK` with a **bare JSON array** of reading objects (no wrapper):

```json
[
  {
    "MeterID": "T-1",
    "Timestamp": "2026-09-01T10:00:00Z",
    "Consumption": 1.2,
    "Voltage": 230,
    "Current": 5,
    "PowerFactor": 0.95,
    "Status": "OK"
  }
]
```

- Field names are the Go field names: only `Status` carries a JSON tag, and it is
  `omitempty` (absent when empty).
- Unknown meter, or a window with no matching readings: `200 OK` with the body
  `null` (the repository returns a nil slice). There is no 404 on this route.

Source: `internal/api/router.go:43-76`, `internal/domain/models/reading.go:9-18`.

## GET /api/anomalies

- Parameters: none. Request body: none.
- `200 OK` with a **bare JSON array** of anomaly objects, no wrapper object:

```json
[
  {
    "id": "T-1-2026-09-01T12:00:00Z",
    "meter_id": "T-1",
    "detected_at": "2026-09-01T12:00:00Z",
    "type": "REAL_ANOMALY",
    "severity": "HIGH",
    "confidence": 0.9,
    "reason": "consumption spike of +50% vs baseline",
    "recommended_action": "inspect the installation",
    "status": "unexplained"
  }
]
```

An empty evidence set produces `[]`.

Source: `internal/api/handlers/endpoints.go:67-77, 90-118, 135-141`.

## GET /api/anomalies/{id}

- Path parameter: `id` (string) — the deterministic composite id
  `<meter_id>-<detected_at>`, where `detected_at` is the anomaly timestamp as UTC
  RFC3339, e.g. `T-1-2026-09-01T12:00:00Z`. This is exactly the `id` published by
  `GET /api/anomalies`, so the list and the detail agree.
- Request body: none.
- `200 OK` with a single anomaly object (same shape as a list element).
- Unknown id: `404 Not Found` with `{"error":"anomaly <id> not found"}`.

Source: `internal/api/handlers/ai.go:149-172`, `internal/api/handlers/endpoints.go:83-85, 90-111`.

## POST /api/ai/analyze

- Parameters: none.
- Request body: **none is read.** The handler consumes no payload, so no body
  fields are defined or validated.
- Behaviour: re-runs the deterministic pipeline through the orchestrator
  (idempotent — it does not duplicate stored readings) and stores an immutable
  snapshot of the produced evidence under a new id.
- `200 OK`:

```json
{ "analysisId": "3f1c9d4e-...-uuid" }
```

- `500 Internal Server Error` if the pipeline run fails, with
  `{"error":"<error message>"}`.

Source: `internal/api/handlers/ai.go:44-56`.

## GET /api/ai/analysis/{id}

- Path parameter: `id` — the `analysisId` returned by `POST /api/ai/analyze`.
- Request body: none.
- `200 OK`, real stored result of that run:

```json
{
  "analysisId": "3f1c9d4e-...-uuid",
  "status": "completed",
  "anomalies": [
    {
      "id": "T-1-2026-09-01T12:00:00Z",
      "meter_id": "T-1",
      "detected_at": "2026-09-01T12:00:00Z",
      "type": "REAL_ANOMALY",
      "severity": "HIGH",
      "confidence": 0.9,
      "reason": "...",
      "recommended_action": "...",
      "status": "unexplained"
    }
  ]
}
```

The `anomalies` elements are the same anomaly object shape as `GET /api/anomalies`
(`[]` when the run produced none).

- Unknown id: `404 Not Found` with `{"error":"analysis <id> not found"}`.
- The store is an in-memory map in the process: ids are valid only for the
  lifetime of the running process, and are lost on restart.

Source: `internal/api/handlers/ai.go:25-30, 63-82`.

## GET /api/dashboard/summary

- Parameters: none. Request body: none.
- `200 OK`:

```json
{ "health": "ok", "meters": 2, "anomalies": 1, "lastRun": "latest" }
```

- `meters` is the current number of known meter ids, `anomalies` the number of
  evidence records produced by the last run.
- `lastRun` is the literal placeholder string `"latest"`; the code does not
  compute a timestamp for it.

Source: `internal/api/handlers/ai.go:88-97`.

---

## Anomaly fields are real pipeline values

The anomaly fields below are produced by the deterministic analysis pipeline and
carried through unchanged by the API (they are not placeholders). Their string
values are part of the public contract
(`internal/domain/models/anomaly.go:7-36`, `internal/domain/models/reading.go:82-95`):

- `type`: `REAL_ANOMALY` | `EXPLAINABLE_ANOMALY` | `FALSE_POSITIVE` | `DATA_QUALITY`.
- `severity`: `LOW` | `MEDIUM` | `HIGH`.
- `confidence`: numeric confidence of the detection.
- `reason`: the human-readable explanation of the deviation.
- `recommended_action`: the recommended operational action.
- `status`: `explained` when an operational event explains the deviation,
  otherwise `unexplained`.

`id` is derived deterministically from `meter_id` plus `detected_at`, and
`detected_at` is the UTC RFC3339 rendering of the anomaly timestamp.

## CORS

Every registered handler is wrapped by the same `corsWrapper`
(`internal/api/router.go:13-23`):

- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type, Authorization`
- An `OPTIONS` request short-circuits with `200 OK`.

The wrapper is attached per route, not installed on the mux, so an unregistered
path returns the mux default `404` **without** CORS headers. There is no other
middleware (no request-id, logging, recovery, auth or heartbeat).
