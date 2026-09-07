# ADR-0003 — The inbound event model

- **Status:** Accepted
- **Date:** 2026-09-07
- **Decides:** `ARCHITECTURE-DECISIONS-TO-MAKE.md` §3.1–§3.16
- **Related:** `PRODUCT-SPEC.md` F2, F6, F9, F10, §7.1, §7.4; ADR-0001
- **Defers to ADR-0004:** the storage port that registration replay populates (§4.2, §4.8, §4.9)

## Context

`PRODUCT-SPEC.md` established that signals arrive as individual events, that delivery is
**at-least-once** and **unordered**, that the backend composes them for the frontend, and that
events are the only writer of state. This ADR decides what those events are and what those
guarantees actually oblige.

The unordered assumption deserves justification, because Kafka *does* guarantee ordering within a
partition, and keying by vehicle would seem to give it to us. It does not, for two reasons:

- **Ordering is never guaranteed across topics.** A status event and a route event live on different
  topics, so they can arrive in either order regardless of keying.
- At-least-once redelivery and non-idempotent producer retries reorder *within* a partition in
  practice.

So designing for unordered delivery is an accurate reading of what Kafka promises, not pessimism.

## Decision

1. **Six event types:** `VehicleRegistered`, `VehiclePosition` (position and heading together),
   `VehicleBattery`, `VehicleStatus`, `RouteAssigned`, `RouteCleared`.
2. **Every event carries** vehicle UUID, event UUID, type, observation timestamp, a
   **per-vehicle-per-signal sequence number**, and the signal payload.
3. **Every signal carries an absolute value. No deltas, ever.**
4. **Ordering is by per-vehicle-per-signal sequence number**, strictly-newer-wins. Timestamps play
   no part in ordering.
5. **Duplicate suppression requires no bookkeeping.** It is a consequence of (3) and (4).
6. **No maximum event age.**
7. **The producer's observation timestamp is authoritative**, including for staleness. Backend
   receive time is recorded only to diagnose clock skew.
8. **Staleness is inferred from absence of any event**, never announced by one.
9. **One uniform position cadence for the whole fleet at 1 Hz.** Battery roughly every ten seconds,
   status on change.
10. **A route is an ordered coordinate list, GeoJSON-shaped, plus its destination and a route id.**
    No progress field.
11. **The backend learns a vehicle exists only from `VehicleRegistered`.** Telemetry for an
    unregistered vehicle is discarded and logged as a warning.
12. **Registrations are replayed at startup**, as a compacted lifecycle topic would deliver them.
13. **No retirement event.**
14. **An uninterpretable event is discarded and logged with a reason.** Never fatal, never blocking.
15. **JSON on the wire.**
16. **The only relationship the backend may rely on** is per-signal sequence monotonicity from a
    given producer.

## Options considered

### Ordering and duplicate suppression (§3.3, §3.4, §3.12)

| Option | For | Against |
|---|---|---|
| **Per-vehicle-per-signal sequence, strictly-newer-wins — chosen** | Each signal is an independent last-write-wins register compared only against itself. Immune to clock skew. Integer comparison. | Requires the producer to maintain a counter per vehicle per signal. |
| A single per-vehicle sequence across all signals | One counter per vehicle. | A battery event at sequence 50 would make an unrelated position event at 49 look superseded. Conflates independent streams. |
| Ordering by observation timestamp | No counter needed; the timestamp is carried anyway. | Ties at coarse granularity, and any clock adjustment can regress. Overloads one field with two jobs. |

**Duplicate suppression needs no seen-set, and this is the simplification worth stating plainly.**
Because every signal application is strictly-newer-wins over an absolute value, applying the same
event twice produces the same state as applying it once — the second arrival is not strictly newer
and is discarded. At-least-once is satisfied *structurally*: no bounded LRU per vehicle, no memory
growth, no tuning, no eviction policy to get wrong. The event UUID is still carried, for tracing and
so a discard can be logged meaningfully.

⚠️ **This holds only because values are absolute.** A delta — "battery decreased by 2%" — would be
corrupted by redelivery and would force a real deduplication set. *Absolute values only* is therefore
a load-bearing constraint on the contract, not a stylistic preference, and any future signal that
looks like an increment reopens this decision.

**No maximum event age**, for the same reason: the sequence comparison already discards anything not
newer, whatever its age. An age cutoff would be a second rule capable of disagreeing with the first,
and would wrongly drop a legitimately delayed event that genuinely *is* the newest for its signal.
Two sources of truth for "is this current" is worse than one.

