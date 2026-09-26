# Implementación de endpoints faltantes

## Tareas

1. **Handler GET `/meters/{meterId}`** – devolver información del medidor solicitado.
2. **Handler GET `/meters/{meterId}/readings`** – reutilizar lógica existente para obtener lecturas filtradas por rango opcional.
3. **Handler GET `/anomalies/{id}`** – detalle de una anomalía específica por su ID.
4. **Handler POST `/ai/analyze`** – disparar (o simular) un análisis AI y devolver un `analysisId`.
5. **Handler GET `/ai/analysis/{id}`** – obtener el resultado del análisis solicitado.
6. **Handler GET `/dashboard/summary`** – ofrecer un resumen agregado (health, número de medidores, anomalías, últimas lecturas, etc.).
7. **Actualizar `internal/api/router.go`** para registrar los nuevos endpoints bajo los prefijos `/api` y sin prefijo.
8. **Crear archivo `internal/api/handlers/ai.go`** con los handlers de los puntos 4‑6 y cualquier helper necesario.
9. **Escribir tests básicos** para los nuevos handlers (opcional en esta fase).
