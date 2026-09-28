# Integración del LLM real (gpt-oss:20b)

**Origen:** T23 del plan original (`T23 Provider real de LLM`), ahora desbloqueada porque el usuario configuró `LLM_API_KEY` (57 chars en `.env`).
**Decisión del usuario sobre el endpoint:** **Ollama remoto usando la key**. Se implementa contra la API nativa de Ollama con `Authorization: Bearer`, y la base URL queda **configurable** con default `https://ollama.com` para no depender de un host fijo.
**Contrato verificado:** `POST /api/chat` con `{"model":"<model:tag>","messages":[{"role":"user","content":"..."}]}`; con `"stream": false` devuelve un único objeto JSON con `message.content`. Fuente: doc oficial de Ollama (`docs/api.md`).
**Fuera de alcance:** cambiar la clasificación determinista. El LLM **sigue siendo sólo la capa de narración/interpretación**; nunca decide tipo, severidad ni confianza.

---

## Hallazgo que condiciona el trabajo

El JSON que pide el usuario necesita cambios porcentuales de **voltaje, corriente y factor de potencia**, y hoy **no existen**:

- `models.Baseline` guarda sólo `Mean`, `StdDev` y `Count` **de consumo**.
- `baselineCalculator.Calculate` sólo acumula `r.Consumption`.
- El detector calcula **un solo** `Delta` (consumo).

Por eso el payload requiere una tarea previa de enriquecimiento estadístico (T23), antes del provider (T24).

**Nota sobre los números del ejemplo:** el JSON que dio el usuario es ilustrativo (`consumption_change_pct: 103.7` para M-109). Nuestra medición real del pico detectado es ~125.3% sobre la media del baseline. Se mantiene la semántica ya existente (cambio relativo contra la media del baseline del medidor) y **no** se ajusta el detector para imitar el ejemplo.

---

## T23 — Payload estadístico enriquecido

Construir, para cada anomalía, este objeto exacto (claves y semántica pedidas por el usuario):

```json
{
  "meter_id": "M-109",
  "consumption_change_pct": 103.7,
  "voltage_change_pct": -4.2,
  "current_change_pct": 31.8,
  "power_factor_change_pct": -12.4,
  "has_operational_event": false,
  "anomaly_score": 0.96,
  "classification": "REAL_ANOMALY",
  "severity": "HIGH"
}
```

### Criterios de aceptación
1. **Baselines por señal**: el baseline por medidor incluye además la media de voltaje, corriente y factor de potencia (además de las de consumo que ya usa el detector). El consumo debe conservar `Mean`/`StdDev`/`Count` sin cambios para no alterar la detección.
2. **Cambios porcentuales**: al emitir un candidato, el detector calcula y guarda los cuatro cambios porcentuales **con signo** contra la media del baseline del medidor.
3. **División por cero**: si la media de una señal es 0, el cambio de esa señal queda en `0` (no `NaN`, no `+Inf`). Debe quedar documentado en el código.
4. **`has_operational_event`** refleja **sólo eventos que explican** (`Correlation.Explains`). Para **M-109 debe ser `false`** (su evento es `UNKNOWN`), para M-104 `true`, para M-106 `true`.
5. **`anomaly_score` = `Confidence`**, **`classification` = `Type`**, **`severity` = `Severity`**.
6. El builder del payload es una función testeable que toma `models.Evidence` y devuelve el objeto/JSON.
7. Test obligatorio: correr el pipeline real sobre `data/` y verificar el payload de **M-109** (has_operational_event false, classification REAL_ANOMALY, severity HIGH, consumption_change_pct > 100) y de **M-112** (DATA_QUALITY).
8. Los 4 casos del dataset siguen intactos (M-104 EXPLAINABLE/MEDIUM, M-106 FALSE_POSITIVE/LOW, M-109 REAL_ANOMALY/HIGH >0.90, M-112 DATA_QUALITY/HIGH).

---

## T24 — Provider real contra Ollama `/api/chat`

### Criterios de aceptación
1. **Config**: base URL y modelo configurables — `LLM_BASE_URL` (default `https://ollama.com`) y `LLM_MODEL` (default `gpt-oss:20b`), además del `LLM_API_KEY` existente. Reflejados en `config.yaml` y en los comentarios de `internal/config/config.go`.
2. **Request**: `POST {LLM_BASE_URL}/api/chat` con cuerpo `{"model": <LLM_MODEL>, "messages": [{"role":"user","content": <prompt>}], "stream": false}` y header `Authorization: Bearer <LLM_API_KEY>` cuando la key no está vacía.
3. **Prompt verbatim** (el usuario lo especificó; se usa tal cual, en español), con el JSON del payload interpolado en `{JSON}`:
   `Analiza la siguiente anomalía eléctrica.` + `Datos:` + `{JSON}` + los 5 puntos numerados (`qué está ocurriendo`, `cuáles son las posibles causas`, `qué evidencia respalda cada hipótesis`, `qué debería revisar un operador`, `qué acciones recomienda`) + `No inventes datos que no estén presentes.`
