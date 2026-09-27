# Arquitectura de bia.backend

Documento de referencia del backend: qué hace, cómo está organizado, cómo funciona
cada etapa del análisis, cómo se integra el LLM y qué se puede verificar.

Describe el estado de `main` en el commit `e6536c7`. **Los identificadores,
rutas, claves JSON, tokens de contrato y nombres de archivo se mantienen en
inglés a propósito**: son parte del contrato técnico. La prosa va en español.

---

## 1. Qué es este proyecto

Backend en Go de una plataforma de gestión energética asistida por IA. Carga
mediciones eléctricas desde CSV, corre un pipeline **completamente determinístico**
sobre ellas y expone los resultados por HTTP. El LLM participa **solo narrando**:
explica una anomalía que ya fue detectada, clasificada y puntuada por reglas.

Esa separación no es un detalle de implementación, es el requisito central: el
enunciado exige que el LLM no sea el detector. Se cumple de forma estructural —
el orquestador corre el LLM **después** de publicar la evidencia, y un fallo del
modelo deja el campo `LLMText` vacío sin alterar nada más
(`internal/analysis/orchestrator.go`, `internal/ai/provider.go`).

**Módulo**: `github.com/neuralium/ai-energy` · **Go**: 1.22 (`go.mod`)

---

## 2. Cómo levantarlo

```sh
go mod tidy
go build ./cmd/api
./api
```

En desarrollo, desde la raíz del repositorio:

```sh
go run cmd/api/main.go
```

El servidor escucha en el puerto **3001** por defecto. Requiere que exista
`config.yaml` **en el directorio de trabajo**: `Load()` lo resuelve con
`SetConfigFile("config.yaml")`, así que las rutas de búsqueda de Viper quedan
anuladas y un `config.yaml` en el directorio padre **no** se encuentra. Si falta,
`Load()` devuelve error y el proceso no arranca (`internal/config/config.go`).

**Arranque esperado**: el puerto queda disponible en **menos de un segundo**. El
pipeline determinístico corre sincrónicamente antes de escuchar; el
enriquecimiento con LLM continúa en segundo plano y, con el proveedor real y el
dataset incluido, tarda entre uno y dos minutos y medio.

### Variables de entorno

| Variable | Clave de configuración | Default |
|---|---|---|
| `READINGS_CSV` | `data.readings_csv` | `data/readings.csv` |
| `EVENTS_CSV` | `data.events_csv` | `data/events.csv` |
| `LLM_API_KEY` | `llm.api_key` | `""` |
| `LLM_BASE_URL` | `llm.base_url` | `https://ollama.com` |
| `LLM_MODEL` | `llm.model` | `gpt-oss:20b` |

`server.port` **no tiene binding de entorno**: se cambia solo por `config.yaml`.
El comentario de `config.go` menciona `SERVER_PORT` con default `8080`, pero el
código usa 3001 y nunca registra ese binding. Es una discrepancia conocida entre
comentario y código (ver §13).

**Sin `LLM_API_KEY`** el backend funciona igual: usa un proveedor determinístico
local (`internal/analysis/llm.go`) que devuelve narrativas fijas. Es el modo
offline/demo, y arranca en 0 segundos.

---

## 3. Mapa del repositorio

```
cmd/api/main.go              arranque: config -> stages -> Detect() sync -> go Enrich() -> ListenAndServe()
internal/analysis/           el pipeline determinístico (paquete plano)
  quality.go                 filtrado de lecturas
  baseline.go                estadísticas por medidor
  detector.go                reglas de detección + medianas por hora
  correlator.go              cruce con eventos operativos
  classifier.go              los 4 tipos de anomalía
  scorer.go                  severidad, confianza, prioridad
  evidenceBuilder.go         texto determinístico (motivo y acción)
  llm.go                     proveedor mock determinístico
  orchestrator.go            orquestación y concurrencia
internal/ai/                 integración con el LLM real
  provider.go                request a Ollama, selección de proveedor, prompt
  payload.go                 qué se le manda al modelo
internal/api/                capa HTTP (net/http puro, sin framework)
  router.go                  registro de rutas y CORS
  handlers/endpoints.go      DTOs y handlers de lectura
  handlers/ai.go             detalle, análisis y dashboard
internal/config/config.go    configuración con Viper
internal/data/csv/           parser de CSV tolerante
internal/data/memory/        repositorios en memoria
internal/domain/models/      modelos de dominio y enums
data/                        readings.csv y events.csv
docs/                        documentación (mixta: endpoints/routing/plan en inglés,
                             requerimientos y architecture en español)
odd/                         registro de features trabajadas (español)
openspec/                    spec SDD en borrador (desactualizada, ver §13)
```

**Decisiones de estructura que conviene saber:**

- `internal/analysis/` es un **paquete plano**, no subpaquetes por etapa. Las
  etapas se comunican por interfaces definidas en el mismo paquete.
- El router usa **`net/http.ServeMux` puro**, sin chi ni ningún framework. Fue
  una decisión explícita, y hay un test que la protege.
- Los repositorios en memoria son **append-only**, no thread-safe por diseño: la
  seguridad la da el lock del orquestador y el hecho de que el dataset se carga
  una sola vez.

