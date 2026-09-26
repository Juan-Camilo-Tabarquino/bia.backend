# PRUEBA TÉCNICA — AI Energy Management Platform

**Stack general:** Backend + Frontend + Data + Inteligencia Artificial  
**Objetivo:** Construir un MVP funcional para gestionar medidores eléctricos y utilizar IA para detectar, explicar, priorizar y recomendar acciones sobre anomalías.

---

## 1. Alcance General

| Métrica / Parámetro | Valor                                                                                                            |
| :------------------ | :--------------------------------------------------------------------------------------------------------------- |
| **Medidores**       | 12 medidores                                                                                                     |
| **Periodo**         | 14 días                                                                                                          |
| **Lecturas**        | 4.032 lecturas                                                                                                   |
| **Variables**       | Consumo (`consumption_kwh`), Voltaje (`voltage_v`), Corriente (`current_a`), Factor de Potencia (`power_factor`) |
| **Demo**            | 5 – 10 minutos                                                                                                   |

---

## 2. Objetivo y Capacidades Evaluadas

### Objetivo Principal

La prueba evalúa la capacidad de construir una solución _end-to-end_ que combine Backend, Frontend, Data/Analytics e Inteligencia Artificial. No se busca únicamente un CRUD de medidores, sino una experiencia que convierta datos en una decisión operativa.

### Capacidades Evaluadas

- **Backend:** API, persistencia/almacenamiento en memoria y procesamiento.
- **Frontend:** UX, dashboard, navegación, filtros y gráficas.
- **Data / Analytics:** Series temporales, baseline, outliers y calidad de datos.
- **IA:** Detección, explicación, priorización, confianza y recomendación.
- **Engineering:** Mantenibilidad, testing y documentación.

### Preguntas Clave que Debe Responder la Plataforma

1. ¿Qué está pasando con los medidores?
2. ¿Qué lecturas se salen de su comportamiento esperado?
3. ¿La anomalía es real, explicable o de calidad de datos?
4. ¿Cuál debería investigarse primero?
5. ¿Por qué la IA llegó a esa conclusión?
6. ¿Qué acción recomienda?

---

## 3. Flujo Principal del Usuario

Dashboard ──► Medidores ──► Detalle ──► Anomalías IA ──► Investigación ──► Acción

---

## 4. Motor de Anomalías y Casos de Prueba del Dataset

La técnica es libre: reglas, estadística, Z-score, Isolation Forest, series de tiempo, ML, LLM o combinación[cite: 1]. Se evalúa el resultado, no una tecnología específica[cite: 1].

### Casos de Prueba Críticos

| Medidor   | Caso u Origen del Cambio                                             | Resultado Esperado             | Severidad / Prioridad |
| :-------- | :------------------------------------------------------------------- | :----------------------------- | :-------------------- |
| **M-104** | Aumento (+47.6%) coincidente con nueva línea productiva[cite: 1].    | `EXPLAINABLE_ANOMALY`[cite: 1] | `MEDIUM`[cite: 1]     |
| **M-106** | Cambio explicado por parada programada / mantenimiento[cite: 1].     | `FALSE_POSITIVE`[cite: 1]      | `LOW`[cite: 1]        |
| **M-109** | Aumento >100% sin evento conocido y con cambios eléctricos[cite: 1]. | `REAL_ANOMALY`[cite: 1]        | `HIGH`[cite: 1]       |
| **M-112** | Consumo estable pero lecturas eléctricas inconsistentes[cite: 1].    | `DATA_QUALITY`[cite: 1]        | `HIGH`[cite: 1]       |

---

## 5. Regla Fundamental de la Inteligencia Artificial

> **El LLM NO debe ser el detector primario de anomalías**[cite: 1].
> La arquitectura debe ser estrictamente determinista en el motor de detección y usar el LLM como capa de interpretación, explicación e investigación explicable[cite: 1].

CSV Data ──► Data Quality ──► Baseline ──► Statistical Detection ──► Correlation ──► Evidence ──► LLM Layer ──► Investigation & Action

### Ejemplo de Respuesta JSON de la IA

`json
{
  "meter_id": "M-109",
  "anomaly": true,
  "type": "REAL_ANOMALY",
  "severity": "HIGH",
  "confidence": 0.96,
  "reason": "Consumo 103,7% por encima del baseline sin evento conocido.",
  "recommended_action": "Investigar medidor e instalación."
}
`[cite: 1]

---

## 6. Criterios de Evaluación

### Evaluación Global (100 Puntos)

- **Frontend / UX:** 20 pts[cite: 1]
- **Backend / API:** 20 pts[cite: 1]
- **Data / Analytics:** 20 pts[cite: 1]
- **Detección de anomalías:** 15 pts[cite: 1]
- **IA y explicabilidad:** 15 pts[cite: 1]
- **Testing / documentación / calidad:** 10 pts[cite: 1]

### Evaluación Específica de IA (100 Puntos)

- Detecta `M-109`: 30 pts[cite: 1]
- Prioriza `M-109`: 25 pts[cite: 1]
- Evita tratar `M-106` como anomalía real: 15 pts[cite: 1]
- Detecta `M-112` como problema de calidad: 10 pts[cite: 1]
- Explica con evidencia: 10 pts[cite: 1]
- Recomienda acción coherente: 10 pts[cite: 1]
