# Cumplimiento de `docs/requerimientos.md` — Plan de tareas

**Fuente autoritativa:** `docs/requerimientos.md` (prueba técnica AI Energy Management Platform).
**Documentos secundarios:** `docs/endpoints.md`, `docs/routing.md`, `docs/backend-implementation-plan.md`, `data/readings.csv`, `data/events.csv`.
**Estado:** auditoría completada (solo lectura). Implementación pendiente de autorización.
**Evidencia base:** `internal/analysis/evidenceBuilder.go:11`, `internal/analysis/orchestrator.go` (clasificación descartada), `internal/analysis/scorer.go`, `internal/analysis/quality.go:27`, `internal/analysis/classifier.go`, `internal/domain/models/reading.go`, `internal/api/handlers/ai.go`, `internal/api/*_test.go`, `docs/routing.md`.

---

## Causa raíz (bloquea toda la evaluación de IA)

`EvidenceBuilder.Build(...)` devuelve siempre un slice vacío y `orchestrator.Run()` descarta la salida del classifier (`_ = o.Classifier.Classify(correlated)`). Por lo tanto `orchestrator.Evidence()` queda vacío: `/api/anomalies` y `/api/anomalies/{id}` no devuelven nada y ningún criterio de IA puede cumplirse.

## Criterios de aceptación de la prueba (a los que apuntan las tareas)

| Criterio | Puntos | Tareas que lo cubren |
|---|---|---|
| Detecta `M-109` | 30 | T1, T2, T5, T6 |
| Prioriza `M-109` | 25 | T3, T4, T5 |
| Evita tratar `M-106` como anomalía real | 15 | T4, T5, T6 |
| Detecta `M-112` como problema de calidad | 10 | T7, T8 |
| Explica con evidencia | 10 | T1, T2, T9 |
| Recomienda acción coherente | 10 | T1, T2, T9 |
| Backend / API (20 pts) | 20 | T10–T14 |
| Testing / documentación / calidad (10 pts) | 10 | T15–T20 |

**Esfuerzo:** S ≈ ≤1 día · M ≈ 1–3 días · L ≈ ≥3 días.

---

## Tareas

### A. Pipeline de IA / análisis (ruta crítica)

| ID | Título | Detalle | Archivos | Esfuerzo |
|----|--------|---------|----------|----------|
| **T1** | Construir `Evidence` real en el builder | Combinar `AnomalyCandidate` + correlación + clasificación en objetos `models.Evidence` (hoy retorna `[]models.Evidence{}`). | `internal/analysis/evidenceBuilder.go` | M |
| **T2** | Consumir la salida del classifier en el orquestador | Reemplazar `_ = o.Classifier.Classify(correlated)` y alimentar el pipeline con esa evidencia; garantizar que `Evidence()` no quede vacío. | `internal/analysis/orchestrator.go` | S |
| **T3** | Implementar el scorer real | Reemplazar `identityScorer` por cálculo de `severity`, `confidence` y `priority`. | `internal/analysis/scorer.go` | M |
| **T4** | Corregir la lógica de prioridad del classifier | Hoy asigna prioridad 1 cuando hay evento correlacionado; un evento correlacionado es `EXPLAINABLE` (no alta prioridad). | `internal/analysis/classifier.go` | S |
| **T5** | Definir enums de clasificación | `REAL_ANOMALY`, `EXPLAINABLE_ANOMALY`, `FALSE_POSITIVE`, `DATA_QUALITY` (no existen en el código). | nuevo `internal/domain/models/anomaly.go` | M |
| **T6** | `confidence > 0.90` para `M-109` | Asegurar que el caso `M-109` (>100% sin evento, con cambios eléctricos) obtenga `REAL_ANOMALY / HIGH / confidence > 0.90`. | `internal/analysis/scorer.go`, `classifier.go` | M |
| **T7** | Detectar `M-112` (calidad de datos) | El detector solo mira `consumption` (media+3σ o >45%); hay que detectar consumo estable con voltaje/corriente/factor de potencia inconsistentes. | `internal/analysis/detector.go` | M |
| **T8** | Filtrado real en Data Quality | `qualityChecker.Check` devuelve el input sin tocar; implementar reglas (consumo negativo, `status != OK`, inconsistencias eléctricas). | `internal/analysis/quality.go` | M |
| **T9** | LLM cableado sobre evidencia real y persistido | Invocar el LLM por evidencia y guardar `explanation` / `recommendation` para que viajen en el JSON. | `internal/analysis/orchestrator.go`, `internal/analysis/llm.go` | S |

