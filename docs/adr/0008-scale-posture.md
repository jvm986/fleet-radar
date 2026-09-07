# ADR-0008 — Scale posture at ~1000 vehicles

- **Status:** Accepted
- **Date:** 2026-09-07
- **Decides:** `ARCHITECTURE-DECISIONS-TO-MAKE.md` §8.1–§8.12
- **Related:** every preceding ADR, and `PRODUCT-SPEC.md` §7.1, §7.2, §7.5

## Context

The brief asks us to be ready to discuss ~1000 vehicles, with no implementation required.

**This ADR is deliberately short, because scale was recorded per-decision rather than saved for the
end.** Each preceding ADR carries an *At ~1000 vehicles* section covering its own subject, and
re-deriving them here would duplicate seven sections and let them drift out of step. So this ADR does
three things only: it *ranks* the pressure points across the whole design, answers the questions that
belong to no single earlier ADR, and states how the claims are to be verified rather than argued.

## Decision

1. **The stated position is that the technical envelope reaches ~1000 vehicles and the human envelope
   does not.**
2. **Zones become the primary view** at that scale, with drill-down to vehicles, and flagged vehicles
   individually visible at all zoom levels.
3. **Territory-based supervision by multiple operators is acknowledged as the largest gap** between
   this design and a real one, and as a product change rather than an architectural one.
4. **Nothing is added to the implementation now.** Five existing decisions carry the scale story.
5. **The scale claims are to be verified by running the system at 1000 vehicles before submission**,
   and reporting what actually happened.

## The ranking

Worked through rather than asserted, because "everything breaks" is the easy answer and it is wrong.

| Component | At ~1000 vehicles | Verdict | Detail in |
|---|---|---|---|
| Ingest | ~1000 events/sec, O(1) per event, partitionable by vehicle | Comfortable | ADR-0003 |
| Backend derivation | ~10,000 point-in-polygon tests/sec over small polygons | Comfortable | ADR-0004 |
| Snapshot copy | ~5000 vehicle copies/sec | Comfortable | ADR-0004 |
| Coverage | Cost proportional to zone count, not fleet size | Unaffected | `PRODUCT-SPEC.md` §7.2 |
| **The wire** | ~220 KB per snapshot at 5 Hz ≈ **8.8 Mbps per viewer**, almost all unchanged data | Degrades; needs deltas | ADR-0005 |
| **Client main thread** | 220 KB parse plus a 1000-feature build and upload, every 200 ms | Tight; needs incremental updates | ADR-0002, ADR-0006 |
| **The operator** | 1000 markers on one screen | **Unusable** | Below |

**The operator breaks first, by a wide margin.** That is also *why* four separate decisions
independently arrived at aggregation as the first extension — attention marks (`PRODUCT-SPEC.md` §7.1),
coverage (§7.2), marker legibility (§7.5), and clustering (ADR-0002). Those were not four coincidences;
they were the same ceiling approached from four directions, and it is a human ceiling rather than a
machine one.

The technical order after that is **wire, then client, then backend** — which is worth stating because
the intuition usually runs the other way.

## Questions belonging to no earlier ADR

### Does the operator still want the whole fleet on screen? (§8.6)

No. Zones become the primary view — per-zone counts and coverage — with the map drilling down to
individual vehicles on demand, and attention-flagged vehicles remaining individually visible at every
zoom level because they are the reason the operator is looking at all.

This requires **no new concept**, which is the payoff from choosing named zones over a uniform grid.
"Downtown is two vehicles short" and "four low-battery vehicles in Downtown" are the same shape of
answer, and both survive the fleet growing tenfold unchanged.

### Does one operator still supervise the whole fleet? (§8.7)

Probably not. At this size one would expect territory-based supervision, with an operator responsible
for a subset of zones.

The system models **no operator identity at all** (`PRODUCT-SPEC.md` §7.4), so this is not a tuning
change: it needs operators, territories, and an assignment between them, plus a decision about whether
a territory filters what an operator sees or merely what they are accountable for. **This is the
largest single gap between this design and a real one**, and it is a product change rather than an
architectural one — which is worth saying plainly, because it is the gap most likely to be probed.

### What must be built now for the story to be credible? (§8.8)

Nothing. The claim is only credible if it rests on decisions already taken, so being precise matters:

1. **Per-vehicle-per-signal ordering keyed by vehicle UUID** (ADR-0003). Ingest partitions with no
   cross-partition coordination. The most important of the five.
