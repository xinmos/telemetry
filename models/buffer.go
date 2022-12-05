package models

import (
	"sync"
)

// Buffer stores metrics in a circular buffer.
type Buffer struct {
	sync.Mutex
	buf   []Metric
	first int // index of the first/oldest metric
	last  int // one after the index of the last/newest metric
	size  int // number of metrics currently in the buffer
	cap   int // the capacity of the buffer

	batchFirst int // index of the first metric in the batch
	batchSize  int // number of metrics currently in the batch
}

// NewBuffer returns a new empty Buffer with the given capacity.
func NewBuffer(capacity int) *Buffer {
	b := &Buffer{
		buf:   make([]Metric, capacity),
		first: 0,
		last:  0,
		size:  0,
		cap:   capacity,
	}
	return b
}

// Len returns the number of metrics currently in the buffer.
func (b *Buffer) Len() int {
	b.Lock()
	defer b.Unlock()

	return b.length()
}

func (b *Buffer) length() int {
	return min(b.size+b.batchSize, b.cap)
}

func (b *Buffer) addMetric(m Metric) int {
	dropped := 0
	// Check if Buffer is full
	if b.size == b.cap {
		dropped++

		if b.batchSize > 0 {
			b.batchSize--
			b.batchFirst = b.next(b.batchFirst)
		}
	}

	b.buf[b.last] = m
	b.last = b.next(b.last)

	if b.size == b.cap {
		b.first = b.next(b.first)
	}

	b.size = min(b.size+1, b.cap)
	return dropped
}

// Add adds metrics to the buffer and returns number of dropped metrics.
func (b *Buffer) Add(metrics ...Metric) int {
	b.Lock()
	defer b.Unlock()

	dropped := 0
	for i := range metrics {
		if n := b.addMetric(metrics[i]); n != 0 {
			dropped += n
		}
	}

	return dropped
}

// Batch returns a slice containing up to batchSize of the oldest metrics not
// yet dropped.  Metrics are ordered from oldest to newest in the batch.  The
// batch must not be modified by the client.
func (b *Buffer) Batch(batchSize int) []Metric {
	b.Lock()
	defer b.Unlock()

	outLen := min(b.size, batchSize)
	out := make([]Metric, outLen)
	if outLen == 0 {
		return out
	}

	b.batchFirst = b.first
	b.batchSize = outLen

	batchIndex := b.batchFirst
	for i := range out {
		out[i] = b.buf[batchIndex]
		b.buf[batchIndex] = nil
		batchIndex = b.next(batchIndex)
	}

	b.first = b.nextby(b.first, b.batchSize)
	b.size -= outLen
	return out
}

// Accept marks the batch, acquired from Batch(), as successfully written.
func (b *Buffer) Accept(batch []Metric) {
	b.Lock()
	defer b.Unlock()

	b.resetBatch()
}

// Reject returns the batch, acquired from Batch(), to the buffer and marks it
// as unsent.
func (b *Buffer) Reject(batch []Metric) {
	b.Lock()
	defer b.Unlock()

	if len(batch) == 0 {
		return
	}

	free := b.cap - b.size
	restore := min(len(batch), free)
	skip := len(batch) - restore

	b.first = b.prevby(b.first, restore)
	b.size = min(b.size+restore, b.cap)

	re := b.first

	// Copy metrics from the batch back into the buffer
	for i := range batch {
		if i < skip {
			continue
		} else {
			b.buf[re] = batch[i]
			re = b.next(re)
		}
	}

	b.resetBatch()
}

// next returns the next index with wrapping.
func (b *Buffer) next(index int) int {
	index++
	if index == b.cap {
		return 0
	}
	return index
}

// nextby returns the index that is count newer with wrapping.
func (b *Buffer) nextby(index, count int) int {
	index += count
	index %= b.cap
	return index
}

// prevby returns the index that is count older with wrapping.
func (b *Buffer) prevby(index, count int) int {
	index -= count
	for index < 0 {
		index += b.cap
	}

	index %= b.cap
	return index
}

func (b *Buffer) resetBatch() {
	b.batchFirst = 0
	b.batchSize = 0
}

func min(a, b int) int {
	if b < a {
		return b
	}
	return a
}
