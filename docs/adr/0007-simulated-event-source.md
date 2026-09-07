# ADR-0007 — The simulated event source

- **Status:** Accepted
- **Date:** 2026-09-07
- **Related:** `PRODUCT-SPEC.md` F10, §6.1.1, §7.1; ADR-0001, ADR-0003, ADR-0004
- **Corrects:** `PRODUCT-SPEC.md` §7.1 — the staleness/jitter constraint was over-stated

## Context

Every earlier ADR placed obligations on the producer, and they all come due here. ADR-0003 requires
per-vehicle-per-signal sequence numbers, absolute values only, registration replay before telemetry,
and a regular cadence. `PRODUCT-SPEC.md` F10 requires a fleet of ~100 with ~10 EN_ROUTE, behaviour
plausible enough that the map reads as a fleet rather than noise, and the low-energy and stale
situations arising **unprompted** — because §6.2.5 removed any operator control over the simulation.

The simulator is therefore not a toy. It is the component that determines whether the rest of the
system is genuinely exercised.

## Decision

1. **The simulator runs in-process** as goroutines, behind the consumer interface.
2. **Serialised bytes cross the boundary**, not Go structs. Ingest deserialises and validates.
3. **A hand-authored road graph**, checked in, provides movement, route geometry and plausibility.
4. **The graph extends beyond the service area boundary.**
5. **Each vehicle runs the teledriving lifecycle** as a state machine, with an assignment scheduler
   holding the EN_ROUTE count near ten.
6. **Energy drains with distance plus a small idle rate, and recovers while parked below a floor.**
7. **Low battery emerges from the drain model. Staleness is modelled explicitly**, including one
   vehicle silent from startup.
8. **No deliberately-wrong vehicles and no malformed events** in a normal run.
9. **A delivery layer duplicates ~2% of events and delays ~2% by 200–400 ms.**
10. **One fine-grained ticker with phase-staggered vehicles**, each reporting at 1 Hz.
11. **A seeded PRNG, with the seed logged at startup.**
12. **Registrations are replayed as a burst before telemetry begins.**
13. **All parameters are constants in the Go constants module.**

## Options considered

### What crosses the boundary (§7.2)

| Option | For | Against |
|---|---|---|
| **Serialised bytes — chosen** | Ingest is **byte-identical** whether the source is this simulator or a real Kafka client, so substituting one changes nothing downstream. Serialisation, validation and the malformed-event discard path are all genuinely executed by the running system. This is what makes "structured as if Kafka were the source of truth" demonstrable instead of asserted. | Serialising and immediately deserialising within one process is wasted work, and slightly more code. |
| Typed Go structs passed directly | Faster and shorter. | Validation and §3.11's discard path would never execute in the running system, and a real client would present a *different* ingest contract — making the seam fictional exactly where the brief is looking. |

### The world model (§7.3, §7.4, §7.12)

| Option | For | Against |
|---|---|---|
| **Hand-authored road graph — chosen** | Vehicles stay on streets, so the map reads as a fleet rather than as noise, which F10 requires. **One artefact serves three purposes**: movement, route geometry, and visual plausibility. No dependency. | Authoring effort, and the geometry is invented. |
| Random walk or Brownian motion | Trivial to write. | Vehicles drift through buildings; the map reads as noise. Fails F10 directly. |
| Straight lines between random points | Simple, and movement is purposeful. | Paths cut across city blocks and look obviously wrong over a street basemap. |
| A real routing engine | Genuinely realistic paths. | A heavy dependency, breaking the "Go, Node and make" prerequisite, for realism nobody is assessing. |

Routes are shortest paths over the graph between the vehicle's current node and a chosen destination,
so §7.4 falls out of §7.3 rather than being a separate mechanism.

⚠️ **Amended after implementation: the geometry is real OpenStreetMap data, extracted by
`cmd/roadgen`. The routing is still ours.** The table above is two decisions wearing one row. Rejecting
a routing *engine* was right and still is — OSRM or Valhalla is a Docker dependency and a `.osm.pbf`,
and the "Go, Node and make" prerequisite is worth more than turn costs. But rejecting the engine never
required inventing the *data*, and the row's "the geometry is invented" cost was accepted as though it
did. `roadgen` queries Overpass by hand, converts the result to the same artefact the simulator already
loaded, and commits it: no runtime dependency, no new toolchain, nothing for a reviewer to configure,
and Dijkstra still ours.

The scale changes by two orders of magnitude — **80 hand-authored intersections become 22,398
intersections and 29,429 roads**, 2.2 MB committed — and three things follow that the authored grid
could not express:

