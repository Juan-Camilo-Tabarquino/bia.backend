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
| POST | `/api/ai/analyze` | object `{"analysisId":"...","meter_id":"...","status":"queued"}` (202) |
| GET | `/api/ai/analysis/{id}` | object (analysis result) |
| GET | `/api/dashboard/summary` | object (summary) |
| POST | `/api/auth/login` | object `{"token":"...","expires_at":"...","user":{...}}` |

All successful responses are JSON with `Content-Type: application/json`. Error
bodies from the handlers are always JSON: `{"error":"<message>"}`.

## Startup warm-up and on-demand LLM narration

The server runs the deterministic pipeline synchronously at startup and begins
serving roughly one second after launch. **The LLM does not run at startup.**
The narrative is produced on demand, one meter at a time, by
`POST /api/ai/analyze` and reported through `GET /api/ai/analysis/{id}`.

> **Reversal of a previous contract.** An earlier version of this document
> promised: *"The narratives fill in progressively and are available from the
> same endpoint on a later request with no client action required. No polling
> protocol, no status field and no `503` state is introduced."* That promise is
> now **deliberately reversed.** The old whole-platform enrichment started a
> background goroutine at boot, could take between 68 s and about 148 s, and
> spent four LLM calls narrating meters the user never opened. The product now
> needs a per-meter, user-triggered analysis with visible progress, so the
> contract introduces exactly a polling protocol plus `status`/`stage` fields
> and an asynchronous run. The sentence above is kept only as the record of
> what changed and why.

- The deterministic fields of an anomaly (`type`, `severity`, `confidence`,
  `reason`, `recommended_action`, `status`, `priority`, the per-meter baseline,
  the change percentages, the correlated events and the data-quality verdict)
  are final from the first request and are never produced by the LLM.
- `llm_analysis` is no longer filled in the background at boot. It is only
  populated by a completed per-meter analysis, whose result carries both that
  meter's anomalies and the whole-platform counts. Until a run for a meter
  completes, `llm_analysis` is absent (the field is `omitempty`), exactly as
  when no provider is configured.
- `GET /api/health` is unaffected and keeps answering `200 {"status":"ok"}`
  immediately, from the first moment.
- The endpoints that read the live evidence (`GET /api/anomalies`,
  `GET /api/anomalies/{id}`, `GET /api/reports` and
  `GET /api/dashboard/summary`) keep answering with the deterministic payload
  and no longer change on their own after startup.
- A run is bounded: a ~90 s watchdog and a `recover()` guard inside its
  goroutine always leave a terminal `completed` or `failed` record, so a client
  spinner can never hang. The provider itself caps each call at 60 s.

Source: `cmd/api/main.go` (runs the deterministic stage synchronously and starts
no enrichment goroutine), `internal/analysis/orchestrator.go` (`Detect`,
`RunMeter`/`EnrichMeter`), `internal/api/handlers/ai.go` (async run, watchdog,
panic guard).

---

## GET /api/health

- Parameters: none. Request body: none.
- `200 OK` with the literal body `{"status":"ok"}` (written byte-for-byte, no
  trailing newline).

Source: `internal/api/handlers/endpoints.go:15-19`.

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
      "explanation": "El consumo del medidor T-1 está 50.0% por encima de su línea base sin ningún evento operativo conocido.",
      "recommendation": "Revisar el medidor y su instalación.",
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

Source: `internal/api/router.go:106-116`, `internal/domain/models/reading.go:107-120`.

## GET /api/meters

- Parameters: none. Request body: none.
- `200 OK` with a **bare JSON array of strings** — the meter identifiers, with no
  wrapper object:

```json
["T-1", "T-2"]
```

Order is map-iteration order (not sorted). With no meters loaded the body is `[]`.

