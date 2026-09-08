# Fleet Radar

One operational view of a fleet of remotely-operated EVs in Las Vegas: where they are, what each is doing,
which need attention, where coverage is thin.

**Fleet Radar observes; it does not act.** The operator reaches a judgement and acts elsewhere, by radio or
phone.

---

## Run it

Prerequisites: Go, Node LTS, `make`. pnpm is pinned in `package.json`; `corepack enable pnpm` provides it.

```
make dev       # then open http://localhost:5173
make test      # Go and client suites
make check     # gofmt, vet, staticcheck, Biome, tsc — fails if the generated TypeScript is stale
make generate  # regenerate that TypeScript from the Go contract
```

CI runs `test` and `check` on every push and pull request. Map tiles are fetched at run time; if the host is
unreachable the fleet draws on a plain background, with a notice.

---

## What to look for

Times are from the seeded run.

**Immediately.** Ten vehicles under remote drive, each with a route; select one and its route is emphasised end
to end. A hatched zone is short of the vehicles it expects — the coverage layer inks only problems, so a
well-covered city shows nothing. The summary reads *1 awaiting a first report*: known from its registration,
never heard from, nowhere to draw it — which is not the same state as gone quiet.

**Ten seconds.** A vehicle goes quiet: last position kept, grey ring, `?`, silence timed in the panel. It is
never removed from the map.

**A minute or two.** One crosses 20% and gains an amber ring. Find one both quiet and low: it shows as quiet,
because an energy reading we have stopped hearing about is not a fact. The panel names both conditions; the map
shows the one that governs.

**Three minutes, then ten.** A customer takes over: purple, and the route disappears, because a customer-driven
vehicle has no plan for the system to hold — stated in the panel rather than left looking like missing data.
Later one leaves the service area and stays on the map, belonging to no zone.

**In the log, throughout:**

```
level=INFO msg=discarded reason=duplicate    ... type=VehiclePosition sequence=41
level=INFO msg=discarded reason=superseded   ... type=VehiclePosition sequence=39
```

At-least-once and unordered delivery being handled; the source duplicates about 2% of events and delays about
2%. There is no deduplication cache: an event is discarded because it is not newer than what is held.

**Failure behaviour.** Two windows show the same fleet, because state is derived once for everyone; filters and
selection are per viewer. Stop the backend and the view says it is not current, and no *further* vehicle is
marked quiet — one failed connection must not blame a hundred healthy vehicles. Filter to *with a customer* and
*not reporting*: it says the filter excluded everything, and the summary still reads the whole fleet.

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

**The simulator** stands in for a Kafka broker: a position a second, JSON, `(topic, key, payload)`, keyed by
UUID. Bytes cross the boundary, not Go structs, so validation runs in the system and not only in tests.
`backend/sim`

**Ingest** checks the type against the topic and the key against the vehicle, and drops with a logged reason.
One bad event must never stop the fleet. `backend/fleet/project.go`

**The store** is five last-write-wins registers a vehicle, sequenced per signal. Strictly-newer-wins over an
absolute value is the whole ordering model: apply an event twice, get the same state. Nothing derived is kept,
and the serving layer holds a read-only view of it. `backend/fleet/store.go`

**Every 200 ms** derivation recomputes attention, zone membership, coverage and counts from one generation of
state. Nothing cached. `backend/derive`

**The snapshot** carries vehicles, coverage, summary and lifecycle together, published whether or not anything
changed, so the client reads a missing snapshot as a lost connection and needs no heartbeat. `backend/api`

**In the browser** it lands in a module store outside React; the map takes the whole fleet in one `setData`, so
a hundred vehicles never touch the component tree. `web/src`

---

## What was traded

**Observe-only, above all else.** The brief mentions dispatching a field agent and supporting a customer trip.
Neither is built. What is built is spotting the vehicle that needs one, and enough about it to hand off.

**Go over full-stack TypeScript.** Go types are the source of truth, TypeScript generated, `make check` fails on
drift. Go is what Vay runs; the cost is two toolchains.

**Full snapshots over deltas.** Every message re-establishes complete truth: no recovery after loss, no resync
after reconnect, a slow viewer's pending snapshot replaced rather than queued. 30 KB a message.

**Derivation in the backend.** Staleness, not performance: a client watching that clock would flood the map with
false quiet markers the moment its own connection dropped.

**SSE over WebSocket.** Server-to-client is the whole of what needs modelling; bidirectionality is capability we
excluded, bought with a dependency and hand-rolled reconnection.

**Deriving rather than caching** zone membership and staleness: no invalidation logic. Coverage changes when a
vehicle is *assigned a job*, without moving a metre, so a cache keyed on position would have been silently
wrong.

---

## At a thousand vehicles

Fleet size is one constant, and the measurements are in
[ADR-0008](docs/adr/0008-scale-posture.md#measured-810).

The technical envelope reaches a thousand; the human envelope does not. ~1085 events/sec with nothing dropped,
derivation 291 µs against a 200 ms tick, 2–4% of one core in 22 MB, the client at 60 fps. The operator breaks
first: a thousand markers are a texture rather than a fleet, 48 attention rings read as wallpaper, and coverage
goes blank because only problems get ink. The wire bends first technically — 274 KB a snapshot, 11 Mbps a
viewer — and gzips to 21% of that, so content encoding is a fivefold win available before deltas.

---

## Known gaps

- **Most operator-facing criteria are verified by a person, not a test.** Testing targets what fails *silently*:
  ordering, thresholds at their boundaries, zone assignment, summary reconciliation, the four ways of knowing
  nothing. Colours and layout fail visibly. With no end-to-end suite, a wiring failure between a correct backend
  and a correct frontend needs a human to catch.
- **Nothing survives a restart.** The stream is the recovery mechanism; a restarted backend replays the roster
  and refills. That obliges honesty during warm-up, not storage.
- **Single operator.** No operator identity, territories or shared state. At a thousand vehicles this is the
  largest gap between this design and a real one, and a product change rather than an architectural one.
- **The routing is ours, not a routing engine.** Real OpenStreetMap geometry; shortest paths that ignore turn
  costs, signals and traffic.

---

## Reading further

| | |
|---|---|
| [`docs/PRODUCT-SPEC.md`](docs/PRODUCT-SPEC.md) | What is built and why, with acceptance criteria and an out-of-scope list. §7 is the product reasoning. |
| [`docs/adr/`](docs/adr/README.md) | Ten architecture decisions, reading order in the index. Later decisions are folded into the ADR each amends, marked ⚠️ **Amended in implementation**. |
| [`transcripts/`](transcripts/) | The two sessions that produced all of it: the decisions, then the code. |

If you read three: [0003](docs/adr/0003-inbound-event-model.md) for the event model everything rests on,
[0004](docs/adr/0004-ingest-state-and-derivation.md) for state and derivation,
[0008](docs/adr/0008-scale-posture.md) for scale. Deferred work is listed there with the trigger that would
prompt each change.
