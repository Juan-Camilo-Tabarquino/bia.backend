# Backlog: pendientes conocidos de bia.backend

**Creado:** 2026-09-27 · **Estado de `main` al crearlo:** `4487840`
**Compañero de lectura:** `docs/architecture.md`, sección 13 (las mismas deudas,
en versión resumida y de cara al lector del repo)

## Cómo usar este archivo

No es un plan de ejecución ni una feature en curso: es una **lista de pendientes
conocidos** para retomar más adelante. Cada item es **independiente** — se puede
tomar uno solo sin tocar los demás.

Cuando agarres un item, el camino recomendado es **convertirlo en su propia
feature** con su `odd/<nombre>/tasks.md`, su rama, sus work units y su
verificación. Este archivo queda entonces como el índice de dónde salió.

Cada item declara: qué está mal, dónde exactamente, por qué importa, qué
significaría darlo por terminado, cómo verificarlo, y **qué está verificado y qué
no**. Esa última parte es la importante: los tres grupos tienen niveles de
evidencia distintos, y confundirlos es la forma más rápida de trabajar de más.

| Grupo | Naturaleza | Evidencia |
|---|---|---|
| A | Hallazgos del reviewer nativo | **Recuperados y descartados** (2026-09-27): el envelope no traía los claims, se reconstruyeron y los tres resultaron descartables |
| B | Drift documental | Verificado contra el código y los archivos — cerrado con B1-B4 |
| C | Robustez del código | Verificado por lectura del código, sin disparador en el dataset |

---

## Grupo A — Hallazgos advisory del review nativo

**Origen:** review del slice `review-c9ab19ba7248651a` (`feat/spanish-analysis-texts`,
tier medium, 18 archivos, 1 lens `review-reliability`), cerrado **aprobado** sin
corrección requerida.

El envelope de cierre declara textualmente que **los tres son no bloqueantes** y
están dispositionados como `informational`: *"none opened a correction, none
reopens this review... Treat them as separate later work, never as a reason to
re-run review on this candidate."*

### A0 — Primer paso obligatorio: recuperar los claims

**Qué está mal:** no lo sabemos. El envelope de cierre trae **id, severidad y
ubicación**, pero **no el texto** de cada hallazgo, y un `STATUS` posterior
tampoco los expone.

**Por qué importa:** sin los claims no se puede evaluar si cada hallazgo es real.
Cualquier trabajo sobre A1–A3 empieza por reconstruirlos leyendo el código en la
ubicación señalada y preguntándose qué vería un revisor de confiabilidad ahí.

**Cómo:** leer el código en las tres ubicaciones, listar las propiedades
sospechosas, y **clasificar cada hallazgo como real o descartable con evidencia**.
Si resultan descartables, cerrar A1–A3 de una y documentar por qué.

**Hecho significa:** cada uno de A1–A3 tiene claim reconstruido, veredicto
(real / descartable) y evidencia.

**Estado: DONE (2026-09-27).** La recuperación se hizo leyendo el código en las
tres ubicaciones. Los tres claims quedaron reconstruidos y clasificados como
**descartables**, cada uno con su evidencia en A1-A3. No había ningún hallazgo
real detrás: el grupo se conserva como registro de que se recuperaron y se
cerraron con evidencia, no como una lista pendiente.

### A1 — `internal/analysis/orchestrator.go:84-93` · severidad `WARNING`

Zona: el cuerpo de `Detect`, donde corre el pipeline determinístico y después se
publica bajo lock.

**Veredicto: descartable.** Verificado en `internal/analysis/orchestrator.go`:
todo el pipeline determinístico (`QualityChecker.Check`, `BaselineCalc.Calculate`,
`dataGapsFor`, `Detector.Detect`, `Correlator.Correlate`, `Classifier.Classify`,
`Scorer.Score`, `EvidenceBuilder.Build`) corre **antes** del `o.mu.Lock()` y opera
solo sobre valores locales. La publicación es una única sección crítica que
asigna `o.evidence`, `o.dataGaps` y `o.generation++` juntos, y `Evidence()` copia
bajo `RLock`. No hay forma de que un lector observe un snapshot partido: la
carrera que el hallazgo sospechaba ya está excluida por el diseño.

