package models

type Metric interface {
	IsMetric()

	// Copy returns a deep copy of the Metric.
	Copy() Metric
}
