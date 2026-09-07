# ADR-0004 — Ingest, state ownership, and derivation

- **Status:** Accepted
- **Date:** 2026-09-07
- **Decides:** `ARCHITECTURE-DECISIONS-TO-MAKE.md` §4.1–§4.12, and §5.2 ahead of its section
- **Related:** `PRODUCT-SPEC.md` F1, F6, F7, F8, F9, §7.1, §7.2, §7.4; ADR-0001, ADR-0003
- **Amends:** `PRODUCT-SPEC.md` F1, F6, F8 — vehicles awaiting a first report

## Context

ADR-0001 placed derivation in the backend. ADR-0003 fixed the events arriving at it. This ADR decides
what holds the resulting state, who may write to it, how derived facts are produced, and what the
operator sees while the backend does not yet know the fleet.

Two `PRODUCT-SPEC.md` guarantees drive most of it. F9 requires that **events are the only writer of
state** — currently a promise, and this ADR is where it becomes a structural property. And §7.2 flagged
two failure modes as the likeliest correctness bugs in the whole design: vehicles belonging to no zone
being dropped from totals, and coverage being recomputed only on movement when it also changes on
status transitions. Both are closed here by construction rather than by care.

## Decision

1. **One process**, with the simulated source running in-process behind the consumer interface.
2. **One `FleetStore` port** holding signal state, observation timestamps and sequence numbers. One
   implementation: in-memory. Nothing derived is stored.
3. **Writes are serialised by a mutex; reads consume an immutable snapshot published atomically once
   per tick.**
4. **Publishing is tick-based** (deciding §5.2 early, since §4.4 depends on it).
5. **Data flows one way** — consumer → projection → store → derivation → broadcaster → clients — and
   the serving layer holds a **read-only** view of the store, enforced by types.
6. **Everything except filtering is computed once per tick for all clients.**
7. **The ingest queue is bounded and blocks when full.** No dropping.
8. **Staleness is derived per tick from the observation timestamp. It is never stored.**
9. **Zone membership is derived per tick from position. It is never cached.**
10. **"No zone" is an explicit value**, and the derived output carries a count of vehicles outside any
    zone.
11. **The backend has an explicit lifecycle:** `Starting → Ready`, where Ready means registration
    replay has completed. Before Ready the read path serves the snapshot plus a filling flag.
12. **Restart recovery is replay of the lifecycle topic from the beginning**, then live telemetry.

## Options considered

### The storage port (§4.2)

An earlier framing of this decision assumed that persisting live telemetry would be *incorrect*,
because restoring positions and presenting them as current would violate F6. **That was wrong, and
correcting it changed the shape of the port.** Because ADR-0003 derives staleness from the observation
timestamp, restored state labels itself stale immediately — persistence is honest by construction. It
is low *value*, since one second of live events makes it irrelevant, but not unsafe. So the port covers
all fleet state rather than carving out the roster as a special case.

| Option | For | Against |
|---|---|---|
| **One `FleetStore` port, in-memory implementation — chosen** | The port is where synchronisation lives, so it is load-bearing today rather than speculatively. It expresses the in-memory decision as a *choice* rather than an assumption, which is what the brief's "in-memory is acceptable" invites. Tests supply a second implementation. Stays small — a handful of methods, no query language, no transactions. | An interface with one production implementation is a recognised over-abstraction, and the covering email penalises exactly that. The justification has to keep holding. |
| A concrete in-memory struct, with a README note on where persistence would go | Least code, zero speculative abstraction, nothing to defend. | The production story becomes prose rather than structure, in a submission that is explicitly asked about production scale. |
| Port plus a real persistent implementation | Proves the seam rather than asserting it. | Requires a database or Redis, breaking the "Go, Node and make, nothing else" prerequisite, for a capability nothing in the spec needs. |

**Derived state is deliberately outside the port.** Attention flags, zone membership, coverage and
summary counts are computed from a snapshot and never written back. This keeps the store minimal, keeps
derived data out of any future persistence question, and means there is exactly one place where product
policy is applied.

### Process topology (§4.1)