---

## 4. Flujo de datos

```
data/readings.csv ─┐
                   ├─> csv.Loader ─> memory repos ─┐
data/events.csv  ──┘                                │
                                                    v
                              Orchestrator.Detect()  (determinístico, sincrónico)
                                quality -> baseline -> detector -> correlator
                                -> classifier -> scorer -> evidenceBuilder
                                                    │
                                          publica evidence  (lock breve)
                                                    │
                       ┌────────────────────────────┴──────────────────┐
                       v                                               v
          http.ListenAndServe (API)                    Orchestrator.Enrich()  (background)
          lee evidencia publicada                        LLM por anomalía -> LLMText
```

El `Loader` **relee los CSV en cada `Detect()`**, así que un cambio en disco se
refleja en la próxima corrida. Los repositorios se pueblan **una sola vez**
(guard `loaded`) porque son append-only y repoblarlos duplicaría cada lectura.

---

## 5. El pipeline determinístico, etapa por etapa

Entrada del pipeline: `Orchestrator.Detect()`
(`internal/analysis/orchestrator.go`), que ejecuta en orden
`Check → Calculate → Detect → Correlate → Classify → Score → Build`.

### 5.1 Quality — `quality.go`

Filtra lecturas. **Solo dos reglas**, y ninguna mira voltaje, corriente ni factor
de potencia:

| Regla | Condición | Efecto |
|---|---|---|
| A | `r.Consumption < 0` | se descarta |
| B | `r.Status != "" && !EqualFold(TrimSpace(r.Status), "ok")` | se descarta |

Consecuencia de B: un `status` **vacío se conserva** (la fuente no reportó
calidad); un `status` no vacío se compara sin distinguir mayúsculas ni espacios,
así que `"OK"`, `"ok"` y `" Ok "` pasan, y cualquier otro valor se descarta.

Las lecturas conservadas son la única entrada de la línea base y, por
transitividad, de las medianas horarias y de la detección.

### 5.2 Baseline — `baseline.go`

**Precondición: al menos 2 lecturas por medidor**
(`minReadingsForBaseline = 2`). La tabla y el párrafo de Bessel de abajo
describen a los medidores que la cumplen. Con menos de 2 lecturas el medidor
**queda afuera del mapa de líneas base**: su varianza y su desvío simplemente no
se calculan. No se lo descarta en silencio — se lo reporta como **medidor no
validado** en el resumen del dashboard, con la razón del contrato (§9).

Calcula, por medidor, en dos pasadas:

| Estadístico | Fórmula |
|---|---|
| Media de consumo | `mean = Σconsumption / cnt` |
| Varianza | `variance = Σ(r.Consumption − mean)² / (cnt − 1)` |
| Desvío estándar | `sd = √variance` |
| Medias por señal | `VoltageMean = Σvoltage/cnt`, `CurrentMean`, `PowerFactorMean` |

**La varianza usa corrección de Bessel** (denominador `cnt − 1`), y está
documentado como intencional. Esa misma corrección es la razón de la
precondición: con una sola lectura, `varSum` es idénticamente `0` y la división
es `0/0`, o sea un desvío `NaN`. Exigir 2 lecturas vuelve ese desvío no finito
**imposible** en lugar de improbable. Importante: la varianza se calcula sobre la
**serie completa del medidor**, incluyendo la ventana anómala — o sea que una
anomalía **infla su propia línea base** y se vuelve más difícil de detectar
contra sí misma.

Las medianas por hora del día **no** se calculan acá: viven en el detector.

### 5.3 Detector — `detector.go`

Constantes, con sus valores literales:

| Constante | Valor | Significado |
|---|---|---|
| `spikeRatioThreshold` | `0.45` | suba relativa sobre la media que califica como pico |
| `dropRatioThreshold` | `0.55` | baja relativa bajo la media que califica como caída |
| `nearMedianTolerance` | `0.25` | distancia relativa máxima a la mediana horaria para seguir siendo "estable" |
| `voltageMin` | `209.0` | límite inferior de la banda de red (220 V − 5 %) |
| `voltageMax` | `231.0` | límite superior (220 V + 5 %) |
| `powerFactorMin` | `0.85` | factor de potencia mínimo sano |

**Delta**: `delta = (r.Consumption − b.Mean) / b.Mean` — una razón con signo, donde
`0.5` significa `+50 %`. Se guarda en el candidato junto con las cuatro
variaciones porcentuales por señal.

`signedChangePct(value, mean)` devuelve **exactamente `0`** cuando `mean == 0`
(nunca `NaN` ni `±Inf`).

**Reglas de decisión** — es un `switch` con **primera coincidencia gana**, y las
ramas son mutuamente excluyentes:

| Orden | Condición exacta | `Kind` |
|---|---|---|
| 1 | `r.Consumption > b.Mean + 3*b.StdDev` **o** `delta >= 0.45` | `CONSUMPTION_SPIKE` |
| 2 | `-delta >= 0.55` | `CONSUMPTION_DROP` |
| 3 | rama por defecto: `electricalInconsistency(r)` **y** `consumptionNearHourMedian(...)` | `DATA_QUALITY` |