- **Roads carry their own shape.** A route step is a polyline, so a vehicle rounds a bend instead of
  cutting the corner, and the route drawn for the operator lies along the street it is drawn over.
- **One-way roads are modelled**, because the data says which they are: **79% of roads in this
  extract**, which is not an artefact but Las Vegas — its arterials are overwhelmingly divided
  carriageways, and OSM tags each direction separately. A vehicle returning along a street now uses
  the other carriageway, as it would.
- **Dijkstra needs a heap**, and stops once the frontier passes the journey's maximum distance. §7.3's
  linear scan was justified by "under a hundred intersections and once per assignment", which was true
  and is now false — the ADR named this as the trigger, and it fired.

**Two earlier amendments dissolve.** The warning below about remembering to author nodes outside the
polygon, and the western exit added because the graph originally crossed the boundary once: the real
network leaves the service area in every direction, 6,019 of its intersections are already outside it,
and `LeavesServiceAreaChance` came *down* from 0.4 to 0.15 as a result. Measured over an hour: first
departure at **6.9 minutes** against 15 before, at most four vehicles outside at once. The old constant
had to be high because the wait was the drive, not the dice; now the dice is the constraint again.

**What is lost is worth naming, because it was the authored file's best property.** `network.json` read
as a list of streets, and an authoring mistake — a road claiming a crossing that does not exist — was a
load-time panic. Generated data can be neither. So the invariants moved into `roadgen`, which keeps
only the largest set of intersections that can all reach one another (a bbox cuts roads mid-street and
one-way tags make reachability directional, so the raw extract contains dead ends a vehicle could drive
into and never leave), and refuses to emit a network with no intersections outside the service area, or
a zone containing none. The loader still range-checks every index. The reachability test that used to
route from all 80 intersections is now two flood fills, forward and backward, which is the same claim
in time that scales.

**It is a derived database under ODbL**, so the artefact carries its attribution, licence, the exact
Overpass query and the date in a `source` block. The basemap was already OpenStreetMap, so the
obligation is not new, but a committed extract makes it ours to state rather than the tile host's.

⚠️ **The graph must include nodes outside the service area polygon.** WITH_CUSTOMER vehicles follow the
graph along a path the system never learns about — no route event is emitted, which is precisely what
that status means — and occasionally head for a node beyond the boundary. If every node sat inside the
polygon, the out-of-area behaviour that ADR-0002 and `PRODUCT-SPEC.md` both specify could never occur,
and a specified behaviour would be undemonstrable. Easy to author, easy to forget.

⚠️ **Amended in implementation: the fleet is placed and dispatched inside the service area; only a
customer's trip may end outside it.** The requirement above is right, but satisfying it by placing the
fleet over the whole graph meant about six vehicles started *parked* outside the boundary. That makes a
specified state observable immediately and for the wrong reason: a vehicle parked outside the area at
startup reads as a bug rather than as information, and it is not how a vehicle is supposed to get there.
Initial placement and dispatch destinations are therefore drawn from nodes inside the polygon, and a
vehicle leaves only by driving a customer out. The graph also gained a western exit — it originally
crossed the boundary in one place only, which made leaving the area a twenty-kilometre drive and put the
first out-of-area sighting 26 minutes into a run.

### Vehicle behaviour (§7.5, §7.6)

The state machine is the teledriving lifecycle itself:

```
FREE ──assigned──▶ EN_ROUTE (to pickup) ──arrives──▶ WITH_CUSTOMER
  ▲                                                       │
  └──arrives── EN_ROUTE (to parking) ◀──customer done──────┘
```

This is not incidental. It is the domain model confirmed in `PRODUCT-SPEC.md` §6.1.1, and route
clearing on reaching the customer falls out of it naturally, because a customer-driven vehicle has no
plan for the system to hold.

**An assignment scheduler keeps the EN_ROUTE count near ten**, dispatching FREE vehicles whenever it
drops. The alternative — a per-vehicle probability of being dispatched — would produce ten *on average*
with visible excursions. A scheduler makes the brief's stated condition an invariant.

⚠️ **Amended in implementation: the return leg is assigned by the scheduler rather than entered on
arrival.** As drawn above, a finished customer trip becomes the EN_ROUTE-to-parking leg directly — and
that transition is not the scheduler's, so the invariant it exists to hold does not hold. Measured, the
count settles at ten dispatched plus everyone driving back: **sixteen, not ten.** So the customer's
drop-off leaves the vehicle parked and available where it stands, and the return journey is dispatched
like any other. Both meanings of EN_ROUTE the spec gives — towards a customer, or away again — still
occur, every transition into it is now the scheduler's, and the count holds at ten. It is also the more
realistic model: a vehicle idle at a drop-off is available to the next customer near it, not obliged to
drive back empty first.