### B. Modelo de datos

| ID | Título | Detalle | Archivos | Esfuerzo |
|----|--------|---------|----------|----------|
| **T10** | Struct `Meter` | `id, meter_id, name, location, status, created_at` (hoy solo existe el alias `MeterID`). | `internal/domain/models/` | M |
| **T11** | Campos nuevos en `Evidence` | Agregar `Type`, `Severity`, `Confidence`, `Status` (el spec los exige en el JSON de la IA). | `internal/domain/models/reading.go` | M |
| **T12** | `status` en `Reading` y en el loader | El dataset trae la columna `status`; el loader la ignora y `Reading` no la tiene. | `internal/domain/models/reading.go`, `internal/data/csv/loader.go` | S |

### C. Endpoints / API

| ID | Título | Detalle | Archivos | Esfuerzo |
|----|--------|---------|----------|----------|
| **T13** | Detalle de anomalía con campos reales | Hoy devuelve `severity: "medium"` y `confidence: 0.9` hardcodeados; deben venir de la evidencia. | `internal/api/handlers/ai.go` | S |
| **T14** | `POST /api/ai/analyze` ejecuta el pipeline | Hoy solo guarda un UUID; debe re-ejecutar el análisis y permitir consultar el resultado. | `internal/api/handlers/ai.go` | S |
| **T15** | Metadatos en `/api/meters/{id}` | Hoy solo hace eco del ID; debe devolver la entidad del medidor (depende de T10). | `internal/api/handlers/ai.go`, `internal/api/router.go` | S |
| **T16** | Unificar versión del prefijo | El código usa `/api`; `docs/routing.md` pide `/api/v1`. Decidir y unificar. | `internal/api/router.go` | S |

### D. Testing

| ID | Título | Detalle | Archivos | Esfuerzo |
|----|--------|---------|----------|----------|
| **T17** | Tests HTTP de todos los endpoints | Hoy solo `/api/health` está testeado (`api_test.go:70`, `handlers/handlers_test.go:11`). Cubrir meters, meter detail, readings, anomalies list/detail, ai/analyze, ai/analysis, dashboard. | `internal/api/*_test.go` | M |
| **T18** | Tests de casos reales del dataset | `M-104 EXPLAINABLE/MEDIUM`, `M-106 FALSE_POSITIVE/LOW`, `M-109 REAL_ANOMALY/HIGH (>0.90)`, `M-112 DATA_QUALITY/HIGH`. | nuevo test del pipeline | M |

### E. Docs / configuración

| ID | Título | Detalle | Archivos | Esfuerzo |
|----|--------|---------|----------|----------|
| **T19** | Actualizar `docs/routing.md` | Describe chi, `/api/v1` y health en `/health`; el código real usa `net/http.ServeMux`, `/api` y `/api/health`. Chi ya fue eliminado. | `docs/routing.md` | S |
| **T20** | Factory de LLM por config | Usar el provider real si existe `LLM_API_KEY`; caer al mock si no. | `internal/ai/provider.go`, `internal/config/config.go`, `internal/analysis/llm.go` | S |

---

## Dependencias

- **T1 → T2 → T3 → T6** es la cadena crítica que desbloquea los criterios de IA.
- **T5** habilita T4 y T6 (tipos de anomalía).
- **T10** habilita T15.
- **T12** habilita T8 (calidad sobre `status`).
- **T1–T9** deben cerrarse antes de T18 (los tests de casos reales validan el pipeline).

## Verificación

- `go test ./...` en verde y cobertura global ≥ 80%.
- Los 4 casos del dataset se comportan como indica la tabla del §4 de `docs/requerimientos.md`.
- Cada tarea cierra con un commit work-unit (Conventional Commit) en la rama de la feature, con tests y docs junto al comportamiento.

> Nota: este documento es de planificación. La implementación no se inicia hasta autorización explícita del usuario.

---

## Estado de ejecución — COMPLETADO (20/20)

Implementado por subagentes `gentle-ai-worker` (secuenciales, un writer a la vez por solapamiento de archivos) y verificado de forma independiente por `gentle-ai-verify`.

### Resultado verificado

- `go build ./...` → OK.
- `go test -count=1 ./...` → **suite completa verde** (analysis, api, api/handlers, config, data/csv, data/memory, domain/services, ai).
- Casos del dataset (§4 de los requerimientos), medidos por `internal/analysis/requirements_dataset_test.go`:

