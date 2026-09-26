# Repo layout cleanup — rename `assets/` y consolidación del layout ODD

**Origen:** revisión de estado del backend solicitada por el usuario (build/vet/test verdes, layout cuestionado).
**Estado:** implementación autorizada por el usuario.
**Decisiones del usuario (no re-litigar):**
1. `assets/` → `data/` para los CSV.
2. Los `.md` que viven en `assets/` se mueven a `docs/`.
3. `cover.out` se ignora/borra: no ensucia el árbol de trabajo.
4. El layout de ODD queda **solo** en `odd/<feature>/tasks.md`; se elimina `odd/tasks/`.

---

## Contexto medido (no asumido)

`assets/` contenía 4 archivos con tres responsabilidades mezcladas:

| Archivo | Rol real |
|---|---|
| `readings.csv` (4032 filas) | Dataset de runtime **y** fixture de integración |
| `events.csv` (4 filas) | Dataset de runtime **y** fixture de integración |
| `Requerimientos.md` | Documentación (enunciado de la prueba técnica) |
| `backend_implementation_plan.md` | Documentación (plan de implementación) |

Referencias medidas a `assets` (13 archivos tracked):

- Código (8): `internal/config/config.go`, `internal/config/config_test.go`, `internal/data/csv/loader.go`,
  `internal/data/csv/loader_test.go`, `internal/analysis/correlator.go`,
  `internal/analysis/requirements_dataset_test.go`, `internal/api/api_test.go`, `internal/ai/payload_test.go`
- Config (1): `config.yaml`
- ODD/OpenSpec (4): 3 docs de `odd/tasks/` + `openspec/specs/backend_implementation.sdd.yaml`

Nota: los tests usan rutas relativas a la altura del paquete (`..`, `..`, `..`). El rename **no cambia la
profundidad**, así que solo se reemplaza el segmento `"assets"` por `"data"`; no se recalcula ningún `..`.

---

## T1 — Consolidar el layout de ODD en `odd/<feature>/tasks.md`

### Alcance
Mover cada `odd/tasks/<feature>.md` a `odd/<feature>/tasks.md` y eliminar `odd/tasks/`.

| Origen | Destino | Conflicto |
|---|---|---|
| `odd/tasks/ai-endpoints.md` | `odd/ai-endpoints/tasks.md` | — |
| `odd/tasks/backend-requerimientos-compliance.md` | `odd/backend-requerimientos-compliance/tasks.md` | — |
| `odd/tasks/backend-requirements-gap.md` | `odd/backend-requirements-gap/tasks.md` | — |
| `odd/tasks/create-backend-sdd-spec.md` | `odd/create-backend-sdd-spec/tasks.md` | destino ya existía, sin heading |
| `odd/tasks/fix-compile-errors.md` | `odd/fix-compile-errors/tasks.md` | destino ya existía, sin heading |
| `odd/tasks/llm-integration.md` | `odd/llm-integration/tasks.md` | — |
| `odd/tasks/repo-hygiene.md` | `odd/repo-hygiene/tasks.md` | — |
| `odd/tasks/review-backend-plan.md` | `odd/review-backend-plan/tasks.md` | — |

### Resolución de los dos conflictos
Los destinos preexistentes (`odd/create-backend-sdd-spec/tasks.md`, `odd/fix-compile-errors/tasks.md`) eran
el mismo cuerpo de bullets **sin** el `# <feature>` ni el `## Tasks`. El origen en `odd/tasks/` es un
superconjunto, así que el origen gana y el destino se sobrescribe sin pérdida de contenido.

### Verificación previa
`grep -rn "odd/tasks"` fuera de `odd/` → sin resultados. Nada del código, los docs ni OpenSpec depende de
esa ruta, por lo que la consolidación no rompe referencias.

### Criterios de aceptación
1. Existen exactamente 8 archivos `odd/<feature>/tasks.md` y ninguno perdió contenido respecto del origen.
2. `odd/tasks/` ya no existe.
3. `git ls-files odd` no lista ningún `odd/tasks/*`.
4. Ningún otro archivo del repo referencia `odd/tasks/`.

---

## T2 — Rename `assets/` → `data/` y barrido completo de referencias

### Alcance
1. **Mover datasets:** `assets/readings.csv` → `data/readings.csv`; `assets/events.csv` → `data/events.csv`.
2. **Mover documentación:** `assets/Requerimientos.md` → `docs/requerimientos.md`;
   `assets/backend_implementation_plan.md` → `docs/backend-implementation-plan.md`.
   Se pasa a minúsculas para respetar la convención ya presente en `docs/` (`endpoints.md`, `routing.md`).
3. **Eliminar** el directorio `assets/` una vez vacío, y el artefacto `cover.out` de la raíz.
4. **Actualizar las 13 referencias** medidas, incluidos los identificadores de test que nombran la carpeta
   vieja (`assetsReadingsPath`, `assetsEventsPath`, `newAssetsEnvironment` → `dataset*`), y los comentarios.
5. **No** duplicar los CSV en `testdata/`: sería crear una segunda fuente de verdad, contra la decisión ya
   registrada en `odd/repo-hygiene/tasks.md` (T21).