### A2 — `cmd/api/main.go:61-66` · severidad `WARNING`

Zona: la goroutine que lanza `Enrich()` en segundo plano. Sospecha razonable a
investigar: **qué pasa si `Enrich` falla o el proceso recibe una señal mientras
corre** — hoy no hay manejo de error en esa goroutine.

**Veredicto: descartable.** Verificado en `cmd/api/main.go`: la goroutine es
best-effort por diseño. `Detect()` ya publicó el payload determinístico completo
antes de arrancarla, así que si `Enrich` falla o el proceso recibe una señal solo
se pierde el texto opcional `llm_analysis`. La única brecha real es de
**observabilidad**, no de corrección: `internal/analysis/orchestrator.go` saltea
cada error del LLM con un `continue` sin loggear, y la goroutine no tiene
`recover`.

### A3 — `data/events.csv:2-5` · severidad `SUGGESTION`

Zona: las cuatro descripciones de eventos. Es un `SUGGESTION` sobre datos, no
sobre código. Sospecha razonable: las descripciones están **incrustadas** en el
`reason` determinístico vía el join de `evidenceBuilder.go`, así que un cambio de
dataset altera el texto de la API.

**Veredicto: descartable, y el mecanismo que afirmaba este backlog es incorrecto.**
Verificado contra `internal/analysis/evidenceBuilder.go`,
`internal/analysis/correlator.go` y `internal/api/handlers/endpoints.go`: de las
cuatro descripciones de `data/events.csv:2-5`, **solo dos** llegan al `reason`
determinístico —las ramas `EXPLAINABLE_ANOMALY` y `FALSE_POSITIVE` de
`describeEvidence`, que interpolan `eventDescription(...)`— (`M-104` y `M-106`).
Una tercera (`M-112`, tipo `DATA_QUALITY`) llega **solo** a
`correlated_events[].description`: `describeEvidence` no cita eventos en la rama
`DATA_QUALITY`, y `newAnomalyDTO` solo publica eventos cuando `Correlation.Explains`
es verdadero —lo que sí ocurre para `EventDataQuality`, que `explainsDeviation`
acepta—. La cuarta (`M-109`, evento `UNKNOWN`) **no llega a ningún campo de la
API**: `classifyType` la manda a `REAL_ANOMALY` (cuya plantilla no cita eventos) y
`anyExplainsDeviation` excluye explícitamente `EventUnknown`, así que
`CorrelatedEvents` sale vacío. No es un dato roto: es la restricción de diseño de
que un evento `UNKNOWN` nunca explica una desviación.

---

## Grupo B — Drift documental

Todo verificado contra el código. El token `/api/v1` y los directorios
inexistentes son afirmaciones que se comprobaron con `grep` y `git log -S`.
El impacto es de credibilidad: un lector o evaluador que abra estos archivos
recibe información que **contradice** el código que está mirando.

### B1 — `openspec/` describe una arquitectura que no existe — **RESUELTO** (2026-09-27)

- **Commit:** `e713860` — `docs: mark the openspec draft historical and correct
  the plan prefix`.
- **Dónde:** `openspec/specs/backend_implementation.sdd.yaml` (`status: draft`,
  versión 0.1.0) y `openspec/config.yaml`.
- **Qué dice mal:** chi como router (`:30` `internal/api/routes/ → chi router
  wiring`), `internal/ai/agents/` (`:28`), y evidencia de tests en
  `internal/domain/services/` (`openspec/config.yaml`), directorio **borrado**.
- **Qué NO contiene:** el token `/api/v1`. Verificado con `grep -rn "v1"
  openspec/` (sin resultados) y `git log --all -S"/api/v1" -- openspec` (vacío).
  Las rutas que lista son **sin prefijo** (`:55` `GET /health`).
- **Terminado significa:** la spec refleja la arquitectura real, o se la marca
  explícitamente como histórica/abandonada para que nadie la tome como vigente.
- **Ojo:** hay que decidir **qué** hacer con ella antes de reescribirla. La spec
  está en `draft` y nunca se completó.