### Which clock (§3.5)

| Option | For | Against |
|---|---|---|
| **Producer observation timestamp — chosen** | Answers the operator's real question: how current is this information *about the vehicle*. Telematics units carry GPS time, so vehicle clocks are genuinely well synchronised — this is not an optimistic assumption. | Vulnerable to producer clock skew; mitigated by logging implausible timestamps. |
| Backend receive time | Immune to skew entirely. | Would report a sixty-second-delayed observation as fresh. That is a lie about how current the picture is, and it would mask a real outage behind delayed delivery. |

### Reporting cadence (§3.13)

**Uniform across the fleet, and that uniformity is the point.** If parked vehicles reported every
five seconds and moving ones every second, "two missed reports" would mean something different per
vehicle — and `PRODUCT-SPEC.md` requires the legend to *state* the threshold, which becomes
unstateable if the interval depends on what the vehicle is doing.

⚠️ 1 Hz with a two-interval threshold means stale after roughly two seconds. **A real operation on
cellular telematics would use something nearer thirty seconds**, because any network blip would
otherwise trip it. Accepted here because the simulator controls cadence precisely so jitter is
near zero, and because a reviewer sees the stale state appear within seconds rather than waiting half
a minute. It is one constant.

### Event granularity (§3.1, §3.2, §3.6, §3.7)

Position and heading travel in one event because they come from a single sensor read; splitting them
would create a window in which heading disagrees with position, for no benefit.

`RouteCleared` is a distinct type rather than `RouteAssigned` with a null geometry, because "absent"
and "not included in this update" would otherwise be indistinguishable.

Route events carry a **route id** as well as a sequence number. Without it, an out-of-order
`RouteCleared` could wipe a route assigned *after* it; clearing applies only when the id matches the
current route. The sequence number would technically cover this, but the id makes the intent explicit
and the logs legible.

A route carries **no progress field**. `PRODUCT-SPEC.md` derives progress from the vehicle's position
along the drawn line, so a number in the contract would be a second, disagreeing source of truth.
Shaping the geometry as the renderer expects avoids a transform, at the cost of a mild coupling
between contract and renderer, which is accepted.

### Fleet membership (§3.9, §3.11, §3.12 lifecycle)

| Option | For | Against |
|---|---|---|
| **`VehicleRegistered`, replayed at startup — chosen** | The roster is recoverable, so **a vehicle that is silent when the backend starts appears as registered-but-stale rather than invisible.** Static data is not repeated on every message. Replaying the lifecycle topic from the beginning is precisely what a compacted Kafka topic delivers on subscribe, so the recovery path is realistic rather than a workaround. Gives warm-up a definite termination condition instead of a timer. | A one-time historical fact must be re-delivered on every start, which only works because the source replays it. Creates a window at startup during which telemetry can arrive before its registration. |
| The label carried on every event | Every event self-sufficient; no bootstrap problem; no unknown-vehicle case at all. | Repeats static data on every message. And it removes any authoritative roster, so the backend can never know whether it has seen the whole fleet — leaving warm-up to a timer and making a vehicle that was already silent at startup **invisible rather than stale**, with no way to know it exists. |
| No registration; accept any UUID on first sight | Simplest possible. | A stray or malformed UUID silently becomes a vehicle on the operator's map. |

**Telemetry for an unregistered vehicle is discarded with a warning.** Because delivery is unordered
this will happen legitimately, not only in error — and it is harmless: at 1 Hz, a dropped
pre-registration position update costs one second, and the next arrives. Buffering such events until
registration lands would be the alternative, and it buys a second of accuracy in exchange for an
unbounded pending map keyed by unknown vehicles. Not worth it.

⚠️ This does place a requirement on the event source (§7): **registration replay must complete before
telemetry starts**, or a large number of updates are dropped at startup for no reason.

**No retirement event.** Nothing in `PRODUCT-SPEC.md` requires a vehicle to leave the fleet, and
adding an event type nothing emits is the dead code the covering email explicitly penalises. Cost: a
genuinely decommissioned vehicle would linger as permanently stale.

### Malformed events and Kafka's real shape (§3.11, §3.14, §3.15)

A malformed event is discarded and logged with a reason and a bounded excerpt of the payload. One bad
event must never stop the fleet. The real-Kafka equivalent is committing the offset and producing to a
dead-letter topic; that seam is named, not built.