Source: `internal/api/handlers/endpoints.go:22-28` (`AllMeterIDs` returns
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
- Unknown meter: `404 Not Found` with `{"error":"medidor <meterId> no encontrado"}`.

Source: `internal/api/handlers/ai.go:136-144, 424-473`.

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

Source: `internal/api/router.go:43-75`, `internal/domain/models/reading.go:9-18`.

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
    "reason": "El consumo del medidor T-1 está 50.0% por encima de su línea base sin ningún evento operativo conocido.",
    "recommended_action": "Revisar el medidor y su instalación.",
    "status": "unexplained",
    "priority": 1,
    "baseline": {
      "mean": 0.8,
      "stddev": 0.1,
      "count": 336,
      "voltage_mean": 230,
      "current_mean": 5,
      "power_factor_mean": 0.95
    },
    "consumption_change_pct": 50,
    "voltage_change_pct": 0.4,
    "current_change_pct": 2.1,
    "power_factor_change_pct": -0.5,
    "correlated_events": [],
    "data_quality": {
      "flagged": false,
      "reason": ""
    },
    "llm_analysis": "Revisión determinista del medidor T-1: el consumo está 50.0% por encima de su línea base sin ningún evento operativo; las firmas eléctricas respaldan una anomalía real (confianza 0.90)."
  }
]
```

The object emits 18 fields (the DTO is `AnomalyDTO` and its nested types in
`internal/api/handlers/endpoints.go`):

- `id`, `meter_id`, `detected_at`, `type`, `severity`, `reason`,
  `recommended_action`, `status`: strings. `detected_at` is UTC RFC3339.
- `confidence`: number (float64), between 0 and 1.
- `priority`: integer (int), the deterministic investigation order (1 = most
  urgent), published as-is by the API and never recomputed here.
- `baseline`: object, the per-meter statistics the anomaly was compared against.
  Sub-keys: `mean`, `stddev`, `voltage_mean`, `current_mean` and
  `power_factor_mean` are numbers (float64); `count` is an integer (int).
- `consumption_change_pct`, `voltage_change_pct`, `current_change_pct` and
  `power_factor_change_pct`: numbers (float64), the signed per-signal changes
  against the baseline means, in percent.
- `correlated_events`: **array of objects**, and always an array, **never
  `null`**: it is `[]` when no operational event explains the anomaly. Each
  element has `id`, `type`, `start`, `end` and `description` (all strings;
  `start` and `end` are UTC RFC3339).
- `data_quality`: object with `flagged` (boolean) and `reason` (string).
- `llm_analysis`: string; present only when the LLM produced a narrative for
  that anomaly (see
  [Anomaly fields are real pipeline values](#anomaly-fields-are-real-pipeline-values)).

An empty evidence set produces `[]`.

Source: `internal/api/handlers/endpoints.go:36-72, 113-169, 216-220`.

## GET /api/anomalies/{id}

- Path parameter: `id` (string) — the deterministic composite id
  `<meter_id>-<detected_at>`, where `detected_at` is the anomaly timestamp as UTC
  RFC3339, e.g. `T-1-2026-09-01T12:00:00Z`. This is exactly the `id` published by
  `GET /api/anomalies`, so the list and the detail agree.
- Request body: none.
- `200 OK` with a single anomaly object (same shape as a list element).
- Unknown id: `404 Not Found` with `{"error":"anomalía <id> no encontrada"}`.

Source: `internal/api/handlers/ai.go:474-491`, `internal/api/handlers/endpoints.go:104-106, 113-169`.

## POST /api/ai/analyze

- Parameters: none.
- Request body: JSON object with the target meter:

```json
{ "meter_id": "M-109" }
```

- Behaviour: starts an **asynchronous, per-meter** analysis run. The handler
  mints an id, records the run as `queued` and answers immediately; the pipeline
  runs in a goroutine. The run executes the deterministic pipeline (global) and
  then narrates **only the requested meter's** evidence, so it costs one LLM
  call per anomaly instead of the whole-platform pass.
- `202 Accepted`:

```json
{ "analysisId": "3f1c9d4e-...-uuid", "meter_id": "M-109", "status": "queued" }
```

- If a run for the same meter is already `queued` or `running`, the response is
  also `202` but carries the **existing** `analysisId` and its current status;
  no second LLM call is started.
- `400 Bad Request` when `meter_id` is missing or names an unknown meter, with
  `{"error":"..."}`.
- Poll `GET /api/ai/analysis/{analysisId}` for progress and the final result.

Source: `internal/api/handlers/ai.go`.

## GET /api/ai/analysis/{id}

- Path parameter: `id` — the `analysisId` returned by `POST /api/ai/analyze`.
- Request body: none.
- `200 OK`, live state of that run:

```json
{
  "analysisId": "3f1c9d4e-...-uuid",
  "meter_id": "M-109",
  "status": "queued | running | completed | failed",
  "stage": "queued | lecturas | baseline | deteccion | correlacion | eventos | explicacion | recomendacion | completed | failed",
  "progress": { "done": 0, "total": 7 },
  "started_at": "2026-09-01T12:00:00Z",
  "finished_at": "2026-09-01T12:00:20Z | null",
  "anomalies": [],
  "platform": { "total_anomalies": 4, "high_priority": 2 },
  "error": null
}
```

- `status` progresses `queued` → `running` → `completed`, or ends in `failed`.
- `stage` is the current pipeline stage; `progress.done` is the number of the
  seven stages that have settled, so a finished run reports
  `{"done":7,"total":7}`. The stage order is `lecturas`, `baseline`,
  `deteccion`, `correlacion`, `eventos`, `explicacion`, `recomendacion`.
- `anomalies` is only populated when the run is `completed`, and contains
  **only the requested meter's** anomalies, mapped through the same DTO and the
  same ascending-priority ordering as `GET /api/anomalies` (`[]` when the meter
  produced none).
- `platform` is present when `completed`: `total_anomalies` is the
  whole-platform evidence count and `high_priority` the number of those items
  whose severity is `HIGH`, both produced by the same run. The frontend renders
  it as "N anomalías detectadas · M requieren atención prioritaria".
- `failed` carries a readable `error`. The ~90 s watchdog and the panic guard
  inside the goroutine guarantee the record always reaches a terminal state.
- Unknown id: `404 Not Found` with `{"error":"análisis <id> no encontrado"}`.
- The store is an in-memory map in the process: ids are valid only for the
  lifetime of the running process and are lost on restart. Terminal records are
  appended, write-only, to `data/analyses.json`; that file is never read back
  into the id space.

Source: `internal/api/handlers/ai.go`, `internal/api/handlers/analysislog.go`.

## GET /api/dashboard/summary

- Parameters: none. Request body: none.
- `200 OK`:

```json
{
  "health": "ok",
  "meters": 2,
  "anomalies": 1,
  "lastRun": "latest",
  "unvalidatedMeters": {
    "count": 0,
    "meters": [],
    "reason": "no hay suficiente información para validar: se requieren al menos 2 lecturas"
  }
}
```

- `meters` is the current number of known meter ids, `anomalies` the number of
  evidence records produced by the last run.
- `lastRun` is the literal placeholder string `"latest"`; the code does not
  compute a timestamp for it.
- `unvalidatedMeters` is always present, including when `count` is `0`, so the
  response shape is stable: `count` is the number of meters that passed the
  quality check but have no baseline because they carry fewer than 2 readings;
  `meters` is a `[]string` of their ids in ascending order that serializes as
  `[]` and **never** as `null`; `reason` is the fixed contract string shown above.
  These meters are intentionally **absent** from `/api/anomalies`: a data gap is
  not an anomaly, so no `type`, kind or severity is invented for them.

Source: `internal/api/handlers/ai.go:396-423`.

## POST /api/auth/login

Demo login for the technical-test flow. There is no database by design: the
credential store is the committed CSV `data/users.csv` (a header row plus one
user, with the password stored as a SHA-256 hex digest). It is read on every
request — one row makes the read negligible, and the credential stays editable
without a restart. The path can be overridden with the `USERS_CSV` environment
variable (default `data/users.csv`).

- Request body (JSON):

```json
{ "username": "jcamilo", "password": "bia2026" }
```

- `200 OK` — valid credentials whose `authorized` column is `true`:

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJqY2FtaWxvIiwibmFtZSI6Ikp1YW4gQ2FtaWxvIiwiYXV0aG9yaXplZCI6dHJ1ZSwiaWF0IjoxNzkwNTU0NTc3LCJleHAiOjE3OTA1ODMzNzd9.2nHHXZc5fBlNotunjHxoti1mjSUVOEFYSheofvQC31s",
  "expires_at": "2026-09-28T08:16:17Z",
  "user": { "username": "jcamilo", "name": "Juan Camilo", "authorized": true }
}
```