- **Hechos corregidos:** ni la spec ni `config.yaml` afirman ya una arquitectura
  vigente. La spec conserva **intacto** su contenido arquitectónico y su
  `status: draft` (no se tocó el enum), pero lleva un banner
  histórico/abandonado como comentario YAML al inicio y una línea equivalente en
  su `## overview`, apuntando a `docs/architecture.md` y `docs/endpoints.md`
  como fuentes vigentes. Se comprobó que **ningún** código ni config del repo
  lee el campo `status:`. En `openspec/config.yaml` se reemplazaron los cinco
  ejemplos muertos de `internal\domain\services\*_test.go` por tests reales de
  hoy (`internal/analysis/`, `internal/api/`, etc.).
- **Qué cambió:** se marcó la spec como histórica sin reescribirla, y se
  corrigieron las rutas de evidencia de tests en `openspec/config.yaml`. El
  conteo `Test-like files detected (5)` se dejó igual a propósito: parece
  generado por tooling y hoy hay 14 archivos `*_test.go` en `internal/`.

### B2 — `docs/backend-implementation-plan.md` especifica `/api/v1/*` — **RESUELTO** (2026-09-27)

- **Dónde:** `docs/backend-implementation-plan.md:159-163`.
- **Qué está mal:** documenta cinco rutas `/api/v1/...` que **no existen**. El
  código no tiene v1, lo rechaza a propósito y hay un test que lo fija
  (`internal/api/api_test.go`, `TestRouterRegistersOnlyAPIPrefixedRoutes`).
- **Este es el archivo donde vive `/api/v1`**, no `openspec/` (ver B1).
- **Terminado significa:** el plan apunta al prefijo `/api` real, o marca
  claramente que `/api/v1` era la intención original y no se implementó.
- **Commit:** `e713860` — `docs: mark the openspec draft historical and correct
  the plan prefix`.
- **Hechos corregidos:** las cinco entradas de
  `docs/backend-implementation-plan.md:159-163` ahora usan el prefijo real
  `/api`; la quinta pasó de `/api/v1/anomalies/{meter_id}` a
  `/api/anomalies/{id}`, porque el path var es el **id estable de la anomalía**,
  no un `meter_id` (ver `docs/endpoints.md` y
  `internal/api/handlers/ai.go`). Se agregó una nota en inglés que registra que
  `/api/v1` fue la intención original y nunca se implementó, y apunta a
  `docs/endpoints.md`. Auditoría del resto del archivo: no hay ninguna otra
  referencia a `/api/v1` ni a esas cinco rutas. Se mantiene el hallazgo
  original: el token `/api/v1` vive en este plan y **no** en `openspec/`.
- **Qué cambió:** el prefijo `/api/v1` → `/api` en las cinco rutas, el path var
  de la ruta de detalle de anomalía, y una nota de intención original. No se
  modernizó nada más del plan.

### B3 — `docs/routing.md:35` muestra una firma vieja — **RESUELTO** (2026-09-27)

- **Commit:** `75a4741` — `docs: fix the routing and endpoint drift (B3, B4)`.
- **Hechos corregidos:** `internal/api/router.go:33` es
  `func NewRouter(orchestrator *analysis.Orchestrator) http.Handler`: el
  parámetro `port` no existe. Además, el health handler no vive en `router.go`
  sino en `internal/api/handlers/endpoints.go:15-19` (`func Health`), y el bloque
  de código de `NewRouter` citaba un símbolo `healthHandler` inexistente (el real
  es `handlers.Health`).
- **Qué cambió:** se corrigieron la firma citada, el handler de health dentro del
  bloque y la sección "Health check". El resto de `docs/routing.md` (bloque de
  `corsWrapper`, tabla de rutas y sección "Adding a new route") se auditó contra
  el código y ya coincidía.

### B4 — `docs/endpoints.md` sub-documenta el DTO de anomalía — **RESUELTO** (2026-09-27)

