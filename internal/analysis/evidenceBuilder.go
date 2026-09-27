package analysis

import (
	"fmt"
	"strings"

	"github.com/neuralium/ai-energy/internal/domain/models"
)

// EvidenceBuilder produces the final, JSON-ready evidence records. It keeps the
// deterministic classification and score produced by the previous stages and
// adds the human-readable explanation and the recommended action, so every
// evidence record travels fully populated through the API.

type EvidenceBuilder interface {
	Build(evidence []models.Evidence) []models.Evidence
}

func NewEvidenceBuilder() EvidenceBuilder { return &defaultEvidenceBuilder{} }

type defaultEvidenceBuilder struct{}

func (b *defaultEvidenceBuilder) Build(evidence []models.Evidence) []models.Evidence {
	out := make([]models.Evidence, 0, len(evidence))
	for _, e := range evidence {
		e.Explanation = describeEvidence(e)
		e.Recommendation = recommendAction(e.Type)
		out = append(out, e)
	}
	return out
}

func describeEvidence(e models.Evidence) string {
	meter := string(e.Anomaly.MeterID)
	percent := e.Anomaly.Delta * 100
	switch e.Type {
	case models.AnomalyDataQuality:
		if e.Anomaly.Reason != "" {
			return fmt.Sprintf("El medidor %s presenta lecturas eléctricas inconsistentes (%s) mientras su consumo se mantiene cerca de la línea base horaria.", meter, e.Anomaly.Reason)
		}
		return fmt.Sprintf("El medidor %s presenta lecturas eléctricas inconsistentes mientras su consumo se mantiene cerca de la línea base horaria.", meter)
	case models.AnomalyReal:
		return fmt.Sprintf("El consumo del medidor %s está %.1f%% por encima de su línea base sin ningún evento operativo conocido.", meter, percent)
	case models.AnomalyExplainable:
		return fmt.Sprintf("El consumo del medidor %s está %.1f%% por encima de su línea base y coincide con un cambio operativo conocido: %s.", meter, percent, eventDescription(e.Correlation.Events))
	case models.AnomalyFalsePositive:
		return fmt.Sprintf("La desviación del consumo del medidor %s de %.1f%% se explica por mantenimiento planificado: %s.", meter, percent, eventDescription(e.Correlation.Events))
	default:
		return fmt.Sprintf("El medidor %s presenta una desviación no explicada de %.1f%%.", meter, percent)
	}
}

func eventDescription(events []models.Event) string {
	if len(events) == 0 {
		return "sin evento registrado"
	}
	parts := make([]string, 0, len(events))
	for _, e := range events {
		if e.Description != "" {
			parts = append(parts, fmt.Sprintf("%s (%s)", e.Description, e.Type))
			continue
		}
		parts = append(parts, string(e.Type))
	}
	return strings.Join(parts, "; ")
}

func recommendAction(anomalyType models.AnomalyType) string {
	switch anomalyType {
	case models.AnomalyReal:
		return "Revisar el medidor y su instalación."
	case models.AnomalyDataQuality:
		return "Revisar la calibración del sensor y el proceso de calidad de datos de este medidor."
	case models.AnomalyExplainable:
		return "No se requiere acción; la desviación coincide con un cambio operativo registrado."
	case models.AnomalyFalsePositive:
		return "No se requiere acción; la desviación se explica por mantenimiento planificado."
	default:
		return "Revisar la lectura antes de escalar."
	}
}