Detalles que importan:

- El test de sigma es **estricto** (`>`): un consumo exactamente igual a
  `Mean + 3·StdDev` no es pico. El test porcentual es `>=`.
- **La calidad de datos se evalúa solo en la rama por defecto.** Como el spike
  gana primero, una lectura elevada con factor de potencia bajo **no** se degrada
  a problema de calidad. Esto es deliberado y está comentado en el código.
- `electricalInconsistency` **chequea el factor de potencia primero**: si
  `0 < PF < 0.85` devuelve la razón de PF; si no, evalúa la banda de tensión.
  Voltaje o PF en cero se tratan como "no reportado" y no son inconsistentes.
- `consumptionNearHourMedian` exige que el consumo esté a **`<= 0.25`** de
  distancia relativa de la mediana de esa hora.

Las **medianas por hora del día** se calculan agrupando por `MeterID` y
`Timestamp.Hour()`; la mediana ordena una copia y promedia los dos centrales si
la cantidad es par.

### 5.4 Correlación de eventos — `correlator.go`

| Constante | Valor |
|---|---|
| `correlationLeadTime` | `1 hora` **antes** del inicio del evento |
| `correlationHorizon` | `12 horas` **después** del fin del evento |

La ventana de matching es inclusiva en ambos extremos:
`[e.Start − 1h, e.End + 12h]`. Como el loader carga cada evento con
`Start = End` (los eventos del dataset son puntuales), en la práctica la ventana
es `[Start − 1h, Start + 12h]`.

**Alcance por medidor**: el loader guarda el `meter_id` del CSV en el campo `ID`
del evento. Si ese `ID` coincide con un medidor conocido, el evento es
*meter-scoped* y solo matchea candidatos de ese medidor. Si no, matchea
cualquier medidor.

`Explains` es verdadero si **al menos uno** de los eventos matcheados tiene tipo
explicativo. Son explicativos: `SCHEDULED_OUTAGE`, `MAINTENANCE`, `SHUTDOWN`,
`OPERATIONAL_CHANGE`, `PRODUCTION_LINE`, `DATA_QUALITY`. **`UNKNOWN` no explica
nunca** — por diseño: un evento desconocido no puede justificar una desviación.

### 5.5 Clasificador — `classifier.go`

**Agrupa** los candidatos por la clave `(MeterID, Kind)`. De cada grupo:

- el **representante** es el candidato de mayor `|Delta|`;
- los eventos se unen y deduplican por `(ID, Type, Start)`;
- `explains` se combina con OR.

O sea: una anomalía sostenida produce **un solo** `Evidence`, y basta un
candidato explicado para que todo el grupo quede explicado.

**Tabla de decisión**, en orden de evaluación — la precedencia es lo importante:

| Orden | Condición | Tipo resultante |
|---|---|---|
| 1 | `representative.Kind == KindDataQuality` **o** hay evento `DATA_QUALITY` | `DATA_QUALITY` |
| 2 | hay evento `SCHEDULED_OUTAGE`, `MAINTENANCE` o `SHUTDOWN` | `FALSE_POSITIVE` |
| 3 | hay evento `OPERATIONAL_CHANGE` o `PRODUCTION_LINE` | `EXPLAINABLE_ANOMALY` |
| 4 | sin eventos, o solo `UNKNOWN` | `REAL_ANOMALY` |

Consecuencias de la precedencia: un grupo con mantenimiento **y** cambio
operativo es `FALSE_POSITIVE`; un grupo de calidad de datos es `DATA_QUALITY`
aunque además tenga un evento de mantenimiento.

**Estado** (`status` del DTO): es `explained` si el grupo está explicado **y** el
tipo es `EXPLAINABLE_ANOMALY`, `FALSE_POSITIVE` o `DATA_QUALITY`. Un
`REAL_ANOMALY` es **siempre** `unexplained`, incluso si estuviera explicado.

### 5.6 Scoring — `scorer.go`

**Severidad** — por tipo, sin fórmulas:

| Tipo | Severidad |
|---|---|
| `REAL_ANOMALY`, `DATA_QUALITY` | `HIGH` |
| `EXPLAINABLE_ANOMALY` | `MEDIUM` |
| `FALSE_POSITIVE` | `LOW` |

**Prioridad** — entero, `1` es lo más urgente:

| Tipo | Prioridad |
|---|---|
| `REAL_ANOMALY` | `1` |
| `DATA_QUALITY` | `2` |
| `EXPLAINABLE_ANOMALY` | `3` |
| `FALSE_POSITIVE` | `4` |

**Confianza** — no es una combinación ponderada, sino una **función afín por
tipo** sobre `magnitude = min(|Delta|, 1.2)`:

| Tipo | Fórmula | Rango |
|---|---|---|
| `REAL_ANOMALY` | `0.85 + 0.10·magnitude` | `[0.85, 0.97]` |
| `DATA_QUALITY` | `0.90` (constante) | `0.90` |
| `EXPLAINABLE_ANOMALY` | `0.80 + 0.05·magnitude` | `[0.80, 0.86]` |
| `FALSE_POSITIVE` | `0.75 + 0.05·magnitude` | `[0.75, 0.81]` |

