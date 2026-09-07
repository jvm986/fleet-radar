package fleet

import "context"

// queueDepth bounds the queue in front of the projection. A few hundred milliseconds of the
// fleet's event rate, which is enough to absorb a tick's worth of work without pretending the
// backend can never fall behind.
const queueDepth = 1024

// Consumer is what a source publishes into. The simulated source and a real Kafka client present
// the same interface and hand over the same serialised bytes, so substituting one for the other
// changes nothing downstream — which is what makes "structured as if Kafka were the source of
// truth" demonstrable rather than asserted (ADR-0003 §3.16, ADR-0007 §7.2).
type Consumer interface {
	Consume(Record)
}

// Ingest is the bounded queue between the source and the projection.
type Ingest struct {
	projector *Projector
	queue     chan queued
}

type queued struct {
	record Record
	// barrier is closed instead of the record being projected. See Barrier.
	barrier chan struct{}
}

func NewIngest(projector *Projector) *Ingest {
	return &Ingest{projector: projector, queue: make(chan queued, queueDepth)}
}

// Consume blocks when the queue is full, and that is the intended behaviour: it models what a
// Kafka consumer genuinely does, which is fall behind and accrue lag without losing anything.
// With an in-process source, blocking the producer *is* consumer lag. Dropping would be quieter
// and would model something Kafka does not do (ADR-0004 §4.7).
func (i *Ingest) Consume(r Record) {
	i.queue <- queued{record: r}
}

// Barrier returns a channel that closes once everything enqueued before it has been projected.
//
// This is how "registration replay has completed" becomes a definite fact rather than a timer:
// completion is a position in the stream, which is what a real broker's offsets would tell us.
// Without it the backend could declare itself Ready while the roster was still in the queue, and
// the operator would be shown a partial fleet as though it were complete (ADR-0004 §4.11).
func (i *Ingest) Barrier() <-chan struct{} {
	reached := make(chan struct{})
	i.queue <- queued{barrier: reached}
	return reached
}

// Run projects until the context ends. One goroutine drains the queue, so ordering within it is
// preserved and the projection needs no synchronisation of its own beyond the store's.
func (i *Ingest) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case next := <-i.queue:
			if next.barrier != nil {
				close(next.barrier)
				continue
			}
			i.projector.Apply(next.record)
		}
	}
}