| Option | For | Against |
|---|---|---|
| **One process, source in-process behind the consumer interface — chosen** | The consumer interface *is* the Kafka client seam, so the boundary that matters is modelled honestly. Keeps the single-command run true. | No real network boundary is exercised, so transport-independence of the ingest path is argued rather than demonstrated. |
| Separate simulator process | A real process and network boundary; proves ingest is transport-agnostic. | The transport would have to be HTTP or a WebSocket — **not Kafka** — so we would be building a fake broker. That is code modelling nothing real, and it has to be explained to a reviewer. Also two processes for `make dev` to orchestrate. |

### Concurrency (§4.4)

| Option | For | Against |
|---|---|---|
| **Mutex for writes, immutable snapshot published per tick — chosen** | Writers serialise cheaply. Readers get a consistent view with **no locking and no contention**, and the snapshot cost is paid once per tick rather than once per reader. Every connected client is guaranteed an identical view. | Only coherent if publishing is tick-based, which is why §5.2 is decided here. |
| `RWMutex` with readers copying on demand | Simplest possible. | Copy cost scales with the number of viewers, and two viewers can observe different states. |
| Single owning goroutine (actor model) | No locks anywhere; naturally serialised. | Reads serialise behind writes over a channel round-trip, making the writer a bottleneck for a read-heavy workload. |
| Sharded by vehicle | Scales furthest. | Unnecessary at a hundred vehicles, and complicates taking a coherent whole-fleet snapshot — which is exactly what the read path needs. |

### The write guarantee (§4.3)

The serving layer is given a read-only view of the store, with write methods unreachable from it. F9's
"events are the only writer of state" is otherwise a convention that holds until somebody adds a
handler that mutates state directly. Making it a property of the type system is also what makes §9.8
— *demonstrate it rather than assert it* — answerable: the demonstration is a signature, not a test.

### Overload (§4.7)

| Option | For | Against |
|---|---|---|
| **Bounded queue, block when full — chosen** | Models what a Kafka consumer genuinely does: fall behind, accrue lag, lose nothing. With an in-process source, blocking it *is* consumer lag. No silent data loss. | The source stalls, so a slow consumer becomes a slow producer — which is the intended behaviour but must not be mistaken for a bug. |
| Bounded queue, drop on full | System stays responsive under any load. | Models something Kafka does not do, and loses observations silently. |
| Unbounded queue | Never blocks, never drops. | Memory grows without limit under sustained overload; failure moves from visible to fatal. |

⚠️ `PRODUCT-SPEC.md` §4.2 excludes "backpressure onto the producer". That exclusion concerns rate
limiting as a *feature*; natural backpressure from a bounded channel is simply how a consumer behaves.
Recorded so the two do not read as contradictory.

### Derivation (§4.10, §4.11, §4.12)

**Staleness derived per tick, never stored — and this dissolves a tension flagged three times.**
`PRODUCT-SPEC.md` §7.1 recorded staleness as "the only operator-visible state not caused by an event",
an asymmetry requiring deliberate construction. But if staleness is a pure function of
`(latest observation timestamp, now)` computed during derivation, then **the store stays purely
event-written and the asymmetry never exists.** The rejected alternative — a background sweep marking
vehicles stale — would have introduced a second writer, in direct conflict with §4.3.

**Zone derived per tick, never cached**, for a specific reason: coverage changes on **status
transitions** as well as on movement, because only FREE vehicles count. A zone cached and invalidated on
position change would be silently wrong the moment a vehicle changed status without moving — which is
precisely the trap §7.2 recorded. Deriving from `(position, status)` every tick removes the invalidation
logic entirely. At a hundred vehicles and ten ticks per second this is around a thousand point-in-polygon
tests per second, which is negligible.

**"No zone" is an explicit value rather than a zero value**, and the derived output carries a count of
vehicles outside any zone. §7.2 named "vehicles belonging to no zone silently dropped from counts" as the
likeliest correctness bug in the design; an explicit value plus an explicit count makes the case
impossible to overlook instead of relying on every future aggregation being written carefully.

### Warm-up and restart (§4.8, §4.9)

Registration replay creates a category that had not been specified: **a vehicle that is known but has
never reported a position.** It cannot be drawn, because there is nowhere to draw it. And it is not
hypothetical — a vehicle offline at startup registers via replay and then never reports, which is exactly
the case replay was introduced to make visible.

