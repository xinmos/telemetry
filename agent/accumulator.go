package agent

import (
	"log"

	"telemetry/models"
)

type accumulator struct {
	metrics chan<- models.Metric
}

func NewAccumulator(
	metrics chan<- models.Metric,
) models.Accumulator {
	acc := accumulator{
		metrics: metrics,
	}
	return &acc
}

func (ac *accumulator) AddMetric(m models.Metric) {
	ac.metrics <- m
}

func (ac *accumulator) AddError(err error) {
	if err == nil {
		return
	}
	log.Printf("Error in plugin: %v", err)
}
