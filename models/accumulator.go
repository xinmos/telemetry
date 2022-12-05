package models

type Accumulator interface {
	AddMetric(Metric)

	AddError(err error)
}