- **Commit:** `75a4741` — `docs: fix the routing and endpoint drift (B3, B4)`.
- **Hechos corregidos:** el ejemplo de `GET /api/anomalies` mostraba 10 campos y
  el `AnomalyDTO` (`internal/api/handlers/endpoints.go:36-72`) emite 18. Los 8
  faltantes son `priority` (int), `baseline` (objeto: `mean`, `stddev`, `count`,
  `voltage_mean`, `current_mean`, `power_factor_mean`), los cuatro
  `*_change_pct` (float64), `correlated_events` (array de objetos; siempre array,
  nunca `null`) y `data_quality` (objeto: `flagged`, `reason`).
- **Qué cambió:** el ejemplo lista los 18 campos con sus tipos y sub-claves, y se
  recomputaron todas las referencias `Source:` del archivo contra el código actual
  (health, reports, meters, meter-detail, readings, anomalies, anomaly-detail,
  analyze, analysis-get y dashboard-summary), más la referencia de `corsWrapper`
  y la de `models.Evidence`.

---

## Grupo C — Robustez del código

### C1 — Varianza sin guarda para una sola lectura — **RESUELTO** (2026-09-27)

- **Commit:** `3051333` — `fix(analysis): require two readings before computing a
  meter baseline`.
- **Qué pasaba, con los hechos corregidos:** el texto original de este item decía
  `+Inf` y "comportamiento indefinido". No era así. Con `cnt == 1`, `varSum` es
  idénticamente `0`, así que la división `0/0` daba **siempre `NaN`**, nunca
  `+Inf`. Y **solo la regla de pico** (`detector.go:82`) consume `b.StdDev`; la
  regla de caída es puramente porcentual y no lo mira. El modo de falla real no
  era indefinido: el `NaN` llegaba a `baseline.stddev` en el DTO de
  `/api/anomalies` y en el payload crudo de `/api/reports`, `json.Marshal` lo
  rechaza (`unsupported value: NaN`) y **ambos handlers descartan el error de
  encode**, así que respondían **200 con cuerpo vacío** en lugar de fallar
  ruidosamente.
- **Decisión:** exigir **al menos 2 lecturas** (`minReadingsForBaseline`, en
  `internal/analysis/baseline.go`). Con menos, el medidor **queda afuera del mapa
  de líneas base** y se lo reporta como **medidor no validado** en
  `GET /api/dashboard/summary` (campo `unvalidatedMeters`); nunca se fabrica una
  anomalía para él: no se agregó tipo, `Kind` ni severidad.
- **Evidencia:** commit `3051333` y los tests
  `TestBaselineCalculatorRequiresTwoReadings`,
  `TestOrchestrator_DataGapsForSingleReadingMeter`,
  `TestAnomaliesAndSummaryForSingleReadingMeter` y
  `TestDashboardSummaryUnvalidatedMetersEmptyShape`. Los 4 resultados benchmark
  de `requirements_dataset_test.go` quedaron sin cambios: todos los medidores del
  dataset tienen 336 lecturas, así que la guarda no puede dispararse ahí.

### C2 — La prosa del clasificador ignora el signo de la desviación

- **Dónde:** `internal/analysis/evidenceBuilder.go:43` y `:45`.
- **Qué pasa:** las plantillas de `REAL_ANOMALY` y `EXPLAINABLE_ANOMALY` dicen
  siempre **"por encima de"**, incluso si la anomalía se originó en una **caída**
  (`delta` negativo).
- **Por qué es latente y no visible:** con el dataset actual esos dos tipos
  **solo surgen de picos**. Una caída cae en `CONSUMPTION_DROP`, y si no tiene
  evento explicativo el clasificador la manda a `REAL_ANOMALY`, cuyo texto diría
  "está X% por encima de su línea base" con un X negativo. O sea: el bug está
  esperando a que alguien use otro dataset.
- **Terminado significa:** el texto refleja el signo real (por encima / por
  debajo), con un test que cubra una caída clasificada como real.
- **Ojo:** cambiar estas plantillas toca texto que hoy está fijado por tests de
  igualdad exacta. Hay que actualizarlos en el mismo commit, no antes.

### C3 — `/api/reports` expone el modelo de dominio crudo