Resolved by amending `PRODUCT-SPEC.md`: the map shows every vehicle **with a known position**, and the
summary reports how many are **awaiting a first report**. F8's map-summary reconciliation therefore
still holds, and the offline-at-startup vehicle is visible as a number even though it cannot be a
marker. F6 additionally requires that "awaiting a first report" stay distinguishable from "stale" —
never heard from, versus heard from and lost.

Restart is replay of the lifecycle topic from the beginning followed by live telemetry, which with real
Kafka is literally "subscribe from earliest on a compacted topic".

## Consequences

**Positive**

- "Events are the only writer" is enforced by types rather than by discipline, and is demonstrable by
  inspection.
- Both of the correctness bugs `PRODUCT-SPEC.md` §7.2 predicted are closed structurally.
- Deriving rather than storing staleness and zone means there is no cache to invalidate and no
  scheduled work — the two commonest sources of stale-derived-data bugs simply do not exist here.
- Every connected client provably sees the same snapshot, which matters because multiple independent
  viewers are supported.
- Nothing is lost under load; overload manifests as lag, which is visible in logs.
- The store is small enough that a persistent implementation would be a genuine drop-in.

**Negative / accepted costs**

- **Derivation recomputes everything every tick**, including for vehicles that have not changed. This
  is wasteful by design — bought deliberately in exchange for having no invalidation logic. It is also
  the first thing that needs revisiting at scale.
- **A one-implementation interface** must keep earning its place; if concurrency moves out of it, it
  becomes an empty abstraction and should collapse into a struct.
- **Tick-based publishing bounds latency from below.** The freshness budget of 250 ms is met by the tick
  interval, so the tick rate is now a product-visible constant, not an implementation detail.
- **A blocking ingest queue means a slow consumer stalls the source**, which is correct but will look
  like a hang if it is ever hit without the log being read.
- **No real network boundary is exercised** between source and ingest, so transport-independence rests
  on the interface being honest rather than on it being proven.
- The `Starting → Ready` lifecycle is another operator-visible state to render and to test.

## What would make us revisit this

- **Full-fleet derivation per tick becoming measurable**, which introduces dirty-tracking or
  incremental derivation — and reintroduces the invalidation logic this ADR deliberately avoided,
  including the status-transition trap.
- **A persistent store being genuinely wanted**, most plausibly to make warm-up instant rather than to
  preserve data.
- **Concurrency moving out of the store** — sharding, or an actor per vehicle — which removes the
  port's main justification.
- **Multiple backend instances**, which breaks the single-snapshot assumption and makes "every client
  sees the same view" a distributed problem.
- **Ingest lag becoming routine**, which turns the blocking queue from a correct model into an
  operational problem and forces a real answer about what to shed.
- **A second writer being genuinely needed** — operator annotations, for instance — which would end the
  read-only serving layer and require the write path to be modelled as events.

## At ~1000 vehicles

- **The mutex plus per-tick snapshot survives, but the snapshot becomes the cost.** Copying a
  thousand-vehicle map ten times a second is real allocation churn. The incremental fixes are
  copy-on-write with structural sharing, or publishing deltas rather than snapshots — the latter also
  being what the read path wants at that size.
- **Per-tick full derivation is the first thing to break.** A thousand point-in-polygon tests plus
  aggregation, ten times a second, is ~10,000 tests per second — still tractable in Go, but the
  headroom is gone. The mitigation is caching zone membership with invalidation on **both** position
  and status change, and the reason to record it here is that a future implementer will otherwise
  invalidate on position alone and reintroduce exactly the bug §7.2 predicted.
- **Ingest parallelises cleanly** because ADR-0003 made ordering per-vehicle-per-signal: partition by
  vehicle UUID, one goroutine per partition, no cross-partition coordination. The store then becomes
  the contended resource, and sharding it by the same key is the natural next step — at which point the
  whole-fleet snapshot needs gathering across shards, which is the real complexity that arrives with
  scale.
- **One process stops being sufficient** somewhere beyond this point, and the first thing to leave is
  the simulated source — which is also the least interesting part to move, because in production it is
  a real broker that was never in the process to begin with.
- **Warm-up lengthens**, since replay grows with the fleet, and warm-up is operator-visible.
- **The blocking queue matters more.** At ten times the event rate, transient lag becomes likelier, and
  "block the producer" stops being free when the producer is a real broker with retention limits rather
  than a goroutine.
