# Implementation order

The decisions are complete. `PRODUCT-SPEC.md` says what to build; `docs/adr/` says how and why. This file
says **what to write first**, and carries the obligations that would otherwise live only in someone's head.

Nothing here is a new decision. If this file appears to contradict an ADR, the ADR wins.

---

## Build sequence

Ordered so that each step can be verified before the next depends on it, and so the parts where
correctness is invisible come first — while there is nothing else to blame.

### 1. Contract and constants — `backend/contract`

The event types (ADR-0003 §3.1, §3.2), the snapshot and config shapes (ADR-0005 §5.1, §5.12), the
thresholds (ADR-0001 §1.8), and the service-area GeoJSON with `go:embed` (ADR-0001 §1.9).

Everything else imports this, and the TypeScript is generated from it. Get the shapes right before
anything depends on them.

- Absolute values only. No field is ever an increment (ADR-0003 §3.3) — this is load-bearing, not stylistic.
- Every event carries vehicle UUID, event UUID, type, observation timestamp, and a per-vehicle-per-signal
  sequence number.
- A route carries geometry, destination and route id, and **no progress field**.

### 2. Store and projection — `backend/fleet`

The `FleetStore` port with its in-memory implementation (ADR-0004 §4.2), and the projection that applies
events to it (ADR-0003 §3.4).

**This is where the correctness of the whole system lives**, and it is testable in isolation with no
transport, no rendering and no simulator.

- Strictly-newer-wins per signal. A duplicate is discarded because it is not newer — there is no dedup
  cache (ADR-0003 §3.3).
- Mutex for writes (ADR-0004 §4.4).
- The serving layer will need a **read-only** view of this. Define that interface now, or "events are the
  only writer" stops being a type-level guarantee (ADR-0004 §4.3).
- **Write the permutation-invariance test here** (ADR-0009 §9.3). Shuffle and duplicate a sequence; assert
  identical final state. If this passes, the hardest part of the system is done.

### 3. Derivation — `backend/derive`

Pure functions from a store snapshot to the operator-facing view: attention conditions, zone membership,
per-zone coverage, summary counts (ADR-0004 §4.10–§4.12).

- Nothing here is stored. Staleness and zone are derived every tick, never cached.
- Takes a **clock as a dependency** (ADR-0009 §9.12).
- "No zone" is an explicit value, and the output carries a count of vehicles outside any zone. This is the
  bug `PRODUCT-SPEC.md` §7.2 predicted; the explicit count is what prevents it.
- Coverage must respond to **status transitions as well as movement** — a vehicle assigned a job changes
  its zone's coverage without moving.

### 4. Simulator — `backend/sim`

Road graph, per-vehicle state machine, assignment scheduler, energy model, delivery layer, ticker
(ADR-0007).

Before the read path, because it is what makes steps 2 and 3 observable end to end.

- **Serialised bytes cross into ingest**, not structs (ADR-0007 §7.2).
- Registration replay completes **before** telemetry starts (ADR-0003 §3.9).
- The road graph must include nodes **outside** the service area, or out-of-area vehicles are impossible
  (ADR-0007 §7.12).
- One vehicle silent from startup, so "awaiting a first report" is observable (ADR-0007 §7.7).
- Phase-staggered emission, seeded PRNG with the seed logged.
- **Write the demo-still-demonstrates test here** (ADR-0009 §9.6).

### 5. Read path — `backend/api`

The SSE endpoint, the 200 ms tick, snapshot publication, per-client depth-one buffers (ADR-0005).

- `config` first, then current routes, then snapshots.
- Vehicles, coverage, summary and lifecycle state in **one message** — one generation, or the summary can
  disagree with the map.
- Publish unconditionally. The tick is the heartbeat; without that, client disconnection detection needs a
  separate mechanism (ADR-0005 §5.7).

### 6. Generated types and the web shell — `web/`

Wire up `make generate` and confirm `make check` fails when the generated TypeScript is stale
(ADR-0009 §9.6). Then the module store and the SSE client (ADR-0006 §6.1).

### 7. Map — `web/`

MapLibre, one symbol layer, two route layers, zone and coverage layers (ADR-0002).

- **`icon-allow-overlap` must be set**, or vehicles silently vanish in dense areas (ADR-0002 §2.3).
- Two route layers, not one with data-driven width — per-feature z-order is not controllable within a layer
  (ADR-0002 §2.7).
- No interpolation between positions.
- Camera **padding** for the detail panel, not manual offsets (ADR-0002 §2.11).

### 8. Chrome — `web/`

Summary overlay, detail panel, filters, legend, label search, the four empty states, the connection
watchdog (ADR-0006, `PRODUCT-SPEC.md` F3–F8).

- The watchdog takes a clock (ADR-0009 §9.12).
- Filters reset on load; layer visibility persists; selection may live in the URL (ADR-0006 §6.3, §6.11).
- The legend renders thresholds **received from the backend**, never local copies.

### 9. README

Last, because it quotes the 1000-vehicle measurements (ADR-0010 §10.1).

---

## Carried obligations

Four things agreed during the walkthrough that are not implementation steps and are easy to lose.

1. ~~**Run at 1000 vehicles before submitting.**~~ **Done.** Fleet size was one constant, as promised. The
   measurements are in ADR-0008 §8.10, including the row where the ranking was wrong: the client main
   thread was called tight and is comfortable. The operator ceiling was confirmed emphatically, and three
   things gave way that the ranking did not include — per-zone minimums, the assignment target, and the
   size of the road graph.