El resultado pasa por un `clamp01`. El orden final de la lista **no** lo decide
este paquete: lo hace el handler con un `sort.SliceStable` por `Priority`
ascendente, después `Timestamp` y después `MeterID`.

### 5.7 Evidence builder — `evidenceBuilder.go`

Es la etapa que produce **el texto que ve el usuario** — y es 100 % determinística.
No pasa por el LLM.

| Campo | Origen |
|---|---|
| `Explanation` | `describeEvidence(e)` — se expone como `reason` |
| `Recommendation` | `recommendAction(e.Type)` — se expone como `recommended_action` |

Plantillas de `describeEvidence`, por tipo:

- `DATA_QUALITY` con razón: `"El medidor %s presenta lecturas eléctricas inconsistentes (%s) mientras su consumo se mantiene cerca de la línea base horaria."`
- `REAL_ANOMALY`: `"El consumo del medidor %s está %.1f%% por encima de su línea base sin ningún evento operativo conocido."`
- `EXPLAINABLE_ANOMALY`: `"...y coincide con un cambio operativo conocido: %s."`
- `FALSE_POSITIVE`: `"La desviación del consumo del medidor %s de %.1f%% se explica por mantenimiento planificado: %s."`

`eventDescription` lista los eventos como `"<Description> (<Type>)"`, unidos por
`"; "`, y devuelve `"sin evento registrado"` cuando no hay ninguno.

`recommendAction` mapea: `REAL` → `"Revisar el medidor y su instalación."`;
`DATA_QUALITY` → `"Revisar la calibración del sensor y el proceso de calidad de datos de este medidor."`;
`EXPLAINABLE` y `FALSE_POSITIVE` → `"No se requiere acción..."`; default →
`"Revisar la lectura antes de escalar."`.

---

## 6. Los cuatro casos del dataset, punta a punta

El dataset está **construido a propósito** para que cuatro medidores ejerciten
las cuatro clasificaciones. Esta sección sigue el código para mostrar cómo cada
uno llega a su resultado publicado. Es la mejor forma de entender el pipeline
completo.

| Medidor | Tipo | Severidad | Prioridad | Confianza |
|---|---|---|---|---|
| M-104 | `EXPLAINABLE_ANOMALY` | `MEDIUM` | 3 | `0.80 + 0.05·min(\|Δ\|,1.2)` |
| M-106 | `FALSE_POSITIVE` | `LOW` | 4 | `0.75 + 0.05·min(\|Δ\|,1.2)` |
| M-109 | `REAL_ANOMALY` | `HIGH` | 1 | `0.85 + 0.10·min(\|Δ\|,1.2)` |
| M-112 | `DATA_QUALITY` | `HIGH` | 2 | `0.90` (constante) |

### M-104 → `EXPLAINABLE_ANOMALY` / `MEDIUM`

Consumo escalona al alza desde `2026-09-11 00:00`. El evento existe:
`OPERATIONAL_CHANGE` — "Nueva línea de producción activada".

1. **Quality**: todas las filas con `status=OK` y consumo positivo → se conservan.
2. **Baseline**: media y desvío sobre las 336 lecturas del medidor.
3. **Detector**: las lecturas elevadas cumplen `delta >= 0.45` → `CONSUMPTION_SPIKE`.
4. **Correlación**: el evento es *meter-scoped* al medidor, cae en la ventana
   `[2026-09-10 23:00, 2026-09-11 12:00]`, y `OPERATIONAL_CHANGE` es explicativo
   → `Explains = true`.
5. **Clasificador**: hay evento operativo y ningún outage → regla 3 →
   `EXPLAINABLE_ANOMALY`, con estado `explained`.
6. **Scoring**: severidad `MEDIUM`, prioridad `3`.
7. **Evidencia**: el motivo incrusta la descripción del evento, ya traducida.

### M-106 → `FALSE_POSITIVE` / `LOW`

Caída sostenida durante `2026-09-08`, con retorno a la normalidad a las 12:00.
El evento existe: `SCHEDULED_OUTAGE` — "Parada programada de mantenimiento de 12 horas".

1. **Detector**: las lecturas bajas cumplen `-delta >= 0.55` → `CONSUMPTION_DROP`.
   Las lecturas de recuperación **no** superan el umbral de pico, así que no
   generan un segundo grupo.
2. **Correlación**: `SCHEDULED_OUTAGE` es explicativo → `Explains = true`.
3. **Clasificador**: regla 2 → `FALSE_POSITIVE`, `explained`.
4. **Scoring**: `LOW`, prioridad `4`.

Este es el caso que el enunciado exige **no** tratar como anomalía real: el
sistema lo resuelve por precedencia de eventos, no por una heurística de
magnitud.

### M-109 → `REAL_ANOMALY` / `HIGH`

Pico fuerte desde `2026-09-12 14:00`, con deriva eléctrica acompañante. El evento
existe pero es `UNKNOWN`.

