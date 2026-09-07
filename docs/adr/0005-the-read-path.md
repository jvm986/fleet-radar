# ADR-0005 — The read path: backend to browser

- **Status:** Accepted
- **Date:** 2026-09-07
- **Decides:** `ARCHITECTURE-DECISIONS-TO-MAKE.md` §5.1–§5.13
- **Related:** `PRODUCT-SPEC.md` F2, F6, F8, F9, §5, §7.6; ADR-0001, ADR-0002, ADR-0004

## Context

`PRODUCT-SPEC.md` sets a 250 ms budget from an event being ingested to the change being visible, and
requires that the operator is never shown a partial fleet as though it were complete, that a lost
connection is distinguishable from a single stale vehicle, and that the summary always reconciles with
the map. ADR-0004 fixed publishing as tick-based and made every reader consume an identical immutable
snapshot.

The system is **observe-only**: no commands, no writes, no client-initiated requests beyond opening the
stream. That single fact settles more of this ADR than any performance consideration.

## Decision

1. **Each message is a complete whole-fleet snapshot.** No deltas.
2. **A 200 ms tick, published unconditionally** whether or not anything changed.
3. **One stream carrying three message kinds:** `config` once at the start, `routes` on connect and on
   change, and `snapshot` every tick.
4. **Vehicles, per-zone coverage, summary counts and the backend lifecycle state travel in one
   message.**
5. **Route geometry is sent on connect and when a route changes**, and referenced by id from snapshots.
6. **Server-Sent Events** carries the stream.
7. **The client requests nothing** beyond the GET that opens the stream.
8. **A newly connected client is served `config`, all current routes, then the next snapshot.** There is
   no separate initial-state endpoint.
9. **Disconnection is detected by a client-side watchdog** — no snapshot for roughly three ticks — with
   the transport's own error as a supplementary signal.
10. **On reconnect the next snapshot replaces client state wholesale.** No resync protocol.
11. **Each client has a send buffer of depth one; the latest snapshot replaces any pending one.** A
    persistently slow client is disconnected.
12. **The completeness claim travels inside the snapshot**, not alongside it.
13. **Filtering is applied in the client.**

## Options considered

### Snapshot versus delta (§5.1)

| Option | For | Against |
|---|---|---|
| **Full snapshot — chosen** | Idempotent and self-healing: every message re-establishes complete truth, so a dropped or skipped message requires no recovery and there is no client-side reconciliation to get wrong. Makes §5.10 satisfiable by construction. Makes §5.9's coalescing trivially correct. | Re-sends unchanged vehicles — roughly 20 KB per message at a hundred vehicles. |
| Deltas | Substantially smaller payloads. | The client must hold state, detect gaps and resynchronise. Each of those is a defect that only manifests under loss, which is precisely when it is hardest to diagnose. Also forecloses dropping stale messages for slow clients. |

Deltas are the answer at scale, not at this one. The decision is bought knowingly.

### Tick interval and publishing policy (§5.2)

A 250 ms tick would consume the entire freshness budget before the browser did any work, so the tick has
to sit strictly inside it. 200 ms leaves roughly 50 ms for serialisation, transport and render.

**Publishing unconditionally** rather than only on change: at a hundred vehicles reporting at 1 Hz, about
twenty vehicles change per tick, so a change check would almost always answer yes while costing a
full-fleet comparison to ask. The second reason is stronger and is recorded under §5.7 — **the
unconditional tick is what makes silence diagnostic.**

### Stream composition (§5.12, §5.13, §5.6)

Vehicles, coverage and summary counts share one message because they must describe **one generation of
state**. Separate streams or endpoints could deliver coverage computed at tick N alongside vehicles from
tick N+1, and F8 treats a summary that disagrees with the map as a defect. One message per generation
makes the inconsistency unrepresentable.

`config` — thresholds, service-area geometry, zone definitions — is sent once, first. Sending it first
guarantees it is present before any snapshot needs it, and because the client renders the legend from
what it receives, the legend cannot disagree with the logic. That was ADR-0001's reason for refusing the
client its own copy of the thresholds.

**Route geometry is the one place the uniform snapshot breaks down**, and it is worth being explicit
about why. Geometry is large and slow-changing: about 10 KB for ten routes, each lasting minutes. Carried
in a 200 ms snapshot, static polylines would dominate the payload — around 500 KB/s at a hundred
concurrent routes, all of it unchanged. So snapshots carry a route id and geometry travels separately.

| Option | For | Against |
|---|---|---|
| **Sent on connect and on change, referenced by id — chosen** | Sends each geometry once per connection per change. Keeps the stream unidirectional. | Introduces client-side cache state, breaking the otherwise uniform "every message is complete" property. |
| Included in every snapshot | Perfectly uniform; no cache. | Static data would dominate the payload, and badly so at scale. |
| Fetched over HTTP when a vehicle is selected | Minimal traffic. | Faint routes are drawn for *all* remotely-driven vehicles, so all of them are needed anyway — and it would reintroduce client-initiated requests, contradicting §5.4. |

Loss is bounded: SSE delivers in order within a connection, and reconnect re-sends everything. If the
client ever holds a route id without geometry it simply does not draw that route — degraded, never
wrong. Periodic re-sends were considered as a self-heal and rejected: within a connection there is
nothing to heal, so it would solve an invented problem.

### Transport (§5.5)