2. **Regenerate and re-scan the transcript after the session ends.** Redaction cannot be done from inside
   the session being transcribed — every pass is itself recorded — so the chain runs afterwards, and the
   shipped conversation must be rebuilt from the final session file rather than refreshed from an earlier
   copy. `transcripts/` holds only what ships; the tooling, the deny-list and the session records live in
   the untracked `.transcript-tools/`, by that same argument: they are process, not submission.
3. ~~**The `FleetStore` port must still earn its keep once written.**~~ **Done: it did not, and it was
   deleted.** Concurrency belongs to the concrete struct whether or not an interface names it, and no
   second implementation ever appeared. The read-only `Reader` interface stayed, because it crosses a
   package boundary and is what makes "events are the only writer" a type-level guarantee (ADR-0004 §4.2,
   amended).
4. **The documentation is only proportionate if the code is.** Ten ADRs against a thin codebase reads as
   over-documentation. The mitigation was explicitly placed on the implementation being substantial and
   clean, not on trimming the reasoning (ADR-0010 §10.2).

## Known-weak points, submitted knowingly

Not defects to fix silently — positions to be able to defend.

| | Where it is argued |
|---|---|
| The simulator is the largest component | ADR-0007 consequences |
| Most operator-facing acceptance criteria are verified by a human against a checklist | ADR-0009 §9.5 |

## Decisions taken while implementing

Recorded per the working ground rules: these were settled while writing the code, not during the
walkthrough, and no ADR covers them. Everything else in the code is traceable to a document.

1. **The wire carries an age, not an observation timestamp** (`silentForMs`). A timestamp would leave a
   disconnected client ageing every vehicle against its own clock, and within two intervals the map
   would blame a hundred healthy vehicles for one failed connection — the false claim `PRODUCT-SPEC.md`
   §7.6 forbids. ADR-0001 §4.6 says backend derivation gets that behaviour "for free"; sending the age
   rather than the timestamp is what "for free" actually requires.
2. **A vehicle is drawn only once position, battery and status have each been reported**; until then it
   is awaiting a first report. Arrival order across topics is not guaranteed, so partial knowledge is
   reachable, and the spec models two categories rather than three. Cost: for well under a second at
   startup, a vehicle with a known position reads as awaiting its first report.
3. **Staleness is measured against the vehicle's own telemetry only** — position, battery, status.
   Registration is excluded because a replayed roster entry is not the vehicle speaking, and route
   events are excluded because they come from the assignment system. Either would let a silent vehicle
   look fresh.
4. **Per-zone minimums live in the GeoJSON properties**, not the constants module. They are per-zone
   data, and keeping them with the geometry makes the whole zone definition reviewable in one diff.
5. **A route id is not a control-flow guard.** ADR-0003 §3.8 notes the per-signal sequence already
   covers an out-of-order clear; the id is carried into the discard log instead, which is the purpose
   the ADR actually claims for it. An id check would be unreachable code.
6. **The `Starting → Ready` lifecycle is not held in the store.** ADR-0004 §4.11 describes the read
   path serving "the snapshot plus a filling flag", and replay completion is a fact about the consumer
   rather than an event about a vehicle. Keeping it out is what lets the store stay purely
   event-written, so §4.3's guarantee needs no exception.
7. **The projector takes a clock too.** ADR-0009 §9.12 named three components that need one; this is a
   fourth, and for the same reason — ADR-0003 §3.5 asks for implausible observation timestamps to be
   logged, and a producer running ahead of the backend makes staleness unreachable while nothing on
   screen says so.
8. **Ingest validates the payload domain**, not just the envelope: a bearing is a bearing, a battery is
   a proportion of capacity, a status is one of the three. This is the discard path ADR-0003 §3.11
   specifies, tested here because ADR-0007 §7.8 keeps malformed events out of a normal run.
9. **Simulator parameters live in `backend/sim`, not the contract package.** ADR-0007 §7.13 puts them in
   "the Go constants module", and ADR-0001 §1.8's module is the one whose values are *sent to the
   client*. Drain rates and dropout chances are not part of the contract and putting them there would
   imply they were. Fleet size is still one constant, so the 1000-vehicle run is still one edit
   (ADR-0008 §8.10).
10. **The zones were enlarged, and their minimums set from measurement rather than from area.** As first
    drawn, the five districts held 30 of the road network's 82 intersections, so two thirds of available
    vehicles were in no zone and coverage described a minority of the fleet — not what "subdivided into
    named zones" should mean (`PRODUCT-SPEC.md` §2.5). Enlarged, they hold 60, and the gaps between
    them are still real, so a vehicle inside the service area and in no zone remains reachable — the
    case §7.2 called the likeliest bug in the design.

    The minimums are then measured, not reasoned: intersection counts predicted availability badly,
    because vehicles do not distribute evenly over a road network. Set against observed availability,
    about a third of moments have at least one district short — coverage stays quiet when the fleet is
    fine and inks where it is not, which is what ADR-0002 §2.9 designed for. Both ADR-0007's numbers
    that this exposed as wrong are amended in that ADR rather than only here.

11. **MapLibre is pinned to 5.x, not the latest.** `maplibre-gl@6.7.0` — the current `latest` — renders
    background layers and nothing else here: no tile requests, `load` never fires, and no error is
    raised. Reproduced with a fifteen-line map containing none of this project's code, in both the dev
    server and a production build, on a real GPU with working workers. 5.24.0 renders correctly. Taking
    `latest` from a package manager is not a decision, which is how this got in.

## The slop pass

Before submission, one rule from `PRODUCT-SPEC.md` §5: **anything in the repository that does not serve a
feature in §3 is a defect.** File by file, ask which acceptance criterion it serves. The specific things to
hunt are listed in ADR-0010 §10.8 — tooling finds unused code, but only reading finds code that is used and
pointless.