### Restricciones
- No cambiar comportamiento observable de la API ni del pipeline.
- No tocar `.git/gentle-ai/candidate-views/**` (copias internas de revisión, no son código fuente).
- No borrar `internal/data/` ni confundirlo con el nuevo `data/` de la raíz.

### Criterios de aceptación
1. `grep -rn "assets"` sobre `cmd internal config.yaml docs odd openspec README.md` → **0 resultados fuera de
   este documento de plan**. Este archivo conserva a propósito los nombres "from" (`assets/...`) porque es
   el registro de la mudanza; un `grep` que lo incluya no puede dar cero y no debe usarse como criterio.
   Criterio correcto: `grep -rn "assets" ... | grep -v '^odd/repo-layout-cleanup/'` → 0 resultados.
2. `go build ./...`, `go vet ./...` y `go test -count=1 ./...` en verde.
3. `config.yaml` apunta a `data/readings.csv` y `data/events.csv`, y coinciden con los defaults de
   `internal/config/config.go`.
4. No queda el directorio `assets/` ni el archivo `cover.out`.
5. Los 4 archivos movidos conservan su contenido byte a byte (mismo `git hash-object` antes y después).

---

## T3 — Verificación independiente: RESULTADO

Delegada a `gentle-ai-verify`. Los 7 claims se confirmaron, con estos resultados y correcciones:

| Claim | Veredicto | Evidencia |
|---|---|---|
| C1 build/vet/test verdes | CONFIRMADO | `go build`/`go vet` EXIT=0; `go test -count=1 ./...` verde |
| C2 cero referencias residuales | CONFIRMADO | post-filtro EXIT=1 (vacío); `assets/` y `cover.out` ausentes |
| C3 contenido byte a byte | CONFIRMADO | los 4 pares de `git hash-object` coinciden; readings 4033 líneas |
| C4 sin identificadores obsoletos | CONFIRMADO (con corrección) | 0 hits de `assets`/`Assets` en `cmd`/`internal` |
| C5 los paths resuelven | CONFIRMADO | 6 rutas de fixture verificadas, todas existen; ningún `t.Skip` |
| C6 coherencia de config | CONFIRMADO | claves YAML == claves del código; bindings de env intactos |
| C7 precedencia de env | CONFIRMADO | `TestLoad_EnvOverridesCSVPathsFromConfigFile` pasa |

### Corrección de C4 (el verificador tenía razón)
`assetsReadingsPath`, `assetsEventsPath` y `newAssetsEnvironment` **nunca existieron en un commit**. `git log -S` sobre
esos nombres no devuelve nada: `HEAD:internal/api/api_test.go` sólo tiene `newTestEnvironment`. Esos helpers son
código **nuevo** del trabajo LLM sin commitear, que yo renombré en el working tree antes de que existieran en git.
La afirmación correcta es "el working tree actual ya no los define", no "se renombraron identificadores versionados".

### Límite del alcance de C1 (corregido)
El prompt de verificación describió el delta como "rename mecánico sin cambio de comportamiento". Eso es **falso para
el árbol completo**: el working tree trae 1416 inserciones / 342 borrados en 24 archivos, incluyendo el trabajo de
integración LLM (campos nuevos en `reading.go`, `ai.NewProvider` con 3 argumentos, campo `llm_analysis` en la API).
Por lo tanto `go test` valida el delta mezclado, no el rename aislado.

**El rename queda probado igual**, pero por evidencia estructural independiente de la suite: C3 (los 4 blobs son
idénticos, así que ningún CSV cambió), C2 (cero referencias al path viejo) y C5 (toda ruta de fixture resuelve a un
archivo real). La suite sólo corrobora.

---

## Estado del índice: trampa detectada (NO commitear el índice como está)

`git mv` **auto-stagea**. Consecuencia medida: el índice tiene los 4 moves y la consolidación de ODD en stage,
mientras que 24 archivos con las ediciones de paths quedan sin stage.

Si se corre `git commit` sin argumentos ahora, el commit resultante contendría `data/readings.csv` **y** un
`config.yaml` que todavía apunta a `assets/readings.csv`. Sería un commit roto que falla en runtime.
Prohibido commitear el índice tal cual: usar `git add -A`, o rutas explícitas, en el momento del commit.

---

## Plan de commits (orden restringido)

El orden no es libre. Los archivos nuevos del trabajo LLM (`internal/ai/payload_test.go`, los helpers de
`internal/api/api_test.go`) ya escriben rutas `data/...`, así que **`data/` tiene que existir en el mismo commit o
antes** que ellos. Eso descarta poner el LLM primero con el path viejo.

Orden viable, cada commit verde:

1. **Consolidación del layout ODD** — sólo moves de `odd/tasks/*.md` → `odd/<feature>/tasks.md`.
2. **Rename `assets/` → `data/`** — los 4 moves + las ediciones de paths/comentarios que existen en HEAD
   (`config.yaml`, `config.go`, `config_test.go`, `loader.go`, `loader_test.go`, `correlator.go`,
   `requirements_dataset_test.go`, el comentario de `api_test.go`, `openspec`, docs de ODD).