2. **Derivation in the backend** (ADR-0001). Fleet-level work happens once rather than once per
   browser — under the alternative, the weakest machines in the system would each repeat it.
3. **Coverage as an aggregate over zones** (`PRODUCT-SPEC.md` §7.2). The one operator-facing output
   whose cost is independent of fleet size.
4. **Data-driven map layers** (ADR-0002). Rendering is identical at 100 and 1000; no rewrite is
   implied.
5. **The `FleetStore` port** (ADR-0004). Sharding or persistence substitutes behind a boundary that
   already exists.

### Does coverage mean the same thing when the fleet is ten times denser? (§8.11)

No. Per-zone minimums are **absolute counts**, so a tenfold fleet makes every zone look comfortably
covered until the numbers are revised. Expressing them as a share of fleet size would scale
automatically but would stop meaning "enough vehicles to serve this area", which is what makes the
figure useful. Absolute counts that require revision is the right trade; the point is that they *do*
require revision, and nobody should assume they are fleet-relative.

### Does the single-process assumption survive? (§8.12)

Not indefinitely. The order of departure is:

1. **The simulated source leaves first** — and it is the least interesting thing to move, because in
   production it does not exist at all: a real broker was never in the process to begin with.
2. **Ingest partitions** across consumers, which ADR-0003's ordering model already permits.
3. **The store becomes the contended resource** and shards by the same key — at which point taking a
   coherent whole-fleet snapshot spans shards, and that is the genuine complexity scale introduces.

## What we deliberately do not do now (§8.9)

Named here so each reads as a choice with a trigger rather than as an omission. The README should carry
the same list.

| Deferred | Trigger |
|---|---|
| Deltas instead of full snapshots | Snapshot size dominating the wire |
| Marker clustering, with flagged vehicles exempt | Individual selection becoming impractical |
| Incremental map updates (feature-state, custom layer) | `setData` per tick dominating the client |
| Cached zone membership | Per-tick derivation becoming measurable — invalidated on **position and status**, or it reintroduces the bug `PRODUCT-SPEC.md` §7.2 predicted |
| Server-side filtering | Not a payload optimisation; it breaks ADR-0005's single-shared-snapshot property |
| Store sharding, multi-process, partitioned ingest | The store becoming contended |
| A binary wire format | JSON parsing becoming a measurable share of CPU; reopens ADR-0001's codegen decision |
| Operators and territories | More vehicles than one person can supervise |

## Measured (§8.10)

**Run before submission, as promised, with fleet size as the only change.** One process on an Apple
M2 Pro, seeded run, 1000 vehicles, ten minutes of running, one and then two connected viewers. Method:
tick spacing and payload size read off a twelve-second capture of the SSE stream; event throughput
inferred from the discard log, since duplicates are a known 2% of all deliveries; derivation timed by a
temporary test over 200 iterations at both fleet sizes; client frame times sampled over 300 frames in
the browser.

| Component | Predicted | Measured | Verdict |
|---|---|---|---|
| Ingest | ~1000 events/sec, O(1) each | ~1085 events/sec sustained; nothing dropped | **Confirmed** |
| Backend derivation | ~10,000 point-in-polygon tests/sec | 291 µs per tick — 0.15% of the tick; 33 µs at 100 vehicles | **Confirmed**, with far more headroom than "comfortable" implied |
| Snapshot copy | ~5000 vehicle copies/sec | no measurable effect: publish spacing p50 200.0 ms, p95 201.1 ms, worst 201.7 ms | **Confirmed** |
| Coverage | cost proportional to zone count | unchanged | **Confirmed** |
| Whole backend | not predicted | ~2–4% of one core, 22 MB resident, unchanged by a second viewer | — |
| **The wire** | ~220 KB per snapshot ≈ 8.8 Mbps per viewer | **274 KB ≈ 11.0 Mbps** per viewer | Direction right, **magnitude 25% optimistic** |
| **Client main thread** | "Tight; needs incremental updates" | **60 fps held; frame times p50 16.7 ms, worst 18.4 ms; no long tasks; parse 0.33 ms and feature build 0.03 ms per snapshot** | **Wrong. Comfortable.** |
| **The operator** | Unusable | **Unusable** — see below | **Confirmed, emphatically** |

### What the measurements change