1. **Detector**: `CONSUMPTION_SPIKE`. **La rama de pico gana antes del chequeo
   eléctrico**, así que el factor de potencia bajo posterior al evento **no**
   degrada el caso a calidad de datos. Esto es exactamente lo que el enunciado
   pide: un aumento real con cambios eléctricos es anomalía real, no problema de
   medición.
2. **Correlación**: el evento matchea por ventana, pero `UNKNOWN` **no explica**
   → `Explains = false`.
3. **Clasificador**: sin evento explicativo → regla 4 → `REAL_ANOMALY`, estado
   `unexplained`.
4. **Scoring**: `HIGH`, **prioridad 1** (lo más urgente), y con `|Δ| >= 1.2` la
   confianza toca el techo: `0.97`.

### M-112 → `DATA_QUALITY` / `HIGH`

Desde `2026-09-13 00:00`, el consumo se mantiene **cerca de la mediana horaria**
pero las señales eléctricas se salen de banda (tensión sobre 231 V, después bajo
209 V, factor de potencia por debajo de 0.85). El evento existe: `DATA_QUALITY`.

1. **Detector**: ni el pico ni la caída disparan (el delta queda entre `-0.55` y
   `0.45`), así que entra a la **rama por defecto**: `electricalInconsistency`
   devuelve verdadero y `consumptionNearHourMedian` también → `DATA_QUALITY`.
2. **Correlación**: el evento `DATA_QUALITY` es explicativo → `Explains = true`.
3. **Clasificador**: el `Kind` es `KindDataQuality` → regla 1 → `DATA_QUALITY`.
4. **Scoring**: `HIGH`, prioridad `2`, confianza `0.90`.

**Resultado global**: el pipeline produce **exactamente 4** registros de
evidencia, uno por medidor, y **ningún otro medidor** genera evidencia. Eso está
fijado por el test de regresión del dataset.

Un detalle fino del payload al LLM: para M-112 el campo `has_operational_event`
es **`false`**, aunque `Explains` sea `true`. La razón es que un evento
`DATA_QUALITY` no pertenece al conjunto de eventos *operativos* que ese campo
considera (`internal/ai/payload.go`). Es coherente: un problema de medición no es
un evento de operación.

---

## 7. La capa de LLM

### Selección del proveedor

Una sola condición decide todo: si `LLM_API_KEY` está **vacío**, se usa el mock
determinístico; si tiene valor, se usa el proveedor real de Ollama. Se resuelve
en `ai.NewProvider(...)`, invocado desde `cmd/api/main.go`. La construcción es
offline y no puede entrar en pánico.

### El request real

| Aspecto | Valor |
|---|---|
| Endpoint | `POST {base_url}/api/chat` |
| Normalización | quita espacios y `/` final; si la base termina en `/api` agrega `/chat`, si no `/api/chat` (evita `/api/api/chat`) |
| Body | `{"model": <LLM_MODEL>, "messages":[{"role":"user","content":<prompt>}], "stream":false}` |
| Header | `Content-Type: application/json`, y `Authorization: Bearer <key>` **solo** si la key no está vacía |
| Timeout | 60 segundos |
| Respuesta | se lee `message.content`; contenido vacío o en blanco es error |

**No se manda `system`, ni `temperature`, ni `format`, ni schema.** El único
input que puede influir en el formato de salida es el texto del prompt.

### El prompt, verbatim

```
Analiza la siguiente anomalía eléctrica.

Datos:
{JSON}

Explica:
1. qué está ocurriendo
2. cuáles son las posibles causas
3. qué evidencia respalda cada hipótesis
4. qué debería revisar un operador
5. qué acciones recomienda

No inventes datos que no estén presentes.

Responde únicamente en español.
```

`{JSON}` se reemplaza por el payload. La última línea es una **instrucción
explícita de idioma**: el prompt ya estaba escrito en español, pero no *pedía*
español, así que el idioma de la respuesta dependía del modelo. Ahora es un
requisito declarado. Hay un test que lo exige y, además, que verifica que no se
haya reescrito ninguna línea previa.

### Qué se le manda al modelo

| Clave JSON | Valor |
|---|---|
| `meter_id` | identificador del medidor |
| `consumption_change_pct` | variación de consumo, redondeada a 1 decimal |
| `voltage_change_pct` | variación de tensión |
| `current_change_pct` | variación de corriente |
| `power_factor_change_pct` | variación de factor de potencia |
| `has_operational_event` | si hay un evento **operativo** explicativo |
| `anomaly_score` | la confianza calculada por el scorer |
| `classification` | el tipo de anomalía |
| `severity` | la severidad |

Es un payload **deliberadamente acotado**: el modelo recibe la clasificación y
las magnitudes ya calculadas, y se le pide explicarlas. No recibe las 4032
lecturas crudas ni la libertad de decidir si hay anomalía.

### Manejo de fallos

Todo error del proveedor devuelve `("", err)`: encode, red, timeout, respuesta
no-2xx (cita hasta 512 bytes del body), decode, contenido vacío. El orquestador
hace `continue`: **solo queda vacío `LLMText`**. Clasificación, severidad,
confianza, motivo y acción no se tocan. Es la garantía estructural de que el LLM
no puede degradar el análisis.

### El mock determinístico

