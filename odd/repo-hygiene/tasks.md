# Repo hygiene — config única y limpieza de código muerto

**Origen:** riesgos residuales R3 (código muerto) y R4 (deriva de claves de config) detectados por la verificación independiente del plan `backend-requerimientos-compliance.md`.
**Estado:** pendiente de implementación con subagentes.
**Fuera de alcance (decisión del usuario):** el provider real de LLM queda pendiente hasta que se configure `LLM_API_KEY`.

---

## T21 — Config: una sola fuente de verdad para los paths de CSV

### Problema medido
- `config.yaml` define `data.csv_path` y `data.events_path`.
- `internal/config/config.go` lee `data.readings_csv` y `data.events_csv` (defaults y `BindEnv` sobre esas claves).
- Resultado: **las claves del YAML se ignoran en silencio** y ganan siempre los defaults. Hay dos nombres para lo mismo.

### Decisión
Canonicalizar en las claves que ya usa el código: `data.readings_csv` y `data.events_csv`. Actualizar `config.yaml` a esas claves. **No** dejar un fallback que acepte el nombre viejo: eso volvería a crear dos fuentes de verdad.

### Criterios de aceptación
1. `config.yaml` usa exactamente `data.readings_csv` y `data.events_csv`.
2. Un test nuevo prueba que **el valor del YAML se aplica realmente** (hoy nada lo prueba).
3. Un test nuevo prueba que la variable de entorno (`READINGS_CSV` / `EVENTS_CSV`) **pisa** el valor del YAML.
4. Un test prueba los defaults cuando no hay YAML ni env.
5. `TestLoad_NoConfigFileAndMissingEnv` sigue pasando.

### Nota de diseño (verificar, no asumir)
`Load()` combina `AddConfigPath(".")`/`AddConfigPath("../")` con `SetConfigFile("config.yaml")`. Con `SetConfigFile` un archivo ausente **no** produce `viper.ConfigFileNotFoundError` sino un error de path, y por eso el test actual pasa: está midiendo el comportamiento de archivo ausente, no el de env faltante. **No** cambiar esa semántica en esta tarea; sólo reportarla.

---

## T22 — Limpieza de código muerto

### Alcance verificado (grep de importadores)

| Elemento | Estado |
|---|---|
| `internal/domain/services/` (16 archivos, incluye sus tests) | ningún importador |
| `internal/data/csv_loader.go` | sólo importado por `internal/domain/services/csv_loader_wrapper.go` (muerto) |
| `internal/logger/` | ningún importador |
| `internal/ai/mock_client.go` (`MockLLMProvider`) | sin referencias |
| `handlers.Readings`, `handlers.AnomalyDetail`, `handlers.Health` | sin referencias (el router usa `handlers.AnomalyDetailByID`) |
| parámetro `port int` de `api.NewRouter` | sin uso dentro; callers: `cmd/api/main.go`, `internal/api/api_test.go` |
| `github.com/rs/zerolog` + `replace => ./stub/zerolog` | sólo lo usaba `internal/logger` |

### Criterios de aceptación
1. Los paquetes/archivos muertos se eliminan y `go build ./...` sigue verde.
2. Cada borrado se justifica con el grep que demuestra que no hay importadores (no se borra nada "porque parece").
3. `api.NewRouter` pierde el parámetro sin uso y **todos** sus callers se actualizan.
4. `go test -count=1 ./...` sigue verde.
5. Si tras borrar `internal/logger` nada importa zerolog: intentar quitar el `require` y el `replace` y correr `go mod tidy`. **Si `go mod tidy` no puede correr offline, revertir sólo el cambio de `go.mod` y dejar `stub/zerolog` en su lugar** (nunca dejar el `replace` apuntando a un directorio borrado).
6. No borrar nada que esté referenciado; el paquete `internal/domain/services` es un duplicado legacy del pipeline, no una API pública.

### Restricciones
- No cambiar comportamiento observable de la API ni del pipeline.
- No tocar `internal/analysis/` (salvo que un borrado lo requiera y se justifique) ni `docs/`.

---

## Verificación
- `go build ./...` y `go test -count=1 ./...` verdes tras cada tarea.
- Los 4 casos del dataset (`M-104` EXPLAINABLE/MEDIUM, `M-106` FALSE_POSITIVE/LOW, `M-109` REAL_ANOMALY/HIGH >0.90, `M-112` DATA_QUALITY/HIGH) siguen intactos.
- `go vet ./...` sin hallazgos nuevos.

---

## Estado de ejecución — COMPLETADO

### T21 — Config canónica ✅
- `config.yaml` reescrito a `data.readings_csv` / `data.events_csv` (única fuente de verdad).
- `internal/config/config.go` no necesitó cambios: ya usaba las claves canónicas.
- Tests nuevos: `TestLoad_ReadsCSVPathsFromConfigFile` (prueba que el YAML se aplica, con valores distintivos), `TestLoad_EnvOverridesCSVPathsFromConfigFile` (el env pisa al YAML), `TestLoad_DefaultsWhenNoYAMLAndNoEnv`.
- `TestLoad_NoConfigFileAndMissingEnv` sigue pasando sin cambios.

### T22 — Limpieza de código muerto ✅
Borrado (21 archivos, cada uno justificado con grep de importadores):
- `internal/domain/services/` completo (16 archivos, duplicado legacy sin importadores).
- `internal/logger/logger.go` (único consumidor de zerolog).
- `internal/data/csv_loader.go` (sólo lo usaba el wrapper muerto; desaparece el paquete `internal/data`).
- `internal/ai/mock_client.go` (`MockLLMProvider` sin referencias).
- `handlers.Readings` y `handlers.AnomalyDetail` de `internal/api/handlers/endpoints.go`.
- `stub/zerolog/go.mod` + `stub/zerolog/zerolog.go`, y de `go.mod` el `require github.com/rs/zerolog` + el `replace => ./stub/zerolog` (`go mod tidy` OK, `go.sum` byte-idéntico).

Modificado:
- `api.NewRouter` perdió el parámetro `port int` sin uso; callers actualizados en `cmd/api/main.go` y `internal/api/api_test.go`.

Divergencia intencional (corrección de un error del reporte de verificación):
- **`handlers.Health` NO se borró**: el verificador lo había marcado como muerto, pero `internal/api/handlers/handlers_test.go` lo invoca desde `TestHealthHandler`. Borrarlo habría roto `go test ./...`. Lección metodológica: la búsqueda de código muerto debe incluir los `_test.go`. Micro-limpieza opcional pendiente de decisión del usuario: borrar `Health` + `TestHealthHandler` juntos.

### Verificación final
- `go build ./...` → exit 0
- `go vet ./...` → exit 0
- `go test -count=1 ./...` → suite completa verde
- `TestRequirementsDatasetOutcomes` → PASS: M-104 EXPLAINABLE_ANOMALY/MEDIUM 0.8297 · M-106 FALSE_POSITIVE/LOW 0.7932 · M-109 REAL_ANOMALY/HIGH 0.9700 · M-112 DATA_QUALITY/HIGH 0.9000

### Pendiente / fuera de alcance
- **T23 Provider real de LLM**: bloqueado por decisión del usuario hasta configurar `LLM_API_KEY`.
- Directorio `stub/` quedó vacío (sin archivos).
- `cmd/api/main.go` lleva un reformat gofmt sólo-espacios (ya estaba fuera de gofmt en el baseline).
- **Ningún commit fue creado**: la política de seguridad prohíbe commitear sin pedido explícito. El repo está mayormente untracked, por lo que los borrados no son recuperables vía git.