- **Dónde:** `internal/api/router.go:106-121`.
- **Qué pasa:** serializa `models.Evidence` directamente, así que sus claves JSON
  son **nombres de campo Go** (`MeterID`, `Baseline`, `Correlation`) y expone el
  enum `Kind` en inglés (`CONSUMPTION_SPIKE`), a diferencia de `/api/anomalies`,
  que usa DTOs con claves en snake_case.
- **Por qué está acá y no en el grupo B:** **no es un bug de documentación**, es
  una decisión de producto sin tomar. Las claves en inglés son contrato técnico y
  no deberían traducirse; la pregunta abierta es si este endpoint debería existir
  con esta forma o reusar el DTO de anomalías.
- **Terminado significa:** una decisión explícita. Si se deja como está, conviene
  que `docs/endpoints.md` lo diga de forma prominente para que nadie asuma que
  comparte la forma de `/api/anomalies`.

### C4 — Valores no finitos vuelven a entrar por la puerta de los datos

- **Dónde:** `internal/data/csv/loader.go` (parseo numérico con
  `strconv.ParseFloat`) y `internal/analysis/quality.go:35` (`Check`).
- **Qué pasa:** `strconv.ParseFloat` acepta las cadenas `NaN`, `Inf` e
  `Infinity`, y el filtro de calidad solo descarta consumo negativo
  (`r.Consumption < 0`, y `NaN < 0` es falso, así que un `NaN` pasa) y status
  distinto de `OK`. Una lectura así vuelve a producir una línea base no finita:
  la precondición de las 2 lecturas de C1 no cubre este caso. En la misma familia,
  un `varSum` que desborde a `+Inf` con entradas **finitas** enormes (por ejemplo
  `1e200`) da un desvío infinito (`internal/analysis/baseline.go`).
- **Por qué importa:** es exactamente el modo de falla que C1 creyó cerrar,
  alcanzable por otra vía: el dato entra por la carga, no por el número de
  lecturas.
- **Estado de verificación:** verificado por lectura del código; sin disparador en
  el dataset.
- **Terminado significa:** validar finitud en la carga o en el filtro, y cubrirlo
  con un test.

### C5 — Los handlers descartan el error de encode

- **Dónde:** `internal/api/handlers/endpoints.go:206` (el `writeJSON` compartido)
  y `internal/api/router.go:112` (handler de `/api/reports`).
- **Qué pasa:** ambos ignoran el error de `json.NewEncoder(...).Encode(...)`, y
  `writeJSON` ya escribió el header `200` antes de intentar el encode.
- **Por qué importa:** cualquier valor no finito que llegue al payload (ver C4)
  responde **200 con cuerpo vacío** en lugar de fallar ruidosamente con un 500.
- **Estado de verificación:** verificado por lectura del código; son las mismas
  líneas que ya cita `docs/architecture.md` §13.
- **Terminado significa:** comprobar el error del encode y responder un 500 con
  cuerpo de error cuando el payload no sea serializable.

### C6 — `SERVER_PORT` documentado pero no usado ni vinculado (heredado de §13)

- **Dónde:** `internal/config/config.go` (comentario del header) y `Load()`.
- **Qué pasa:** el comentario documenta `SERVER_PORT` con default `8080`, pero el
  código hace `v.SetDefault("server.port", 3001)` y **nunca** registra
  `v.BindEnv("server.port", ...)`. El puerto solo se cambia por `config.yaml`.
- **Por qué importa:** un operador que exporta `SERVER_PORT` espera otro puerto y
  el server sigue escuchando en 3001.
- **Estado de verificación:** verificado por lectura del código; es la misma deuda
  que `docs/architecture.md` §13. Se agrega acá porque hasta hoy existía **solo**
  en §13 y los dos documentos no coincidían.
- **Terminado significa:** alinear comentario y default, y vincular la variable de
  entorno (o borrar la mención).

### C7 — Arranque sin señal de readiness (heredado de §13)

- **Dónde:** `internal/api/handlers/endpoints.go:15-19` (`Health`) y el tag
  `json:"llm_analysis,omitempty"` del `AnomalyDTO` en el mismo archivo.
