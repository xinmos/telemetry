package models

import (
	"sync/atomic"
	"telemetry/plugin"
	"time"
)

type RunningInput struct {
	Input Input
	Name  string

	GatherTime int64
}

func (r *RunningInput) Init() error {
	if p, ok := r.Input.(plugin.Initializer); ok {
		err := p.Init()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RunningInput) Gather(acc Accumulator) error {
	start := time.Now()
	err := r.Input.Gather(acc)
	elapsed := time.Since(start)
	atomic.StoreInt64(&r.GatherTime, elapsed.Nanoseconds())
	return err
}
