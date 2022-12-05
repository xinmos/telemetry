package models

type Input interface {
	Gather(Accumulator) error
	ParseConfig(map[string]any) error
}

type ServiceInput interface {
	Input

	Start(Accumulator) error

	Stop()
}