`internal/analysis/llm.go` devuelve una narrativa fija en español por tipo de
anomalía, con los valores interpolados. Nunca falla. Se usa en tests y en modo
offline, y también está fijado por tests.

---

## 8. Arranque y concurrencia

**Secuencia de `main`**:

1. `godotenv.Load()` — tolera que no exista `.env`.
2. `config.Load()` — fatal si falla.
3. Se construyen loader, repositorios, todas las etapas y el proveedor de LLM.
4. `api.NewRouter(orchestrator)`.
5. **`orchestrator.Detect()` sincrónico** — fatal si falla. Carga los CSV,
   corre el pipeline determinístico y **publica** la evidencia.
6. **`go orchestrator.Enrich()`** en segundo plano — enriquece con el LLM.
7. `http.ListenAndServe`.

El orden importa: el punto 5 corre **antes** del 7, así que el puerto abre con la
evidencia determinística ya publicada. Y el punto 6 va **después**, así que el
puerto no espera al LLM. Antes de este diseño, el arranque tardaba ~69 segundos
con el proveedor real, y el frontend recibía `ERR_CONNECTION_REFUSED`.

**El modelo de concurrencia** es un solo `sync.RWMutex` con una regla explícita:

| Etapa | Qué hace con el lock |
|---|---|
| `Detect` | **corre todo el pipeline fuera del lock**; lo toma solo para publicar: `evidence = built; generation++` |
| `Enrich` | toma `RLock` para copiar la evidencia y capturar `generation`; **lo suelta**; llama al LLM sin lock; escribe cada narrativa con un lock corto |
| `Evidence()` | `RLock` y devuelve una **copia** |

`generation` es un contador monótono que se incrementa **en la misma sección
crítica** que publica la evidencia. `Enrich` escribe solo mientras la generación
siga siendo la suya:

- si otro `Detect` republicó evidencia, **aborta y descarta** sus narrativas,
  porque describen un snapshot superado;
- además corta el loop temprano para no gastar llamadas al LLM al pedo.

El comentario del código lo dice explícitamente: **no colapsar ese lock de
vuelta alrededor del loop del LLM**, porque reintroduce la ventana muerta de
arranque.

**Por qué la generación y no la identidad**: dos snapshots consecutivos pueden
tener la **misma** anomalía (mismo `meter_id` + timestamp, o sea el mismo id
compuesto) pero con campos determinísticos distintos, porque `Detect` relee los
CSV desde disco. Con un chequeo de identidad, un `Enrich` viejo pisaría la
narrativa nueva con números que ya no coinciden. La generación es el invariante
más fuerte, y hay tests de regresión para ambos casos.

---

## 9. Superficie HTTP

Todos los endpoints viven bajo el prefijo **`/api`**. No hay `/api/v1` ni alias
sin prefijo: fue una decisión explícita y hay un test que lo protege. El
`ServeMux` se registra con patrones de ruta y **no exige método HTTP**, así que
un `POST /api/health` también responde 200.

CORS: `Access-Control-Allow-Origin: *`, métodos `GET, POST, OPTIONS`, headers
`Content-Type, Authorization`. Un `OPTIONS` responde 200 y corta.

| # | Ruta | Para qué |
|---|---|---|
| 1 | `GET /api/health` | liveness: devuelve `{"status":"ok"}` |
| 2 | `GET /api/reports` | evidencia cruda, con la forma del modelo de dominio |
| 3 | `GET /api/meters` | array pelado de ids de medidor |
| 4 | `GET /api/meters/{meterId}` | metadata del medidor; 404 si no existe |
| 5 | `GET /api/meters/{meterId}/readings` | lecturas, con `from`/`to` RFC3339 opcionales |
| 6 | `GET /api/anomalies` | array de anomalías ordenado por prioridad |
| 7 | `GET /api/anomalies/{id}` | una anomalía por id compuesto `meterID-<RFC3339 UTC>` |
| 8 | `POST /api/ai/analyze` | re-corre el pipeline y guarda un snapshot bajo un UUID |
| 9 | `GET /api/ai/analysis/{id}` | el snapshot guardado, con `status: "completed"` |
| 10 | `GET /api/dashboard/summary` | contadores agregados del dashboard |

Notas de contrato que conviene conocer:

- El id de anomalía es **compuesto**: `<meter_id>-<detected_at en RFC3339 UTC>`.
  El listado y el detalle coinciden exactamente.
- `POST /api/ai/analyze` **no lee body** y es **sincrónico**: corre `Run()` (que
  es `Detect` + `Enrich`) y responde con las narrativas ya pobladas. Tarda entre
  uno y dos minutos y medio con el proveedor real. Es una decisión de UX
  deliberada y el frontend le informa la latencia al usuario; por eso **no** hay
  estado pendiente que pollear.