⚠️ **Energy recovery exists because without it the fleet dies.** Charging locations are out of scope, so
drain alone means every vehicle reaches zero and a demo left running for half an hour ends with a dead
fleet. A vehicle below roughly 5% while FREE therefore stays parked and its battery rises. It remains
FREE because no charging status exists and the spec excludes inventing one; the operator sees a battery
increase, which is honest, since a parked vehicle being charged is exactly what would happen. Recorded
because "battery sometimes goes up" is otherwise the kind of behaviour that reads as a bug.

### Making the interesting states occur (§7.7, §7.13)

**Low battery needs no mechanism** — a spread of starting levels plus distance-proportional drain means
vehicles cross 20% within a couple of minutes.

**Staleness must be modelled explicitly**, because nothing else in the simulation would cause silence: a
small per-vehicle chance of a 5–30 second silent period, tuned so one to three vehicles are quiet at any
moment.

⚠️ **One vehicle is silent from startup.** Otherwise "registered but never reported" — the state that
required amending three spec features in ADR-0004 — would never be observable. A specified state that
cannot be seen is worse than one that does not exist.

**No deliberately-wrong vehicles.** An off-route vehicle would produce a visual artefact about which the
UI makes no claim, since deviation detection was declined, and a reviewer would reasonably read it as a
bug.

**No malformed events in a normal run**, despite the temptation. They would prove §3.11's discard path in
the running system, but at the cost of the log *always* carrying warnings — which makes a healthy system
look broken. Unit tests cover that path instead. Duplicates and reordering are different: they are
silent and correct by design, and their handling surfaces in the log as `discarded: duplicate` and
`discarded: superseded`. **That is the demonstration**, and it is consistent with logs being the whole of
our observability.

### Delivery imperfection (§7.8) — and a correction

Per-event duplication at around 2%, and delay of around 2% of events by 200–400 ms.

⚠️ **`PRODUCT-SPEC.md` §7.1 warned that aggressive delay simulation would cause false staleness. That is
only true of uniform delay, and the distinction changes how this is tuned.**

Staleness compares the clock against the **newest observation timestamp received**. Delaying event *N*
does not delay event *N+1* — so the late event arrives, is discarded as superseded, and staleness is
unaffected because a newer observation already landed on time. Only delaying *every* event, or a mean
delivery lag approaching the threshold, trips it.

So the real constraint is narrower than recorded: per-event delay applied to a subset is safe at any
plausible probability; uniform delay is not. The spec note has been corrected rather than left to
misdirect a future implementer into tuning conservatively for the wrong reason.

⚠️ **Amended in implementation: 200–400 ms is too short to reorder anything, so the window is
1.5–2.5 reporting intervals.** The reasoning above is right and the number contradicts it. A delayed
event is only out of order if something overtakes it, and the next observation of the same signal is a
whole reporting interval away — so at 1 Hz a 400 ms delay still arrives first, is applied as the newest,
and demonstrates nothing. Measured: duplicates appeared in the log and superseded observations never
did.

The window is therefore expressed as a multiple of the reporting interval rather than in milliseconds,
which is the same argument the staleness threshold is expressed in reports: a flat duration silently
stops meaning anything the moment the cadence moves. The safety argument is unaffected — only 2% of
events are held, so a newer observation still lands on time and staleness never trips.

### Cadence mechanics (§7.14)

| Option | For | Against |
|---|---|---|
| **One fine-grained ticker, vehicles phase-staggered — chosen** | Uniform 1 Hz per vehicle with no drift between vehicles, and a smooth ~100 events per second rather than a burst. Closer to how a real fleet reports. | One scheduling loop to get right; a vehicle's phase must be stable. |
| One ticker emitting all vehicles each second | Simplest, perfectly uniform. | 100 events in an instant then silence — a spiky load that makes ingest behaviour under burst the normal case rather than an edge case. |
| A goroutine and timer per vehicle | Conceptually clean, each vehicle owns its cadence. | Timers drift and jitter independently, and ADR-0001's threshold tolerates only one interval of it. |

### Mechanics (§7.9, §7.10, §7.11)

A **seeded PRNG with the seed logged** — a simulation that cannot be reproduced cannot be debugged, and
the cost is one log line.

**Registration replayed as a burst before telemetry**, satisfying ADR-0003's ordering requirement. From
the client's perspective: connect, `config`, a snapshot of ~100 registered vehicles mostly awaiting a
first report, then all positioned within about a second.

**All parameters as constants** in the shared module, consistent with ADR-0001 §1.8. The simulation is
hidden from the client, not from the code.

