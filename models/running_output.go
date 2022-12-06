package models

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"telemetry/plugin"
)

const (
	// Default size of metrics batch size.
	DefaultMetricBatchSize = 1000

	// Default number of metrics kept. It should be a multiple of batch size.
	DefaultMetricBufferLimit = 10000
)

type RunningOutput struct {
	// Must be 64-bit aligned
	newMetricsCount int64
	droppedMetrics  int64

	Output            Output
	MetricBufferLimit int
	MetricBatchSize   int
	Name              string

	buffer *Buffer
	log    *logrus.Entry

	BatchReady chan time.Time
	WriteTime  int64

	aggMutex sync.Mutex
}

func NewRunningOutput(output Output, name string, batchSize, bufferLimit int) *RunningOutput {
	if bufferLimit == 0 {
		bufferLimit = DefaultMetricBufferLimit
	}
	if batchSize == 0 {
		batchSize = DefaultMetricBatchSize
	}

	ro := &RunningOutput{
		buffer:            NewBuffer(bufferLimit),
		BatchReady:        make(chan time.Time, 1),
		Output:            output,
		MetricBufferLimit: bufferLimit,
		MetricBatchSize:   batchSize,
		Name:              name,

		log: NewLogger("running_output"),
	}

	return ro
}

func (r *RunningOutput) Init() error {
	if p, ok := r.Output.(plugin.Initializer); ok {
		err := p.Init()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RunningOutput) AddMetric(metric Metric) {
	r.log.Debugf("get output: %v", metric)

	dropped := r.buffer.Add(metric)
	atomic.AddInt64(&r.droppedMetrics, int64(dropped))

	count := atomic.AddInt64(&r.newMetricsCount, 1)
	if count == int64(r.MetricBatchSize) {
		atomic.StoreInt64(&r.newMetricsCount, 0)
		select {
		case r.BatchReady <- time.Now():
		default:
		}
	}
}

// Close closes the output
func (r *RunningOutput) Close() {
	err := r.Output.Close()
	if err != nil {
		r.log.Errorf("Error closing output: %v", err)
	}
}

// Write writes all metrics to the output, stopping when all have been sent on
// or error.
func (r *RunningOutput) Write() error {
	atomic.StoreInt64(&r.newMetricsCount, 0)

	// Only process the metrics in the buffer now.  Metrics added while we are
	// writing will be sent on the next call.
	nBuffer := r.buffer.Len()
	nBatches := nBuffer/r.MetricBatchSize + 1
	for i := 0; i < nBatches; i++ {
		batch := r.buffer.Batch(r.MetricBatchSize)
		if len(batch) == 0 {
			break
		}

		err := r.writeMetrics(batch)
		if err != nil {
			r.buffer.Reject(batch)
			return err
		}
		r.buffer.Accept(batch)
	}
	return nil
}

// WriteBatch writes a single batch of metrics to the output.
func (r *RunningOutput) WriteBatch() error {
	batch := r.buffer.Batch(r.MetricBatchSize)
	if len(batch) == 0 {
		return nil
	}

	err := r.writeMetrics(batch)
	if err != nil {
		r.buffer.Reject(batch)
		return err
	}
	r.buffer.Accept(batch)

	return nil
}

func (r *RunningOutput) writeMetrics(metrics []Metric) error {
	dropped := atomic.LoadInt64(&r.droppedMetrics)
	if dropped > 0 {
		r.log.Warnf("Metric buffer overflow; %d metrics have been dropped", dropped)
		atomic.StoreInt64(&r.droppedMetrics, 0)
	}

	start := time.Now()
	err := r.Output.Write(metrics)
	elapsed := time.Since(start)
	atomic.AddInt64(&r.WriteTime, elapsed.Nanoseconds())

	if err == nil {
		r.log.Debugf("Wrote batch of %d metrics in %s", len(metrics), elapsed)
	}
	return err
}

func (r *RunningOutput) LogBufferStatus() {
	nBuffer := r.buffer.Len()
	r.log.Debugf("Buffer fullness: %d / %d metrics", nBuffer, r.MetricBufferLimit)
}