- `GET /api/dashboard/summary` **siempre** incluye `unvalidatedMeters`, también
  cuando no hay ninguno:
  `{"count": 0, "meters": [], "reason": "no hay suficiente información para validar: se requieren al menos 2 lecturas"}`.
  `count` es la cantidad de medidores que pasaron el control de calidad pero no
  tienen línea base por tener menos de 2 lecturas; `meters` es un `[]string` en
  orden de id que **siempre** serializa como `[]` y **nunca** como `null`;
  `reason` es el texto fijo del contrato. Esos medidores **no** aparecen en
  `/api/anomalies`, y eso es deliberado: una falta de datos **no** es una
  anomalía, así que no se inventa un `type`, un `Kind` ni una severidad para
  ellos. La clave está presente incluso con `count` en `0`, para que la forma de
  la respuesta sea estable.
- `/api/reports` serializa el modelo de dominio `models.Evidence`, no el DTO. Sus
  claves JSON son por lo tanto nombres de campo Go (`MeterID`, `Baseline`), a
  diferencia de `/api/anomalies`, que usa los DTO con claves en snake_case.
- Los **tokens de contrato** quedan en inglés y no deben traducirse: `type`
  (`REAL_ANOMALY`, `EXPLAINABLE_ANOMALY`, `FALSE_POSITIVE`, `DATA_QUALITY`),
  `severity` (`LOW`, `MEDIUM`, `HIGH`), `status` (`explained`, `unexplained`) y
  `status: "completed"` del análisis. El frontend los mapea a sus etiquetas.

Ver `docs/endpoints.md` para el contrato detallado de cada endpoint.

---

## 10. Configuración y datos

### Configuración

`internal/config/config.go`, con Viper. Los defaults y los bindings están en la
tabla de §2. Si `config.yaml` no existe, `Load()` devuelve error: el archivo es
**de facto obligatorio**.

### Dataset

`data/readings.csv` — encabezado
`meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor,status`:

- **4032 filas** de datos = 12 medidores × 14 días × 24 horas;
- medidores `M-101` … `M-112`;
- rango `2026-09-01 00:00:00` → `2026-09-14 23:00:00`;
- todas las filas con `status=OK`.

`data/events.csv` — encabezado
`meter_id,event_timestamp,event_type,description`, con **4 filas**:

| Medidor | Timestamp | Tipo | Descripción |
|---|---|---|---|
| M-104 | `2026-09-11 00:00` | `OPERATIONAL_CHANGE` | Nueva línea de producción activada |
| M-106 | `2026-09-08 00:00` | `SCHEDULED_OUTAGE` | Parada programada de mantenimiento de 12 horas |
| M-109 | `2026-09-12 14:00` | `UNKNOWN` | Sin evento operativo reportado |
| M-112 | `2026-09-13 00:00` | `DATA_QUALITY` | Lecturas intermitentes y saltos eléctricos anómalos |

### El loader

Es **tolerante a propósito**. Acepta alias de columna (`consumption` o
`consumption_kwh`, `voltage` o `voltage_v`, `current` o `current_a`), tolera
filas cortas siempre que las columnas requeridas existan (`FieldsPerRecord = -1`),
y para eventos acepta el id desde `event_id` o `meter_id` y el timestamp desde
`event_timestamp`, `timestamp` o `start_time`. Un valor malformado produce error
con el número de fila.

Los eventos se cargan como **puntuales**: `Start = End = timestamp`. Por eso la
ventana de correlación de 12 horas hacia adelante es la que hace el trabajo real.

---

## 11. Estrategia de tests

La suite no se apoya en un solo test de integración: combina seis enfoques.

1. **Unitarios por etapa** con fixtures construidos a mano: aritmética de la
   línea base, la precondición de las 2 lecturas (un medidor con una sola lectura
   no recibe entrada en el mapa de líneas base, y ninguna línea base producida
   puede tener un desvío no finito), el pico por sigma con un fixture concreto,
   los cambios porcentuales por señal y el caso `mean == 0` devolviendo
   exactamente `0`.
2. **Pines de texto exacto**: el motivo, la acción y las descripciones
   determinísticas, las narrativas del mock y los cuerpos de error 404 se
   comparan por igualdad de string completo, no por `contains`. Así un cambio de
   redacción no pasa silencioso.
3. **Contrato del LLM**: método, ruta, header de autorización (presente y
   ausente), `model`, `stream`, el mensaje único de rol `user`, substrings del
   prompt, la normalización de la base URL en cuatro formas y el payload JSON
   exacto.
4. **Concurrencia, bajo `-race`**: que `Evidence()` no se bloquee durante el
   enriquecimiento, que `Detect` nunca llame al LLM, la publicación progresiva
   ítem por ítem, la tolerancia a un LLM nulo, y las dos regresiones de escritura
   sobre un snapshot superado.
5. **Regresión del dataset**: corre el pipeline **real** sobre los CSV reales y
   exige exactamente 4 registros, con el tipo y la severidad de los 4 medidores
   benchmark, `confidence > 0.90` para M-109, todos los campos de texto no vacíos,
   línea base no trivial, y **ningún otro medidor** con evidencia.
6. **Contrato HTTP de los medidores no validados**: un fixture hermético con un
   medidor de una sola lectura deja fijo que `/api/anomalies` sigue siendo
   decodable y que ese medidor **no** aparece ahí, y que `Detect` lo reporta como
   falta de datos sin producir evidencia para él. El resumen del dashboard expone
   `unvalidatedMeters` con el conteo, el arreglo de ids y la razón exacta,
   también cuando `count` es `0` (con `meters` como `[]`, nunca `null`).