| Option | For | Against |
|---|---|---|
| **Server-Sent Events — chosen** | Matches the shape of the problem exactly: the system is observe-only, so a server-to-client channel is the whole of what needs modelling. Plain HTTP, no upgrade, no library — in Go it is writing to a `ResponseWriter` and flushing. Browser-native reconnection, which §5.8 uses directly. | Text only, so JSON rather than a binary encoding. No client-to-server channel on the same connection. |
| WebSocket | Bidirectional, binary-capable, widely understood. | Bidirectionality is capability the product deliberately excludes. It costs a dependency, manual ping/pong liveness, and hand-rolled reconnection — all to obtain something we decided not to have. |
| HTTP polling | Trivially simple, no streaming concerns. | Cannot meet a 250 ms budget without hammering, and wastes a request per interval. |

Choosing WebSocket would mean adopting a bidirectional transport for a deliberately unidirectional
system. The "no client requests" decision (§5.4) and the transport decision are the same decision seen
twice.

### Failure handling (§5.7, §5.8, §5.9, §5.10)

**A client-side watchdog rather than transport errors.** A stalled TCP connection can hang silently —
socket open, nothing arriving, no error raised. Relying on an error event would leave the operator
looking at a frozen map that appears live, which F6 explicitly forbids. The transport's error event is
used when it fires; the watchdog is authoritative.

⚠️ **The unconditional tick is what makes this work.** Because a snapshot arrives every 200 ms regardless
of activity, silence is unambiguous. Under conditional publishing we would have had to invent a separate
heartbeat message to distinguish "nothing is happening" from "the connection is dead" — so §5.2's
simplification and §5.7's reliability are the same choice.

**Reconnect needs no protocol** because the next snapshot is complete. Selection and filters are
client-side and survive untouched. And this is where ADR-0001's backend-derived staleness pays off as
predicted: the client renders the staleness it is told about, so while disconnected nothing advances and
no healthy vehicle is falsely blamed. §6.10 requires no code at all.

**Per-client buffer of depth one.** An old snapshot is worthless once a newer one exists, so replacing a
pending message is not merely acceptable but correct. This is only available because messages are
snapshots; with deltas nothing can be dropped. Blocking the broadcaster on a slow client was rejected —
one slow viewer would degrade every other viewer.

**The completeness claim travels inside the snapshot.** Because the lifecycle state is in the payload,
there is no window in which the client holds data but not the claim about that data. A separate status
endpoint would race exactly there, and the failure would be intermittent and rare — the worst kind.

## Consequences

**Positive**

- There is one data path, one message shape per generation, and no reconciliation logic anywhere in the
  client.
- Every failure mode has a defined behaviour that follows from the snapshot model rather than from
  bespoke handling: missed message, slow client, reconnect, and stalled connection all resolve the same
  way.
- The transport is a standard library concern in Go and a browser primitive in the client — no dependency
  on either side.
- Internal consistency between map, coverage and summary is structural.
- The legend cannot contradict the logic.

**Negative / accepted costs**

- **Bandwidth is spent on unchanged data**, deliberately. Roughly 800 kbps at a hundred vehicles, almost
  all of it repetition.
- **Route geometry is the exception to the uniform model**, so there is exactly one piece of client-side
  cache state, and one degraded-rendering path when a referenced geometry is absent.
- **The tick interval is now a product-visible constant**: it bounds freshness from below, so changing it
  changes a stated guarantee.
- **SSE is text-only**, so a binary encoding is not available without changing transport — which
  compounds ADR-0001's JSON constraint.
- **No client-to-server channel exists at all.** If the product ever gains an action, this ADR and
  ADR-0001's observe-only premise are reopened together.
- A slow client is disconnected rather than degraded, so a genuinely weak machine gets a reconnect loop
  rather than a lower frame rate.

## What would make us revisit this

- **Snapshot size becoming the constraint**, which introduces deltas — and with them gap detection,
  resync, and the loss of the coalescing property in §5.9.
- **The product gaining any operator action**, which requires a client-to-server path and makes
  WebSocket's bidirectionality relevant for the first time.
- **A tighter freshness requirement**, which shortens the tick and squeezes the serialise-transport-render
  budget rather than changing anything structural.
- **Multiple backend instances**, which breaks the assumption that every client's snapshot comes from one
  authoritative store.
- **Needing server-side filtering** — see the scale note, because this is not a payload optimisation.
- **Route geometry becoming large or frequently revised**, which would make the on-change model expensive
  and argue for fetching geometry on demand, reintroducing client requests.

## At ~1000 vehicles

- **Full snapshots stop being viable, and this is the first thing to break.** Roughly 200 KB per message
  at 5 Hz is about 8 Mbps per viewer, of which the overwhelming majority is unchanged data. Deltas become
  necessary, and they bring back everything the snapshot model gave us for free: gap detection, resync
  after reconnect, and the inability to drop a stale pending message for a slow client. The right
  sequence is probably deltas plus a periodic full snapshot, so recovery remains bounded.
- **The unconditional tick becomes expensive** rather than merely wasteful, since serialising the fleet
  five times a second dominates. Change-driven publishing then becomes worthwhile — and it takes the
  heartbeat with it, so an explicit heartbeat message has to be added back to keep §5.7 working. Two
  decisions that currently reinforce each other come apart at scale.
- **Route geometry scales better than the fleet does**, because the on-change model already avoids
  repetition; a hundred concurrent routes sent once each is unremarkable.
- **`config` is unaffected** — thresholds and zone geometry are fixed in size.
- **Server-side filtering becomes attractive and is not a payload optimisation.** It would break the
  "one identical snapshot for every viewer" property that ADR-0004's concurrency model rests on, turning
  a single shared snapshot into per-connection derived views. That is a change to the state-sharing
  model, and it is the point at which multiple viewers stop being free.
- **SSE itself holds up fine**; the constraint is what we put through it, not the transport.