Ese token es un **ejemplo real capturado de la suite de tests** y está firmado con
el secreto que la suite fija (`test-only-signing-secret`), no con el fallback de
desarrollo. Su estructura, sus claims y su `exp` son los del contrato, pero **no
valida** contra `bia-demo-only-jwt-secret-do-not-use-in-production`: para
comprobar una firma, usá un token emitido por el servidor que estés corriendo.

- Errors. Every body is `{"error":"<message>"}`:

| Status | Body | When |
|--------|------|------|
| `401` | `{"error":"usuario o contraseña incorrectos"}` | unknown username **or** wrong password |
| `403` | `{"error":"el usuario no está autorizado"}` | valid credential with `authorized: false` |
| `400` | `{"error":"usuario y contraseña son obligatorios"}` | missing/empty `username` or `password`, or a body that is not valid JSON |
| `405` | `{"error":"método no permitido"}` | any method other than `POST` (`OPTIONS` is answered `200` by `corsWrapper` first) |
| `500` | the credential-store read error | `data/users.csv` is missing or malformed; a broken store is a server-side fault and never a `401` |

The `401` body is deliberately **identical** for an unknown username and a wrong
password, so the endpoint does not enumerate users. The supplied password is
hashed before the lookup and the two hex digests are compared in constant time.

