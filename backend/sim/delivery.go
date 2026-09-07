package sim

import (
	"math/rand/v2"
	"time"

	"fleetradar/fleet"
)

// delivery makes the stream imperfect on purpose. At-least-once and unordered delivery are
// guarantees the backend claims to handle, and a perfect source would leave both claims resting
// entirely on tests — whereas duplicates and reordering are visible in the log of a running system
// as `discarded` with a reason, which is the demonstration (ADR-0007 §7.8).
//
// Delayed records are held until a later tick rather than handed to a timer, so the imperfection is
// deterministic under a seeded run and works against a fake clock.
type delivery struct {
	consumer fleet.Consumer
	random   *rand.Rand
	pending  []delayed
}

type delayed struct {
	at     time.Time
	record fleet.Record
}

// send hands one record over, sometimes twice, sometimes later.
//
// Per-event delay on a subset is safe for staleness, which is worth stating because the opposite
// was assumed once: staleness compares the clock against the newest observation received, so
// delaying one event does not delay the next. The late one arrives, is discarded as superseded, and
// staleness is untouched because a newer observation already landed on time. Only uniform delay
// would trip it (ADR-0007 §7.8, correcting PRODUCT-SPEC §7.1).
func (d *delivery) send(now time.Time, record fleet.Record) {
	if d.random.Float64() < DelayChance {
		d.pending = append(d.pending, delayed{
			at:     now.Add(DelayMin + time.Duration(d.random.Int64N(int64(DelayMax-DelayMin)))),
			record: record,
		})
		return
	}

	d.consumer.Consume(record)
	if d.random.Float64() < DuplicateChance {
		d.consumer.Consume(record)
	}
}

// direct hands a record over untouched. The registration replay uses it: a replay is not the live
// stream, and delaying a registration would drop a second of a vehicle's telemetry to prove
// nothing (ADR-0003 §3.9).
func (d *delivery) direct(record fleet.Record) {
	d.consumer.Consume(record)
}

// release hands over everything whose delay has elapsed.
func (d *delivery) release(now time.Time) {
	held := d.pending[:0]
	for _, item := range d.pending {
		if item.at.After(now) {
			held = append(held, item)
			continue
		}
		d.consumer.Consume(item.record)
	}
	d.pending = held
}
