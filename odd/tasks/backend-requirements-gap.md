## Requerimientos MVP vs Implementación Actual

- **Objetivo:** Verificar que el backend cumple con todos los requisitos del documento `assets/Requerimientos.md` y crear tareas para cubrir cualquier brecha.

### Brechas identificadas
1. **Importes no usados – chi**
   - `internal/api/handlers/endpoints.go` importa `github.com/go-chi/chi/v5` pero no lo utiliza en la ruta expuesta (router usa `http.ServeMux`). Mantener el import viola el requisito de *sin dependencias de routing*.
2. **Tests faltantes para eventos**
   - No hay pruebas unitarias que cubran la carga flexible de `events.csv` con timestamps sin segundos y con diferentes formatos.
3. **Encabezado `Content‑Type` en `/api/meters`**
   - El handler `Meters` no establece explícitamente `Content‑Type: application/json` antes de escribir la respuesta.
4. **Documentación en README**
   - El README no describe los endpoints actuales ni cómo ejecutar la API con los CSV de prueba.

### Tareas para alcanzar el MVP
| ID | Título | Descripción | Responsable (subagente) |
|----|--------|--------------|--------------------------|
| 1 | Eliminar import chi no usado | Modificar `internal/api/handlers/endpoints.go` para remover la importación de `github.com/go-chi/chi/v5` y actualizar cualquier referencia. | gentle-ai-worker |
| 2 | Añadir test de carga de eventos | Crear `internal/data/csv/loader_test.go` que verifique la correcta lectura de `events.csv` con timestamps en `"2006-01-02 15:04"` y RFC3339. | gentle-ai-worker |
| 3 | Establecer Content‑Type en `/api/meters` | En el handler `Meters` agregar `w.Header().Set("Content-Type", "application/json")` antes de codificar la respuesta. | gentle-ai-worker |
| 4 | Actualizar README con documentación de API | Incluir tabla de endpoints, ejemplos de curl y pasos para cargar `.env` y los CSV de datos. | gentle-ai-worker |

### Próximos pasos
- Cada subagente debe crear un commit con su respectiva tarea y ejecutar `go test ./...` para asegurar que la cobertura se mantiene > 80 %.
- Después de completar todas las tareas, ejecutar la aplicación y validar que todos los endpoints responden según el documento de requerimientos.