**The client estimate was wrong, and wrong for an identifiable reason.** It assumed the cost was parsing
220 KB and building a thousand features. Those together take 0.36 ms of a 200 ms budget — under a fifth of
one percent. Whatever the client's ceiling is, it is not the main thread at this size, and the ranking's
claim that the client is second to give way is therefore **unverified rather than confirmed**: the wire
degrades, and after that nothing measured here bends. The right correction is to stop asserting an order
past the wire.

**Compression comes before deltas, and that reorders the deferral table.** A snapshot gzips from 274 KB to
56 KB — 21% — taking the wire from 11.0 to 2.3 Mbps. Deltas remain the right end state, but content
encoding is a transport setting that buys a fivefold reduction while keeping every property the snapshot
model provides for free: self-healing on loss, correct coalescing for a slow viewer, and no resync after
reconnect. ADR-0005 reached for deltas without considering it.

**Three things gave way that this ADR did not rank, each predicted elsewhere and each now observed:**

- **Per-zone minimums stopped meaning anything.** Every zone read as meeting its minimum by roughly ten
  times over — the Strip at 209 available against a minimum of 21. §8.11 predicted exactly this; what the
  run adds is that the coverage feature does not degrade gracefully, it goes silent, because "only problem
  zones get ink" means a fleet ten times too large produces a blank layer.
- **The assignment target did not scale**, so ten vehicles of a thousand were working. Routes all but
  disappear from the map and no vehicle reached a customer in the observation window. ADR-0007 named this
  as a product question rather than a simulator one; at this size it is the difference between the map
  showing a working fleet and showing a car park.
- **The road graph is too small.** A thousand vehicles over 82 intersections stack about twelve deep, so
  the map reads as *artificial* rather than merely crowded — clumps at junctions rather than a spread
  fleet. ADR-0007 predicted the crowding; what it did not say is that this is what makes the 1000-vehicle
  view unconvincing as a demonstration, independently of it being unusable as an operator view.

**Attention marks became wallpaper**, as `PRODUCT-SPEC.md` §7.1 predicted: 36 low-energy and 12
not-reporting rings on screen at once, which is past the point where a flagged set can be eyeballed.

**The backend cost is not per-viewer.** Two viewers left it at the same ~2% of one core, because the
snapshot is derived and serialised once for everyone. The wire, by contrast, is per-viewer, so viewer count
multiplies the one thing already known to be the binding constraint.

### The headline stands

The stated position — the technical envelope reaches ~1000 vehicles and the human envelope does not — held
up, and by a wider margin than expected on the backend. The screenshot is the argument: a thousand markers
on one screen is not a view of a fleet, it is a texture.

## How this was to be verified (§8.10)

⚠️ **These claims are to be measured, not argued.** Fleet size is already a constant in the shared
module (ADR-0007 §7.11), so running at 1000 vehicles requires changing one value and no code.

**A run at 1000 vehicles is a task to be completed before submission**, recorded in ADR-0009 as a
verification step, with the results reported in the README. Three reasons it is worth the effort:

- It converts every row of the ranking above from reasoning into observation.
- It is a materially stronger position in the system-design discussion: *"we ran it at 1000; here is
  what gave way first"* rather than *"we believe the wire would"*.
- It protects against the case where something breaks for a reason none of this analysis predicted —
  which is the most likely way this ADR turns out to be wrong.

If the run contradicts the ranking, **this ADR is amended with the measurements rather than the
measurements being explained away.** It did, in one row, and the amendment is above.

## Consequences

- The scale discussion is grounded in per-decision analysis rather than a retrospective narrative,
  which is why this ADR can be short.
- The headline claim — human ceiling before machine ceiling — is falsifiable, and the run at 1000 is
  what tests it.
- Naming the deferred work with triggers makes the deferral reviewable. A reader can disagree with a
  trigger, which they cannot do with a silent omission.
- The operator-identity gap is stated rather than glossed, at the cost of admitting the design is
  single-operator by construction.
- Committing to a verification run means committing to publishing an unflattering result if that is
  what we get.

## What would make us revisit this

- **The 1000-vehicle run contradicting the ranking**, which amends the table directly.
- **More than one backend instance**, which breaks the single-authoritative-snapshot assumption that
  ADR-0004 and ADR-0005 both rest on, and turns "every viewer sees the same state" into a distributed
  problem.
- **Many concurrent viewers** rather than many vehicles. Nothing here is analysed against viewer count,
  and the wire cost is per-viewer, so 20 operators at 100 vehicles stresses a different axis entirely.
- **Operators and territories arriving**, which changes what a snapshot even means.
