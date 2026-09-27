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
| A | Hallazgos del reviewer nativo | **No verificado**: el envelope no trae los claims |
| B | Drift documental | Verificado contra el código y los archivos |
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

### A1 — `internal/analysis/orchestrator.go:84-93` · severidad `WARNING`

Zona: el cuerpo de `Detect`, donde corre el pipeline determinístico y después se
publica bajo lock.

### A2 — `cmd/api/main.go:61-66` · severidad `WARNING`

Zona: la goroutine que lanza `Enrich()` en segundo plano. Sospecha razonable a
investigar: **qué pasa si `Enrich` falla o el proceso recibe una señal mientras
corre** — hoy no hay manejo de error en esa goroutine.

### A3 — `data/events.csv:2-5` · severidad `SUGGESTION`

Zona: las cuatro descripciones de eventos. Es un `SUGGESTION` sobre datos, no
sobre código. Sospecha razonable: las descripciones están **incrustadas** en el
`reason` determinístico vía el join de `evidenceBuilder.go`, así que un cambio de
dataset altera el texto de la API.

---

## Grupo B — Drift documental

Todo verificado contra el código. El token `/api/v1` y los directorios
inexistentes son afirmaciones que se comprobaron con `grep` y `git log -S`.
El impacto es de credibilidad: un lector o evaluador que abra estos archivos
recibe información que **contradice** el código que está mirando.

### B1 — `openspec/` describe una arquitectura que no existe — **RESUELTO** (2026-09-27)

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
- **Commit:** `_pendiente_`.
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
- **Commit:** `_pendiente_`.
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

- **Commit:** `_pendiente_`.
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

- **Commit:** `_pendiente_`.
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

---

## Orden sugerido

Si hay que elegir, el que más rinde por esfuerzo es **B4**, y después el resto del
drift documental (B1/B2/B3).

1. **A0** primero, porque desbloquea (o cierra) todo el grupo A y no requiere
   escribir código.
2. **B4** — el impacto es real para el frontend, el arreglo es acotado y es puro
   texto.
3. **B1/B2/B3** — juntos, porque son el mismo tipo de trabajo y comparten
   decisión de política ("¿qué hacemos con los documentos históricos?").
4. **C2** — barato, pero conviene esperar a tener un caso que lo dispare o a
   tocarlo por otro motivo.
5. **C3** — es una decisión de producto; no hay trabajo técnico hasta que se
   tome.

**C1 ya está resuelto** (commit `3051333`, 2026-09-27 y ver arriba), así que sale
del conjunto de pendientes y no participa del orden.

## Fuera de alcance de este backlog

- Los tres PRs (`#1`, `#2`, `#3`) están mergeados y `main` está al día.
- La traducción al español de la superficie de análisis **está hecha y
  verificada**; no queda texto inglés en campos visibles de usuario. Lo que
  queda en inglés son tokens de contrato, claves JSON y el body de `README.md`,
  y en los tres casos es deliberado.
- Las narrativas del LLM: un modelo en vivo no es determinista. La instrucción de
  idioma del prompt eleva la probabilidad de español pero no la garantiza, y eso
  es una propiedad del modelo, no una deuda del repo.