⚠️ **Amended in implementation: the parameters are constants in `backend/sim`, not in the shared module.**
"The shared module" was the wrong module. ADR-0001 §1.8's module holds the values that are *sent to the
client*, and drain rates, dropout chances and delay windows are not part of the contract — putting them
there would imply they were, and would place the simulation's tuning in the very payload the operator's
legend is rendered from. Fleet size remains one constant, so the 1000-vehicle run is still one edit
(ADR-0008 §8.10). "Hidden from the client, not from the code" is unaffected.

## Consequences

**Positive**

- The ingest path is exercised exactly as a real broker would exercise it, including validation and
  discards, so the Kafka seam is real rather than notional.
- The road graph makes movement, routes and plausibility one problem with one artefact.
- The state machine *is* the domain model, so the simulator documents the domain as well as driving it.
- Every operator-facing state in the spec is reachable in a running system within a short observation
  window — including the two that only exist because of earlier decisions (out-of-area, and awaiting a
  first report).
- Correct handling of duplicates and reordering is visible in logs without polluting them with
  synthetic errors.
- A seeded run is reproducible.

**Negative / accepted costs**

- **Pointless serialisation** in-process, accepted to keep the seam honest.
- ~~**The road graph is hand-authored**, so it is both effort and invented data, and its coverage
  determines what the simulation can express — including whether out-of-area is possible at all.~~
  **Superseded:** the geometry is real and generated. The cost that replaces it is a 2.2 MB artefact in
  the repository, and a regeneration that has to be a deliberate act — OpenStreetMap changes daily, so a
  build-time fetch would make two clones of the same commit disagree about the world.
- **Energy recovery is a fiction the operator can see.** It is plausible, but it is not derived from any
  modelled charging infrastructure.
- **Staleness is synthetic.** Nothing about the simulated world causes silence, so the rate of it is a
  tuned constant rather than an emergent property.
- **The malformed-event path never runs in a normal session**, so its correctness rests entirely on
  tests.
- **The simulator is a substantial component** — graph, state machine, scheduler, energy model, delivery
  layer, ticker — and every part of it is code a reviewer may reasonably ask why we needed.
- Tuning is interdependent: drain rate, starting battery spread, dropout probability and assignment rate
  all have to be jointly plausible, and a change to one can make an operator-facing state stop occurring.

## What would make us revisit this

- **A real Kafka broker**, which removes the delivery layer and the in-process serialisation entirely,
  and makes §7.2's byte boundary the actual boundary.
- **Real recorded telemetry** replacing the simulation, which would remove the graph, the state machine
  and the energy fiction at once, and would make ADR-0003's route-deviation decision worth reopening.
- ~~**The graph proving too small** to make movement look plausible, or too small to place vehicles across
  every zone — which would make coverage untestable in parts of the map.~~ **This happened, and is
  resolved above.** The smallest zone now contains 1,492 intersections.
- **Charging being modelled**, which replaces the energy-recovery fiction with a real mechanism and
  changes what "may need charging" means product-side.
- **Any operator-facing state becoming unreachable** after tuning, which is a regression in the
  simulator even though no product code changed. This is the argument for testing that each state
  occurs.

## At ~1000 vehicles

- **The simulator becomes the most expensive component in the process**, which is a fair reflection of
  reality: in production it does not exist at all, and a real broker sits outside the process entirely.
  So the first thing to move out under scale is also the least interesting to move.
- **Event production reaches ~1000/second**, and the phase-staggered ticker handles that without change —
  it was chosen partly for that reason, since a burst model would present ingest with a 1000-event spike
  every second.
- **In-process serialisation stops being negligible.** At a hundred vehicles the wasted encode/decode is
  invisible; at a thousand it is real CPU spent purely to keep a seam honest, and the honest answer at
  that point is to make the seam real by putting a broker behind it.
- ~~**The road graph needs to grow**, or a thousand vehicles will be visibly stacked on a small number of
  edges — which would make the map read as artificial in a way a hundred vehicles conceal.~~ **No longer
  the constraint:** 29,429 roads absorb a thousand vehicles without stacking. What replaces it is
  routing cost — a thousand vehicles is a thousand journeys to plan, and the distance-limited Dijkstra
  is what makes that affordable. The next thing to reach for is caching routes between the same pair of
  intersections, which at that fleet size stops being a coincidence.
- **The assignment scheduler needs its target scaled** — ~10 EN_ROUTE out of 100 is a tenth of the fleet
  working; whether the ratio or the absolute number is the right thing to hold is a product question,
  not a simulator one.
- **The seeded-run property becomes more valuable**, because emergent problems at that scale are much
  harder to reproduce by chance.
