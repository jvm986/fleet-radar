# Fleet Radar

A single operational view of a fleet of remotely-operated EVs in Las Vegas — where they are, what each
one is doing, which ones need attention, and where coverage is thin.

**Fleet Radar observes; it does not act.** The operator uses it to reach a judgement and then acts
elsewhere, by radio or phone. That one decision shapes most of the others below.

---

## Run it

Prerequisites: the Go toolchain, Node LTS, and `make`. Nothing else — no Docker, no database, no broker.
pnpm is pinned in `package.json`; if you do not have it, `corepack enable pnpm` provides it, and corepack
ships with Node.

```
make dev
```

Then open **http://localhost:5173**. The first run installs the web dependencies; after that it is a few
seconds to a running fleet.

```
make test     # Go and client suites
make check    # gofmt, vet, staticcheck, Biome, tsc — and fails if the generated TypeScript is stale
make generate # regenerate that TypeScript from the Go contract
```

An internet connection is used at run time for the map imagery. If the tile host is unreachable the fleet
still draws, on a plain background, with a notice saying so.

---

## What to look for

Most of what this system does correctly is invisible. Watching the map, you see markers moving; you cannot
see that a duplicate was suppressed, that a superseded observation was discarded, or that the summary
counted the vehicles sitting outside every zone. So it is worth two minutes of guided looking. The times
are measured from the seeded run, not estimated.

**Immediately.** Ten vehicles are being remotely driven, each with a route drawn under it; select one and
its route is emphasised end to end with a marker on its destination. One or two zones are hatched, meaning
they are short of the number of available vehicles that zone expects — the coverage layer inks only
problems, so a well-covered city shows no shading at all. The summary reads *1 awaiting a first report*:
that is a vehicle the backend knows exists from its registration and has never heard from, which is a
different thing from one that has gone quiet, and it cannot be drawn because there is nowhere to draw it.

**Within ten seconds.** A vehicle goes quiet. It keeps its last known position, gains a grey ring and a `?`,
and the panel says how long it has been silent — a vehicle that has stopped reporting is the *most*
interesting vehicle on the map, not the least, so it is never removed. Watch it come back a few seconds
later.

**Within a minute or two.** A vehicle crosses 20% and gains an amber ring and a `!`. Look for one that is
both quiet and low: it shows as quiet, because an energy reading we have stopped hearing about is not a
fact — the panel names both conditions, the map shows only the one that governs.

**Within three minutes.** A customer takes over a vehicle: it turns purple, and its route disappears,
because a customer-driven vehicle has no plan for the system to hold. That absence is stated in the panel
rather than left looking like missing data.

**Within a quarter of an hour.** A customer drives a vehicle out of the service area entirely, and it stays
on the map belonging to no zone. This one takes patience for an honest reason: the boundary is about fifteen
kilometres from where customers are picked up, so leaving the area takes as long as driving out of a city
does.

**In the log, throughout:**

```
level=INFO msg=discarded reason=duplicate    ... type=VehiclePosition sequence=41
level=INFO msg=discarded reason=superseded   ... type=VehiclePosition sequence=39
```

That is at-least-once and unordered delivery being handled. The simulated source duplicates about 2% of
events and delays about 2% of them past the next report, so both lines appear within seconds of starting.
There is no deduplication cache anywhere: an event is discarded because it is not newer than what is
already held, which is why the log says *superseded* rather than *seen before*.

**Two windows side by side** show the same fleet, because state is derived once for everyone. Filters and
selection are per viewer.

**Try to break the honesty.** Stop the backend and leave the browser open: the view says it is not current
and stops trusting itself, and no *further* vehicle becomes marked as having gone quiet — one failed
connection must not blame a hundred healthy vehicles. Then filter to *with a customer* and *not reporting* together: the map
empties, and it says the filter excluded everything rather than showing you a blank map, while the summary
keeps reading the whole fleet's numbers.

---

## One event, end to end

```
simulator ──JSON bytes──▶ consumer ──▶ validate ──▶ projection ──▶ store
                                                                    │
                                    ┌───────── 200ms tick ──────────┘
                                    ▼
                                 derive ──▶ snapshot ──SSE──▶ module store ──▶ map
                                                                          └──▶ React
```

**The simulator** is a stand-in for a Kafka broker. A vehicle reports its position once a second, and the
event is serialised to JSON and handed over as `(topic, key, payload)` with the vehicle's UUID as the key.
Bytes cross this boundary rather than Go structs, so validation and the discard path are genuinely executed
by the running system rather than only by tests. `backend/sim`

**Ingest** decodes it, checks that the type belongs on the topic it arrived on and that the key names the
vehicle it claims to, and drops it with a logged reason if not. One bad event must never stop the fleet.
`backend/fleet/project.go`

**The projection** applies it to one of five last-write-wins registers, compared by a sequence number that
is per vehicle *per signal*. Strictly-newer-wins over an absolute value is the whole of the ordering model:
applying the same event twice leaves the same state, so at-least-once needs no bookkeeping.
`backend/fleet/store.go`

**The store** holds signal values, their sequence numbers and when they were observed. Nothing derived is
kept. Writes serialise on a mutex; the serving layer holds a read-only view of it, which is what makes
"events are the only writer of state" a property of a type signature. `backend/fleet/store.go`

**Every 200 ms**, derivation turns one generation of that state into what the operator sees: which vehicles
warrant attention, which zone each is in, whether each zone has enough available vehicles, and the
whole-fleet counts. All of it is recomputed every tick and none of it is cached, because both of the
correctness bugs this design predicted for itself were caching bugs. `backend/derive`

