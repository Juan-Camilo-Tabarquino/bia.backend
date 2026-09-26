# AI Energy Management Platform — Backend Implementation Plan & Prompt Guide

## Context & Project Overview
You are acting as a Senior Go Engineer. Your goal is to build the backend MVP for the **AI Energy Management Platform**.

This system ingests electrical meter readings and operational events, processes them through a deterministic analytical pipeline, flags anomalies, correlates them with known events, scores their severity and confidence, and uses an LLM to generate natural-language explanations and actionable recommendations.

The project must follow **idiomatic Go**, clean architecture, zero unnecessary external infrastructure (no DBs or external queues), and maintain a clear separation between statistical/deterministic analysis and AI interpretation.

---

## 1. Core Objectives
- Load electrical readings and operational events from CSV files into memory upon application startup.
- Validate and clean incoming data (Data Quality phase).
- Calculate baseline behavior for each meter.
- Detect statistical anomalies and analyze electrical consistency.
- Correlate statistical anomalies with known operational events.
- Classify anomalies into deterministic categories.
- Dynamically compute severity, confidence, and priority scores (no hardcoded logic for specific meter IDs).
- Construct explainable evidence objects.
- Feed structured evidence into an AI/LLM layer for automated investigation and actionable recommendations.
- Expose all results via a simple, clean REST API for frontend consumption.

---

## 2. Technical Constraints & Principles
- **Language**: Go 1.22+ (idiomatic Go, explicit error handling, `context.Context`, interfaces where valuable, clean dependency injection).
- **Persistence**: **No Database**. All data is loaded from CSV into **In-Memory** data structures.
- **Scale**: Small dataset (~12 meters, 14 days of data, ~4,032 records). In-memory processing is fast, efficient, and appropriate.
- **Dependencies**: No Redis, Kafka, RabbitMQ, PostgreSQL, Elasticsearch, or heavy ORMs. Keep it lightweight and easily testable.
- **AI Rule (CRITICAL)**: **The LLM is NOT the anomaly detector.**
  - **Pipeline order**: `CSV Data` -> `Data Quality` -> `Baseline Calculation` -> `Statistical Detection` -> `Electrical Analysis` -> `Event Correlation` -> `Classification` -> `Scoring (Severity & Confidence)` -> `Evidence Generation` -> **`LLM Layer`** -> `REST API`.
  - The analysis pipeline must be 100% deterministic, testable, and reproducible without calling the LLM. The LLM acts purely as an interpretation, explanation, and recommendation layer.

---

## 3. Data Models & CSV Input

### 3.1 Readings (`readings.csv`)
Columns:
- `meter_id` (string)
- `timestamp` (RFC3339 / ISO-8601 string)
- `consumption` (float64 - kWh)
- `voltage` (float64 - V)
- `current` (float64 - A)
- `power_factor` (float64 - range 0.0 to 1.0)

### 3.2 Operational Events (`events.csv`)
Columns:
- `meter_id` (string)
- `event_type` (string: `PRODUCTION_LINE_STARTED`, `MAINTENANCE`, `SCHEDULED_SHUTDOWN`, `OPERATIONAL_CHANGE`)
- `start_time` (RFC3339 string)
- `end_time` (RFC3339 string)
- `description` (string)

---

## 4. Test Scenarios & Expected Outcomes
The pipeline must correctly distinguish between these 4 representative cases using generic logic:

1. **Meter `M-104`**:
   - **Behavior**: Consumption spiked by ~+47.6%.
   - **Correlated Event**: `PRODUCTION_LINE_STARTED` event active during the same timeframe.
   - **Expected Classification**: `EXPLAINABLE_ANOMALY`
   - **Expected Severity**: `MEDIUM`

2. **Meter `M-106`**:
   - **Behavior**: Consumption dropped or fluctuated.
   - **Correlated Event**: `MAINTENANCE` or `SCHEDULED_SHUTDOWN` event active during the same timeframe.
   - **Expected Classification**: `FALSE_POSITIVE`
   - **Expected Severity**: `LOW`

3. **Meter `M-109`**:
   - **Behavior**: Consumption spiked by ~+103.7% above baseline with accompanying electrical parameter drift (e.g., voltage/current inconsistency).
   - **Correlated Event**: None.
   - **Expected Classification**: `REAL_ANOMALY`
   - **Expected Severity**: `HIGH`
   - **Scoring**: High confidence score (e.g., > 0.90) derived algorithmically.