3. **Integración LLM** — todo el resto, incluidos los archivos nuevos que ya dicen `data/`.

---

## Commits ejecutados (evidencia)

| # | SHA | Mensaje | Diff |
|---|---|---|---|
| 1 | `fc3fd37` | `chore(odd): consolidate task docs into odd/<feature>/tasks.md` | 10 archivos, +208/-19 |
| 2 | `403b32f` | `chore(repo): rename assets/ to data/ and move planning docs into docs/` | 13 archivos, +21/-21 |
| 3 | `a9067ac` | `feat(ai): narrate anomalies with the real Ollama provider` | 18 archivos, +1396/-322 |

Base: `11c9508`. Rama: `split-commits`.

### Verificación de integridad
El árbol final y el snapshot previo a la división tienen el **mismo tree hash** (`f72bc947535758b68115fa81e2224723ec3f08fd`):
la división no perdió ni alteró un solo byte. `go build`, `go vet` y `go test -count=1 ./...` verdes sobre el
tree committeado.

### Verificación del commit 2 en aislamiento
Se creó un `git worktree` temporal en el SHA del commit 2 (sin tocar el árbol principal) y allí
`go build`/`go vet`/`go test -count=1 ./...` dieron verde, con los CSV ya resueltos desde `data/`. Es la prueba
de que el commit 2 es autoconsistente y no depende del trabajo LLM.

---

## Desviación encontrada al ejecutar (no prevista en el plan)

El plan original del writer decía "requiere `git add -p` para separar hunks". Al ejecutar apareció que
**eso era imposible**: `internal/config/config.go` en HEAD está indentado con espacios y con el bloque `import`
malformado, y el trabajo LLM le aplicó un `gofmt` de archivo completo. Con un reformateo total, todo el archivo
es un solo hunk: no hay hunks que separar.

Solución aplicada, uniforme para los 5 archivos solapados (`config.yaml`, `config.go`, `config_test.go`,
`requirements_dataset_test.go`, `api_test.go`):

    git show <base>:<archivo> | sed -e 's|"assets"|"data"|g' -e 's|assets/|data/|g'

Es decir, la versión del commit 2 se **reconstruye** desde la base aplicando sólo la sustitución de paths, en vez
de recortar hunks. Ventaja: el commit 2 quedó en +21/-21 líneas, todas sustituciones, sin reformateo — mucho más
revisible de lo que habría sido un corte por hunks. El `gofmt` viaja en el commit 3, que es de donde vino.

Trampa del primer intento, para no repetirla: el `sed` de `assets/` no matchea `filepath.Join("..", "..", "assets", ...)`
porque ahí el token es `"assets"` sin barra. Hay que sustituir las dos formas.

### Red de seguridad usada
Antes de tocar el índice se creó la rama `wip-checkpoint` con el árbol completo. Durante la ejecución un
`git reset --hard` mal compuesto borró el árbol de trabajo y la recuperación desde esa rama fue inmediata y sin
pérdida. La rama sigue existiendo como red; se puede borrar con
`git branch -D wip-checkpoint` una vez conforme con los 3 commits.

---

## Pendiente abierto (fuera de las superficies autorizadas del writer)

### `.env` local queda apuntando al path viejo

Hallazgo de la implementación, verificado contra el código:

- `cmd/api/main.go:18` ejecuta `godotenv.Load()`.
- `internal/config/config.go` hace `v.BindEnv("data.readings_csv", "READINGS_CSV")` y
  `BindEnv("data.events_csv", "EVENTS_CSV")`.
- En viper, la variable de entorno **pisa** el valor de `config.yaml`.

El `.env` local (gitignored) define `READINGS_CSV=./assets/readings.csv` y `EVENTS_CSV=./assets/events.csv`,
así que **el binario corriendo con ese `.env` no encuentra los CSV**. El estado *committeado* es consistente
(`config.yaml` + defaults del código apuntan a `data/`), por lo que un clone limpio funciona; lo que queda
roto es sólo este archivo de estado local.

**No lo modificó el agente**: `.env` es estado local sensible y queda fuera de toda superficie de edición.

#### Recomendación
No repuntar las dos líneas a `./data/...`, sino **borrarlas** del `.env`. Duplicar los paths de CSV en dos
lugares es exactamente la doble fuente de verdad que `odd/repo-hygiene/tasks.md` (T21) ya decidió eliminar:
si `config.yaml` queda como fuente única, el próximo cambio de path se hace en un solo lugar y este bug no
puede repetirse.

---

## Nota de conflicto con el contrato del harness

El contrato ODD inyectado en el prompt del parent indica `odd/tasks/<feature-name>.md` como ruta canónica
(el paso 5 de ODD). La decisión del usuario (T1) la reemplaza por `odd/<feature>/tasks.md`. Este documento
vive en la ruta nueva justamente para dogfoodear la convención. Consecuencia conocida: una sesión futura
que arranque ODD puede volver a crear `odd/tasks/` si nadie recuerda esta decisión.