**Modelled from Kafka:** logical topics for telemetry, lifecycle and routes; **partition key = vehicle
UUID**, which is what makes per-vehicle sequencing the right granularity; at-least-once delivery; and
an ingest interface shaped as a consumer over `(topic, key, payload)` so a real client substitutes
without touching projection logic.

**Deliberately not modelled:** offsets and replay positions, consumer groups and rebalancing, and log
compaction. Compaction is worth naming because it is the mechanism that makes registration replay
natural rather than contrived — the design leans on its semantics while implementing them in the
source.

## Consequences

**Positive**

- At-least-once and unordered delivery are handled by one mechanism — a per-signal newer-wins
  register — rather than by two mechanisms that could disagree. There is no dedup cache, no age
  cutoff, and no eviction policy.
- Each signal is independent, so the projection is a small amount of obviously-correct code.
- Registration replay makes the roster recoverable, which converts the worst failure (a silent
  vehicle being invisible) into an honest one (a silent vehicle being visibly stale).
- Warm-up has a definite end: replay complete.
- The ingest boundary is shaped like a Kafka consumer, so "structured as if Kafka were the source of
  truth" is demonstrable rather than asserted.

**Negative / accepted costs**

- **The producer carries real obligations**: a counter per vehicle per signal, absolute values only,
  registration replay before telemetry, and a regular cadence. The simulator is therefore not a toy,
  and §7 must honour all four.
- **Cross-signal arrival order cannot be relied on**, which has two operator-visible consequences that
  must be handled deliberately rather than discovered:
  - a vehicle may be EN_ROUTE with **no route yet**, and must draw no route without looking broken;
  - a route may be held for a vehicle that is currently FREE or WITH_CUSTOMER, and must not be drawn.
  The rule "draw a route only when status is EN_ROUTE" resolves both, and is already what
  `PRODUCT-SPEC.md` F2 requires.
- **Telemetry is dropped during the registration window**, silently to the operator and visibly only
  in logs.
- **A ~2 second staleness threshold is unrealistically tight** for real telematics.
- **Absolute-values-only constrains every future signal.** Anything naturally expressed as an
  increment cannot be added without revisiting duplicate suppression.
- A decommissioned vehicle would linger as permanently stale.
- JSON is verbose; see scale.

## What would make us revisit this

- **A signal that is naturally a delta** — cumulative distance, energy consumed — which breaks the
  structural idempotence in §3.3 and forces real deduplication.
- **Producer clocks that cannot be trusted**, which moves staleness onto receive time and accepts that
  delayed delivery then reads as fresh.
- **A need for cadence to vary by vehicle state**, which makes the staleness threshold per-state and
  forces the legend to explain a rule rather than state a number.
- **Vehicles being decommissioned in practice**, which introduces a retirement event.
- **A second consumer of the stream**, which is also ADR-0001's trigger for replacing Go-sourced types
  with a neutral schema.
- **Payload volume becoming the constraint**, which points at a binary encoding and reopens ADR-0001's
  codegen decision.
- **Telemetry dropped at startup becoming material**, which introduces buffering of pre-registration
  events with a bounded, expiring pending map.

## At ~1000 vehicles

- **Event volume rises linearly to roughly 1000 position events per second**, plus battery and status.
  Well within a Go consumer's capacity, but it is the point at which JSON parsing becomes a measurable
  share of CPU and a binary encoding starts to pay.
- **Per-signal sequence comparison is O(1) per event** and entirely independent per vehicle, so ingest
  parallelises cleanly — partition by vehicle UUID, one goroutine per partition, no cross-partition
  coordination. This is the property that makes the scale story credible, and it exists because
  ordering is per-vehicle-per-signal rather than global.
- **Having no dedup cache matters more at scale, not less.** A seen-set would have been per-vehicle
  state growing with both fleet size and event rate, with an eviction policy to tune. Structural
  idempotence has no such term.
- **Registration replay grows with the fleet**, so startup replays a thousand registrations before
  telemetry begins. Still trivial, but it lengthens warm-up, and warm-up is operator-visible.
- **The registration window widens**, so more telemetry is dropped at startup. At some fleet size
  buffering pre-registration events becomes worthwhile.
- **Uniform cadence becomes expensive for parked vehicles.** With most of a large fleet stationary,
  reporting identical positions at 1 Hz is mostly waste — and the obvious fix, slowing parked
  reporting, is exactly what §3.13 rejected because it makes staleness unstateable. That tension is
  unresolved and is the most likely thing to give way at scale.