| Medidor | Kind | Type | Severity | Confidence | Status | Priority |
|---|---|---|---|---|---|---|
| M-104 | CONSUMPTION_SPIKE | EXPLAINABLE_ANOMALY | MEDIUM | 0.8297 | explained | 3 |
| M-106 | CONSUMPTION_DROP | FALSE_POSITIVE | LOW | 0.7932 | explained | 4 |
| **M-109** | CONSUMPTION_SPIKE | **REAL_ANOMALY** | **HIGH** | **0.9700** | unexplained | **1** |
| M-112 | DATA_QUALITY | DATA_QUALITY | HIGH | 0.9000 | explained | 2 |

- Cuerpo real de `GET /api/anomalies/M-109-...`:
  `{"id":"M-109-2026-09-14T13:00:00Z","meter_id":"M-109","detected_at":"...","type":"REAL_ANOMALY","severity":"HIGH","confidence":0.97,"reason":"Meter M-109 consumption is 125.3% above its baseline with no known operational event.","recommended_action":"Investigate the meter and its installation.","status":"unexplained"}`

### Trampas que hubo que resolver

1. **`evidenceBuilder.Build` devolvía `[]`** y el orquestador descartaba la salida del classifier: era la causa raíz de las anomalías vacías.
2. **El evento `UNKNOWN` de M-109 explicaba la anomalía**: el correlador marcaba `Explains=true` ante cualquier evento solapado, lo que habría impedido que M-109 fuera `REAL_ANOMALY`. Ahora sólo explican `SCHEDULED_OUTAGE`/`MAINTENANCE`/`SHUTDOWN`/`OPERATIONAL_CHANGE`/`PRODUCTION_LINE`/`DATA_QUALITY`.
3. **M-112 era invisible al detector de consumo** (consumo estable): se agregó un candidato `DATA_QUALITY` por voltaje fuera de 209..231 V o PF < 0.85 con consumo cerca de la mediana horaria.
4. **`orchestrator.Run()` no era idempotente**: `AddMany` acumulaba, así que re-ejecutar el pipeline duplicaba las 4032 lecturas. Ahora la carga se hace una sola vez bajo lock.

### Intervención humana requerida (resuelta)

El único bloqueo que necesitó decisión del usuario fue el contrato del loader: al hacer opcional `status`, `TestLoadReadingsMissingColumns` quedaba inválido. **Se aprobó la Opción 1**: reescribir ese test para omitir una columna requerida y agregar `TestLoadReadingsRealDataset` (4032 lecturas, 12 medidores).

### Riesgos residuales (verificación independiente, no bloqueantes)

| Sev. | Hallazgo | Archivo |
|---|---|---|
| HIGH | `RealProvider.GenerateExplanation` siempre devuelve `ErrProviderNotImplemented`: con `LLM_API_KEY` presente el runtime produce `LLMText` vacío (los tests usan el mock). | `internal/ai/provider.go` |
| MED | Race detector no ejecutable (falta gcc): la afirmación "race-free" no está probada con `-race`. | entorno |
| MED | Código muerto: paquete `internal/domain/services/` completo (16 archivos), `internal/data/csv_loader.go`, `internal/logger/`, `internal/ai/mock_client.go`, `handlers.Readings`, `handlers.AnomalyDetail`, y el parámetro `port` sin uso en `NewRouter`. | varios |
| MED | Deriva de config: `config.yaml` usa `data.csv_path`/`data.events_path` pero el código lee `data.readings_csv`/`data.events_csv`; las claves YAML se ignoran en silencio. | `internal/config/config.go` |
| LOW | `GET /api/meters/{id}/readings` devuelve `200 null` para medidor desconocido (nunca 404) y no está testeado. | `internal/api/router.go` |
| LOW | Un evento `DATA_QUALITY` puede reclasificar un spike real del mismo medidor como `DATA_QUALITY` (latente, no se dispara con el dataset actual). | `internal/analysis/correlator.go` |
| LOW | `DATA_QUALITY` usa confianza constante 0.90; severity/priority son función pura del `Type`. | `internal/analysis/scorer.go` |
| LOW | Sin enforcement de método HTTP; `lastRun` es el literal `"latest"`. | `router.go`, `handlers/ai.go` |

### Commits

**No se creó ningún commit.** La política de seguridad del orquestador prohíbe commitear sin pedido explícito del usuario, y no fue solicitado. El árbol de trabajo contiene todos los cambios listos para revisión.