4. **Response**: parsea `message.content` del JSON de respuesta **no streaming**.
5. **Resiliencia**: timeout explícito (context), errores envueltos y accionables (status HTTP no-2xx, body ilegible, JSON inválido, content vacío). **Nunca panic**. Un fallo del LLM **no** cambia clasificación, severidad, confianza ni el texto determinista: sólo deja `LLMText` vacío (comportamiento ya existente del orquestador).
6. **Testeable sin red**: la base URL y el `*http.Client` son inyectables; los tests usan `httptest` y verifican **la forma exacta del request** (método, path, header Authorization, `model`, `stream: false`, que el prompt contiene el JSON y los 5 puntos) y el parseo de la respuesta.
7. Paths de error testeados: status 500, body no-JSON, `message.content` vacío.
8. `NewProvider` pasa a recibir también base URL y modelo; se actualizan `cmd/api/main.go` y los tests del factory.

---

## T25 — Exponer el análisis del LLM en la API + docs

Hoy `Evidence.LLMText` se guarda pero **no se expone**: desde el frontend la integración sería invisible.

### Criterios de aceptación
1. `AnomalyDTO` suma el campo `llm_analysis` (string, `omitempty`) mapeado desde `Evidence.LLMText`.
2. Se conserva `reason` con el texto **determinista** (el LLM no lo reemplaza): el motivo de la clasificación sigue viniendo del pipeline.
3. `docs/endpoints.md` documenta el campo nuevo y aclara que puede venir vacío si el LLM falla o no está configurado.
4. Test: el DTO incluye `llm_analysis` cuando la evidencia trae `LLMText`.

---

## Orden de ejecución y dependencias

`T23` (payload) → `T24` (provider) → `T25` (API + docs).

- T24 consume el payload de T23.
- T25 sólo depende de que exista `LLMText`, que ya existe; se hace al final para no tocar `internal/api` mientras corre T24.
- Un writer a la vez: T23 y T24 comparten `internal/ai`.

## Verificación final
- `go build ./...`, `go vet ./...`, `go test -count=1 ./...` verdes.
- Los 4 casos del dataset intactos.
- Trazabilidad: cada tarea se guarda en Engram al terminar.
- **No se commitea ni se pushea sin autorización explícita.**

---

## Estado de ejecución — COMPLETADO (T23, T24, T25)

Ejecutado con subagentes `gentle-ai-worker` (uno por tarea, secuencial porque T23 y T24 comparten `internal/ai`), con verificación en vivo hecha por el orquestador.

### Evidencia

- `go build ./...`, `go vet ./...` y `go test -count=1 ./...` verdes.
- `TestRequirementsDatasetOutcomes` PASS: M-104 EXPLAINABLE_ANOMALY/MEDIUM 0.8297 · M-106 FALSE_POSITIVE/LOW 0.7932 · M-109 REAL_ANOMALY/HIGH 0.9700 · M-112 DATA_QUALITY/HIGH 0.9000.
- **Contra el servicio real**: `POST https://ollama.com/api/chat` con la key del `.env` → HTTP 200, body con `message.content`, `done: true`, `model: gpt-oss:20b`.
- **End-to-end**: binario levantado en el puerto 3007 (se respetó el server del usuario en 3001) con config temporal en `/tmp`; `GET /api/anomalies` devolvió los 4 casos con `llm_analysis` generado por el modelo real, en la estructura de los 5 puntos, y con `reason`/`recommended_action` deterministas intactos.

### Payload real producido (los números del ejemplo eran ilustrativos)

- M-109: `{"meter_id":"M-109","consumption_change_pct":125.3,"voltage_change_pct":-2.7,"current_change_pct":111.2,"power_factor_change_pct":-18.2,"has_operational_event":false,"anomaly_score":0.97,"classification":"REAL_ANOMALY","severity":"HIGH"}`
- M-112: `{"meter_id":"M-112","consumption_change_pct":-26.4,"voltage_change_pct":8.4,"current_change_pct":28.8,"power_factor_change_pct":-23.4,"has_operational_event":false,"anomaly_score":0.9,"classification":"DATA_QUALITY","severity":"HIGH"}`

### Hallazgos operativos (importantes)

1. **`.env` usa rutas CSV relativas** (`./data/...`) y las env vars pisan al YAML: el binario **debe correrse desde la raíz del repo**. Desde otro cwd falla con `cannot find the path specified`.
2. El arranque del server tarda ~88 s porque `Run()` hace **4 llamadas reales al modelo** (~20 s cada una) antes de escuchar. Si eso molesta, conviene un timeout menor o llamadas en paralelo (hoy son secuenciales).
   > **Superado.** Este hallazgo ya no aplica: el arranque no hace ninguna llamada
   > al modelo. `cmd/api/main.go` corre solo `Detect()` antes de `ListenAndServe`;
   > la narrativa del LLM corre bajo demanda, por medidor, desde
   > `POST /api/ai/analyze`. El cuerpo se conserva como registro.
3. El modelo devuelve **markdown** (tablas y encabezados) en `llm_analysis`; el frontend debe renderizarlo como tal.
4. `has_operational_event` refleja **eventos operativos**, no `Correlation.Explains`: para M-112 (evento `DATA_QUALITY`) es `false` aunque su `status` sea `explained`.

### Pendiente / fuera de alcance

- El server que estaba en el puerto 3001 quedó caído durante la verificación (era un proceso leftover de sesiones anteriores de agentes, con código viejo). No afecta al repo; hay que relanzarlo desde la raíz del repo.
- **Ningún commit fue creado** en esta tanda (T23–T25): hace falta autorización explícita.
