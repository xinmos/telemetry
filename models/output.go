package models

type Output interface {
	Connect() error

	Close() error

	Write(metrics []Metric) error

	ParseConfig(map[string]any) error
}