**The snapshot** goes out as one message — vehicles, coverage, summary and the backend's own lifecycle
together, because they must describe one generation of state. It is published whether or not anything
changed, which is what makes silence diagnostic: the client reads the absence of a snapshot as a lost
connection and needs no separate heartbeat. `backend/api`

**In the browser** it lands in a module-level store outside React. The map is handed the whole fleet with
one `setData`, so a hundred vehicles moving five times a second never touch the component tree; React only
renders the summary, panel, filters and legend, and only when their own slice changes. `web/src`

---

## What was traded

**Observe-only, above all else.** The brief's user story mentions sending a field agent and supporting a
customer trip. Neither is built. What is built is the ability to *spot* the vehicle that needs one and to
get enough about it to hand off — because the system issues no commands, so every operator need is a
question of information quality. That decision removed a whole feature, and it is upstream of most of what
follows.

**Go over full-stack TypeScript.** A shared contract package would have been free; instead the Go types are
the source of truth and the TypeScript is generated, with `make check` failing when it drifts. Chosen
because Go is what Vay runs in its backend, so the submission demonstrates the relevant thing rather than
the adjacent one. The cost is two toolchains and generated code in the tree.

**Full snapshots over deltas.** Every message re-establishes complete truth, so a dropped message needs no
recovery, a reconnect needs no resync, and a slow viewer's pending snapshot can be *replaced* rather than
queued. Bandwidth is spent to buy that: about 30 KB per message at a hundred vehicles.

**Derivation in the backend, not the browser.** Driven by staleness, not performance. Staleness is the one
operator-visible state derived from *absence*, so something must watch the clock — and if that something is
the client, a dropped connection floods the map with a hundred vehicles falsely marked as having gone
quiet. In the backend, a disconnected client simply receives nothing and its flags freeze, which is the
required behaviour for free.

**SSE over WebSocket.** The product is observe-only, so a server-to-client channel is the whole of what
needs modelling. Bidirectionality would be capability we deliberately excluded, bought with a dependency
and hand-rolled reconnection.

**Deriving rather than caching**, for zone membership and staleness. Recomputing the whole fleet every tick
is wasteful by design, in exchange for having no invalidation logic — and one of the two invalidation bugs
avoided is subtle: coverage changes when a vehicle is *assigned a job*, without it moving a metre, so any
cache keyed on position would have been silently wrong.

---

## At a thousand vehicles

Run, not argued: fleet size is one constant, and the results are in
[ADR-0008](docs/adr/0008-scale-posture.md#measured-810).

The position was that the technical envelope reaches a thousand vehicles and the human envelope does not.
That held. Ingest sustained ~1085 events/sec with nothing dropped, derivation took 291 µs against a 200 ms
tick, publish spacing stayed at 200.0 ms median and 201.7 ms worst, and the whole backend sat at 2–4% of one
core in 22 MB. The client held 60 fps with no long tasks.

The operator is what breaks, and by a wide margin — the screenshot in `docs/1000-vehicles.png` is the
argument. The wire is the first technical thing to bend, at 274 KB per snapshot and 11 Mbps per viewer, 25%
worse than estimated.

Two findings worth reading the ADR for. The ranking was **wrong** about the client, which it called tight
and which is comfortable; the amendment says so rather than explaining it away. And a snapshot gzips to 21%
of its size, which makes content encoding a fivefold win available before the complexity of deltas —
something the read-path decision never considered.

---

## Known gaps

- **Most operator-facing acceptance criteria are verified by a person, not a test.** Testing here targets
  what fails *silently* — event ordering, attention thresholds at their boundaries, zone assignment
  including the no-zone case, summary reconciliation, the four ways of knowing nothing. Marker colours and
  layout fail visibly. There is no end-to-end suite, so a wiring failure between a correct backend and a
  correct frontend would be caught by a human running it.
- **Nothing survives a restart.** The stream is the recovery mechanism, so a restarted backend replays the
  roster and refills. The obligation this creates is honesty during warm-up, not storage.
- **Single operator.** No operator identity, no territories, no shared state. At a thousand vehicles this is
  the largest gap between this design and a real one, and it is a product change rather than an
  architectural one.
- **The simulated road network is a grid of arterials.** It keeps vehicles on streets and gives routes real
  geometry, but it is invented, and at a thousand vehicles it is visibly too sparse.

---

## Reading further

Depth on demand. Nobody needs to read all of this.

| | |
|---|---|
| [`docs/PRODUCT-SPEC.md`](docs/PRODUCT-SPEC.md) | What is built and why, with acceptance criteria, and an explicit out-of-scope list. §7 is the product reasoning. |
| [`docs/adr/`](docs/adr/README.md) | Ten architecture decisions. The index carries a reading order, including a three-ADR path. |
| [`docs/IMPLEMENTATION-ORDER.md`](docs/IMPLEMENTATION-ORDER.md) | What was built first and why, and the decisions taken while writing the code that no ADR covers. |
| [`docs/ARCHITECTURE-DECISIONS-TO-MAKE.md`](docs/ARCHITECTURE-DECISIONS-TO-MAKE.md) | The questions, before they had answers. Kept as the record of what was asked. |
| [`transcripts/`](transcripts/) | The session that produced all of it. |

Three ADRs if you only read three: [0003](docs/adr/0003-inbound-event-model.md) for the event model that
everything else rests on, [0004](docs/adr/0004-ingest-state-and-derivation.md) for state ownership and
derivation, and [0008](docs/adr/0008-scale-posture.md) for scale.

Deferred work is listed with the trigger that would prompt each change, in
[ADR-0008](docs/adr/0008-scale-posture.md) and in `PRODUCT-SPEC.md` §4.2. A trigger can be disagreed with;
a silent omission cannot.
