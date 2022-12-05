package json

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	jsonata "github.com/blues/jsonata-go"

	"telemetry/models"
)

type Serializer struct {
	TimestampUnits  time.Duration
	TimestampFormat string

	transformation *jsonata.Expr
}

func NewSerializer(timestampUnits time.Duration, timestampFormat, transform string) (*Serializer, error) {
	s := &Serializer{
		TimestampUnits:  truncateDuration(timestampUnits),
		TimestampFormat: timestampFormat,
	}

	if transform != "" {
		e, err := jsonata.Compile(transform)
		if err != nil {
			return nil, err
		}
		s.transformation = e
	}

	return s, nil
}

func (s *Serializer) Serialize(metric models.Metric) ([]byte, error) {
	var obj interface{}
	obj = s.createObject(metric)

	if s.transformation != nil {
		var err error
		if obj, err = s.transform(metric); err != nil {
			if errors.Is(err, jsonata.ErrUndefined) {
				return nil, fmt.Errorf("%v (maybe configured for batch mode?)", err)
			}
			return nil, err
		}
	}

	serialized, err := json.Marshal(obj)
	if err != nil {
		return []byte{}, err
	}
	serialized = append(serialized, '\n')

	return serialized, nil
}

func (s *Serializer) transform(obj interface{}) (interface{}, error) {
	return s.transformation.Eval(obj)
}

func (s *Serializer) createObject(metric models.Metric) map[string]any {
	m := make(map[string]any)
	m["metric"] = metric
	return m
}

func truncateDuration(units time.Duration) time.Duration {
	// Default precision is 1s
	if units <= 0 {
		return time.Second
	}

	// Search for the power of ten less than the duration
	d := time.Nanosecond
	for {
		if d*10 > units {
			return d
		}
		d = d * 10
	}
}