### The token

- Algorithm: `HS256` over a compact JWS,
  `base64url(header).base64url(claims).base64url(HMAC-SHA256(...))`, built with
  the standard library only (`crypto/hmac`, `crypto/sha256`, `encoding/base64`,
  `encoding/json`). No third-party JWT package is used.
- Header: `{"alg":"HS256","typ":"JWT"}`.
- Claims: `sub` (username), `name`, `authorized` (boolean), `iat` and `exp` in
  Unix seconds. Lifetime: **8 hours** (`exp - iat = 28800`).
- `expires_at` is that same expiry rendered as RFC3339.
- Secret: `JWT_SECRET`. When the variable is unset the handler falls back to a
  documented development secret, so the demo needs no configuration at all; a
  real deployment must set `JWT_SECRET`. Neither the secret nor the token is
  logged, and the secret never appears in a response or an error message.

The token is **issued only**: no other route validates it. That is a deliberate
scope decision for this login-flow demo, not a missing middleware — see
[`docs/architecture.md`](./architecture.md) §9. Because nothing validates the
token, a frontend guard is UX, not a security boundary.

Source: `internal/api/handlers/auth.go`, `internal/api/router.go`, `data/users.csv`.

---

## Anomaly fields are real pipeline values

The anomaly fields below are produced by the deterministic analysis pipeline and
carried through unchanged by the API (they are not placeholders). Their string
values are part of the public contract
(`internal/domain/models/anomaly.go:7-40`, `internal/domain/models/reading.go:107-120`):

- `type`: `REAL_ANOMALY` | `EXPLAINABLE_ANOMALY` | `FALSE_POSITIVE` | `DATA_QUALITY`.
- `severity`: `LOW` | `MEDIUM` | `HIGH`.
- `confidence`: numeric confidence of the detection.
- `reason`: the human-readable explanation of the deviation, from the
  deterministic pipeline (`models.Evidence.Explanation`). It is always present
  for an anomaly and is the deterministic source of truth.
- `recommended_action`: the recommended operational action, from the
  deterministic pipeline (`models.Evidence.Recommendation`). It is always
  present for an anomaly and is the deterministic source of truth.
- `status`: `explained` when an operational event explains the deviation,
  otherwise `unexplained`.
- `llm_analysis`: the LLM-generated interpretation of the anomaly
  (`models.Evidence.LLMText`). It is an **additional** narrative, never a
  replacement: `type`, `severity`, `confidence`, `reason` and
  `recommended_action` always come from the deterministic pipeline.
  `llm_analysis` may be **absent/empty** when no LLM provider is configured or
  when the provider call failed; because the field is `omitempty`, an empty
  narrative is omitted from the JSON entirely and the key does not appear.

`id` is derived deterministically from `meter_id` plus `detected_at`, and
`detected_at` is the UTC RFC3339 rendering of the anomaly timestamp.

## CORS

Every registered handler is wrapped by the same `corsWrapper`
(`internal/api/router.go:13-24`):

- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type, Authorization`
- An `OPTIONS` request short-circuits with `200 OK`.

The wrapper is attached per route, not installed on the mux, so an unregistered
path returns the mux default `404` **without** CORS headers. There is no other
middleware (no request-id, logging, recovery, auth or heartbeat).