- **Qué pasa:** `GET /api/health` responde `200` desde el primer instante, pero
  mientras corre el enriquecimiento en segundo plano `llm_analysis` puede estar
  **ausente** del JSON (el campo es `omitempty`, así que se omite la clave entera
  y no se emite `""`), aunque el resto del payload ya es final y correcto.
- **Por qué importa:** un cliente no puede distinguir "listo y sin narrativa" de
  "todavía enriqueciendo".
- **Estado de verificación:** verificado por lectura del código; es la misma deuda
  que `docs/architecture.md` §13, agregada acá por la misma razón que C6.
- **Terminado significa:** una señal explícita de readiness, o documentar que la
  ausencia de `llm_analysis` es el estado válido.

---

## Pendientes heredados de `odd/review-backend-plan/tasks.md`

Tres ítems de ese archivo siguen genuinamente abiertos y no tienen diseño
iniciado. Se indexan acá para que el backlog quede completo; el detalle y el
contexto original viven en `odd/review-backend-plan/tasks.md`.

- **Structured logging:** no existe un framework de logging estructurado; el
  código usa el `log` estándar y `internal/logger` (zerolog) fue eliminado en
  `repo-hygiene`.
- **Especificación OpenAPI:** no existe spec OpenAPI/Swagger en el repo; el
  contrato HTTP se documenta a mano en `docs/endpoints.md`.
- **Workflow de CI:** no existe `.github/` ni ningún workflow; `go test`, lint y
  build no corren automáticamente.

---

## Orden sugerido

Con B1-B4, C1 y todo el grupo A ya cerrados, el conjunto de pendientes cambió por
completo: el orden anterior estaba armado alrededor de trabajo que ya no existe.
Hoy lo que más rinde por esfuerzo es cerrar la vía por la que un dato no finito
todavía puede romper la respuesta.

1. **C5** primero: son dos llamadas (`endpoints.go:206` y `router.go:112`), el
   arreglo es chico y convierte un fallo silencioso (200 con cuerpo vacío) en un
   500 ruidoso. Es el que más rinde por esfuerzo de toda la lista.
2. **C4** inmediatamente después: sin la validación de finitud en la carga, C5
   solo cambia el silencio por un error; juntos cierran la clase de falla que C1
   creyó cerrar.
3. **C6** — trivial (comentario, default y binding de entorno) y elimina una
   trampa real para quien despliega.
4. **C7** — chico, y deja explícito qué significa que `llm_analysis` falte.
5. **C2** — barato, pero sigue latente: conviene esperar a tener un caso que lo
   dispare o a tocarlo por otro motivo.
6. **C3** — es una decisión de producto; no hay trabajo técnico hasta que se
   tome.
7. **Los tres heredados de `review-backend-plan`** (logging estructurado, OpenAPI,
   CI) al final: cada uno es una feature propia, no un ajuste, y ninguno
   desbloquea a otro pendiente.

**Ya no participan del orden:** A0-A3 (recuperados y descartados, ver arriba),
B1-B4 (commits `e713860` y `75a4741`) y C1 (commit `3051333`).

## Fuera de alcance de este backlog

- Los tres PRs (`#1`, `#2`, `#3`) están mergeados en `main`. El fix de las dos
  lecturas vive en la rama `fix/baseline-min-two-readings` (commits `6047d80` a
  `e713860`) y **todavía no está mergeado**: `main` sigue en `210f727`.
  **Corrección posterior:** el fix **ya está mergeado** en `main`; la guarda de
  las dos lecturas vive y está activa en `internal/analysis/baseline.go`
  (`minReadingsForBaseline = 2`, commit `3051333`).
- La traducción al español de la superficie de análisis **está hecha y
  verificada**; no queda texto inglés en campos visibles de usuario. Lo que
  queda en inglés son tokens de contrato, claves JSON y el body de `README.md`,
  y en los tres casos es deliberado.
- Las narrativas del LLM: un modelo en vivo no es determinista. La instrucción de
  idioma del prompt eleva la probabilidad de español pero no la garantiza, y eso
  es una propiedad del modelo, no una deuda del repo.