Correr la suite:

```sh
go test ./...
go test -race ./...
```

El detector de carreras está disponible en este entorno (requiere `gcc`/`clang`).

---

## 12. Lo que demuestra este proyecto

Para quien lo evalúe, los puntos fuertes están en el diseño, no en el volumen:

- **Separación estricta entre determinismo e IA.** El LLM recibe un payload ya
  clasificado y no puede alterar la decisión; un fallo suyo deja un campo vacío y
  nada más. Es la garantía que el enunciado pide, y está impuesta por estructura,
  no por disciplina.
- **Un dataset que prueba comportamiento, no solo que el código corra.** Los
  cuatro medidores ejercitan las cuatro clasificaciones, incluidas las dos
  trampas clásicas: un evento programado que **no** debe reportarse como anomalía
  real (M-106) y una medición inconsistente con consumo estable que **no** debe
  reportarse como anomalía real (M-112).
- **Precedencia explícita.** El orden de las reglas del detector y del
  clasificador está escrito y justificado. Las decisiones difíciles —qué gana
  cuando dos reglas aplican— están resueltas y cubiertas por tests.
- **Concurrencia con invariante nombrado.** El lock no se mantiene sobre la
  llamada externa, y la escritura diferida se protege por generación de snapshot.
  Los dos bugs de escritura cruzada que aparecieron en el camino están fijados con
  tests de regresión reproducibles.
- **Trazabilidad.** El directorio `odd/` registra cada feature trabajada: el
  problema medido, las decisiones tomadas, los commits y la evidencia de
  verificación.

---

## 13. Limitaciones y deudas conocidas

Se listan porque son reales y están verificadas, no por completitud formal.

**En el código:**

- **Valores no finitos siguen entrando por la puerta de los datos**:
  `internal/data/csv/loader.go` parsea los números con `strconv.ParseFloat`, que
  acepta las cadenas `NaN`, `Inf` e `Infinity`, y `internal/analysis/quality.go`
  solo descarta consumo negativo y status distinto de `OK`. Una lectura así
  vuelve a producir una línea base no finita: la precondición de las 2 lecturas
  no cubre este caso. En la misma familia, un `varSum` que desborde a `+Inf` con
  entradas **finitas** enormes (por ejemplo `1e200`) da un desvío infinito;
  estructuralmente abierto, aunque impracticable con datos de medidores reales.
- **Los handlers descartan el error de `json.Marshal`**: `writeJSON`
  (`internal/api/handlers/endpoints.go:206`) y el handler de `/api/reports`
  (`internal/api/router.go:112`) ignoran el error de `Encode`, así que cualquier
  valor no finito que llegue al payload responde **200 con cuerpo vacío** en vez
  de fallar ruidosamente con un 500. Es el mismo modo de falla que disparaba el
  `NaN` de una sola lectura, ahora alcanzable por la vía de los datos.
- **`SERVER_PORT`**: el comentario de `config.go` documenta esa variable con
  default 8080, pero el código usa 3001 y nunca registra el binding. El puerto
  solo se cambia por `config.yaml`.
- **Prosa del clasificador vs. signo**: las plantillas de `REAL_ANOMALY` y
  `EXPLAINABLE_ANOMALY` dicen siempre "por encima de", incluso para una anomalía
  originada en una **caída**. Con el dataset actual esos tipos solo surgen de
  picos, así que la inconsistencia es latente y no observable.
- **`/api/reports` expone el modelo de dominio** con claves en inglés y valores
  de enum en inglés (`CONSUMPTION_SPIKE`). Son claves y tokens, no prosa; pero un
  consumidor de ese endpoint ve una forma distinta a la de `/api/anomalies`.

**En la documentación:**

- **`openspec/` está desactualizado**: nombra chi y directorios que no existen
  (`internal/ai/agents/`, `internal/api/routes/`). La spec sigue en
  `status: draft`.
- **`docs/backend-implementation-plan.md` especifica `/api/v1/*`**, rutas que no
  existen y que el código rechaza a propósito. (Ojo: el token `/api/v1` aparece
  en este plan, **no** en `openspec/`.)
- **`docs/routing.md` muestra una firma vieja** de `NewRouter` con un parámetro
  `port` que ya no existe.
- **`docs/endpoints.md` sub-documenta el DTO de anomalía**: el ejemplo muestra 10
  campos y el código emite 18 (faltan `priority`, `baseline`, los cuatro
  `*_change_pct`, `correlated_events` y `data_quality`).

**En el arranque:**

- **El arranque no bloquea, pero el puerto no tiene señal de readiness**: el
  `GET /api/health` responde 200 desde el primer instante. Durante el
  enriquecimiento, `llm_analysis` puede estar **ausente** del JSON (el campo es
  `omitempty`, así que se omite la clave entera y no se emite `""`), mientras el
  resto del payload ya es final y correcto.
- **Un modelo en vivo no es determinista**: la instrucción de idioma del prompt
  eleva la probabilidad de respuesta en español, pero no la garantiza contra otro
  modelo, otro proveedor u otra configuración.