4. **Meter `M-112`**:
   - **Behavior**: Total consumption seems baseline-like, but electrical variables are inconsistent (e.g., invalid power factor, missing/zero voltage despite active consumption).
   - **Expected Classification**: `DATA_QUALITY`
   - **Expected Severity**: `HIGH`

---

## 5. Software Architecture & Directory Structure

Organize the repository using standard Go project layouts:

```text
ai-energy/
├── cmd/
│   └── api/
│       └── main.go
│
├── internal/
│   ├── api/
│   │   ├── handlers/       # HTTP Handlers (Readings, Anomalies, Summary)
│   │   └── routes/         # Router setup (chi, gin, or stdlib net/http)
│   │
│   ├── config/
│   │   └── config.go       # App configurations (file paths, LLM keys, server port)
│   │
│   ├── data/
│   │   ├── csv/            # CSV parsing & transformation to domain models
│   │   │   ├── readings.go
│   │   │   └── events.go
│   │   └── memory/         # Thread-safe in-memory storage/repositories
│   │       ├── readings.go
│   │       └── events.go
│   │
│   ├── domain/
│   │   └── models/         # Core structs (Reading, Event, Anomaly, Baseline, Evidence)
│   │
│   ├── analysis/           # Deterministic Engine
│   │   ├── quality/        # Data quality check service
│   │   ├── baseline/       # Baseline computation logic
│   │   ├── detection/      # Statistical anomaly detection (Z-score / IQR / threshold)
│   │   ├── correlation/    # Correlates anomalies with operational events
│   │   ├── classification/ # Assigns classification categories
│   │   ├── scoring/        # Computes confidence, severity, and priority scores
│   │   ├── evidence/       # Assembles explainable payload for AI
│   │   └── orchestrator.go # Runs the complete analytical pipeline
│   │
│   └── ai/
│       └── agents/         # LLM integration (OpenAI/Gemini/Anthropic API client)
│
├── go.mod
└── go.sum
```

---

## 6. Development Pipeline & Implementation Tasks

### Phase 1: Domain & Data Layer
1. Define core domain models (`Reading`, `Event`, `Baseline`, `Anomaly`, `Evidence`, `AnalysisResult`).
2. Implement CSV readers in `internal/data/csv/` using `encoding/csv` to parse raw records into domain structs.
3. Build thread-safe memory repositories in `internal/data/memory/`.

### Phase 2: Analysis Engine (Deterministic)
1. **Data Quality Module**: Validate sensor bounds (e.g., $0 \le PF \le 1.0$, non-negative consumption).
2. **Baseline Module**: Calculate mean, standard deviation, and median per meter over normal periods.
3. **Statistical Detection**: Identify spikes/drops beyond calculated thresholds.
4. **Event Correlation**: Check if flagged time windows overlap with `events.csv` ranges.
5. **Classification & Scoring**: Apply generic rules to classify into `REAL_ANOMALY`, `EXPLAINABLE_ANOMALY`, `FALSE_POSITIVE`, or `DATA_QUALITY`. Compute numeric confidence and severity ratings.
6. **Evidence Builder**: Generate detailed structured text/JSON summarizing key metrics, standard deviations from baseline, and correlated event details.

### Phase 3: AI / LLM Integration
1. Define an AI client interface in `internal/ai/agents/`.
2. Construct structured system prompts instructing the LLM to process the **Evidence JSON** and return:
   - Concise explanation of the root cause.
   - Specific, actionable recommendations.
3. Provide a fallback mock implementation when no API key is set.

### Phase 4: API Layer & HTTP Handlers
1. Expose REST endpoints:
   - `GET /api/v1/health`: Basic health check.
   - `GET /api/v1/meters`: List of monitored meters.
   - `GET /api/v1/meters/{id}/readings`: Raw/aggregated readings.
   - `GET /api/v1/anomalies`: List of all detected anomalies with scores and AI recommendations.
   - `GET /api/v1/anomalies/{meter_id}`: Detailed investigation for a specific meter.

### Phase 5: Testing
1. Unit tests for statistical detection, event correlation logic, and data validation modules.
2. Integration test validating the pipeline against all 4 benchmark meters (`M-104`, `M-106`, `M-109`, `M-112`).

---

## 7. Instructions for Execution
When generating code, write **clean, modular, and idiomatic Go**:
- Use proper error wrapping (`fmt.Errorf("...: %w", err)`).
- Ensure high testability through dependency injection.
- Do not hardcode specific meter IDs inside analytical algorithms.