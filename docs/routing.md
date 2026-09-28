# HTTP Routing Conventions

This service uses the Go standard library router, `net/http.ServeMux`. It does
**not** use `chi` or any other third-party router, and every route is registered
explicitly in `internal/api/router.go`.

For the request parameters and response bodies of each endpoint, see
[`docs/endpoints.md`](./endpoints.md). That file is the authoritative
per-endpoint reference; this document only covers routing, prefixes and
middleware.

---

## Base path: a single `/api` prefix

Every route is registered under the single `/api` prefix, and nowhere else. This
is an intentional contract decision documented on `NewRouter`:

- There is **no** `/api/v1`. The project deliberately keeps one prefix so
  clients and the UI track a single base path.
- Unprefixed paths are **not** served. `GET /health`, `GET /meters` and
  `GET /anomalies` all return `404 Not Found`, because the mux only knows the
  `/api/...` patterns.

Do not add a second prefix or an unprefixed alias for an existing route.

## Router construction

`api.NewRouter` builds a single `*http.ServeMux`, wraps every handler with the
CORS wrapper, and registers it under `/api`. The mux is never `StripPrefix`-ed,
so handlers that own sub-paths parse the path variable themselves by trimming
the leading `/api` and their own prefix from `r.URL.Path`.

```go
func NewRouter(orchestrator *analysis.Orchestrator) http.Handler {
    mux := http.NewServeMux()

    mux.Handle("/api/health", corsWrapper(http.HandlerFunc(handlers.Health)))
    mux.Handle("/api/reports", corsWrapper(reportsHandler(orchestrator)))
    mux.Handle("/api/meters", corsWrapper(handlers.Meters(orchestrator.ReadingRepo)))
    mux.Handle("/api/meters/", customMeters)                 // detail + readings subtree
    mux.Handle("/api/anomalies", corsWrapper(handlers.Anomalies(orchestrator)))
    mux.Handle("/api/anomalies/", corsWrapper(handlers.AnomalyDetailByID(orchestrator)))
    mux.Handle("/api/ai/analyze", corsWrapper(handlers.AnalyzePOST(orchestrator)))
    mux.Handle("/api/ai/analysis/", corsWrapper(http.HandlerFunc(handlers.AnalysisGET)))
    mux.Handle("/api/dashboard/summary", corsWrapper(handlers.DashboardSummary(orchestrator)))
    mux.Handle("/api/auth/login", corsWrapper(http.HandlerFunc(handlers.AuthLogin)))

    return mux
}
```

The actual mux patterns are literal strings (`/api/meters/`, `/api/anomalies/`,
`/api/ai/analysis/`). The `{...}` segments in the table below are the logical
path variables each handler extracts from `r.URL.Path`.

## Registered routes

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/health` | Health check. |
| GET | `/api/reports` | Evidence/report objects produced by the pipeline. |
| GET | `/api/meters` | List of known meter identifiers. |
| GET | `/api/meters/{meterId}` | Metadata for a single meter. |
| GET | `/api/meters/{meterId}/readings` | Readings for a meter, optionally filtered by `from`/`to`. |
| GET | `/api/anomalies` | List of anomaly evidence. |
| GET | `/api/anomalies/{id}` | Detail for one anomaly id. |
| POST | `/api/ai/analyze` | Re-run the pipeline and return a new analysis id. |
| GET | `/api/ai/analysis/{id}` | Result of a previous analysis run. |
| GET | `/api/dashboard/summary` | High-level counts for the dashboard. |
| POST | `/api/auth/login` | Demo login: verifies the committed `data/users.csv` store and issues an HS256 JWT. It only issues the token; no route validates it. |

See `docs/endpoints.md` for the parameters and JSON body of each route.

## Health check

`GET /api/health` returns `200 OK` with the literal body `{"status":"ok"}`. It is
the `handlers.Health` function in `internal/api/handlers/endpoints.go`, wrapped by
`corsWrapper` and registered in `internal/api/router.go`; there is no heartbeat
middleware.

## Middleware

The only middleware is the CORS wrapper, applied per registered handler:

```go
func corsWrapper(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusOK)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

- Allowed methods: `GET`, `POST`, `OPTIONS`; an `OPTIONS` request short-circuits
  with `200 OK`.
- Allowed headers: `Content-Type`, `Authorization`.
- The wrapper is attached per route, not installed on the mux, so an
  unregistered path returns the mux default `404` without CORS headers.
- There is no request-id, logging, recovery, heartbeat or auth middleware. Do not
  document or add those.

## Adding a new route

1. Write the handler in `internal/api/handlers`.
2. Register its `/api/...` pattern on the mux in `NewRouter`.
3. Wrap it with `corsWrapper`, like every existing route.

```go
// internal/api/router.go
mux.Handle("/api/users", corsWrapper(handlers.Users(orchestrator)))

// Sub-paths use a trailing-slash pattern; the handler trims "/api" and then its
// own sub-path from r.URL.Path to recover the path variable.
mux.Handle("/api/users/", corsWrapper(handlers.UserDetail(orchestrator)))
```

Keep the single `/api` prefix and the per-handler CORS wrapper consistent with
the routes already registered.
