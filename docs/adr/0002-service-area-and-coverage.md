# ADR-0002 — The service area, and what coverage means inside it

- **Status:** Accepted
- **Date:** 2026-09-07
- **Decides:** `DECISIONS-TO-MAKE.md` §1.7, §1.8, §1.9, §1.10, §1.11, §1.12, §1.13
- **Related:** `PRODUCT-SPEC.md` §2.3, §2.5, F1, F7, F8; ADR-0001

## Context

N5 — *spot areas with low vehicle coverage* — is the vaguest need in the brief and the easiest to
fake. A density heatmap looks like an answer and is not one: it shows where vehicles **are**,
leaving the operator to work out where they are **missing**, which is the entire task N5 asks us
to perform on their behalf.

"Low" is only meaningful against an expectation, and R2's vehicle state contains no such thing.
The spec resolved that the expectation is hard coded (§6.1.4) and that the fleet operates in a
single realistic Las Vegas service area (§6.1.7), but not what the expectation consists of. This
ADR settles that, along with the geography it is measured over and which vehicles count towards
it.

These seven questions are treated as one decision because they are not separable: the meaning of
"low coverage" is jointly determined by the geography, the expectation, the counting rule, and
what happens at the edges.

## Decision

1. **The service area is a single hand-authored polygon** over the real Las Vegas street grid —
   the Strip, downtown, and adjacent residential — held as a small checked-in constant.
2. **The polygon is subdivided into a handful of named zones**, each carrying a hard-coded
   minimum number of available vehicles. Coverage is assessed per zone.
3. **A zone has three coverage states:** meeting its minimum, below its minimum, and nothing
   available at all.
4. **Only FREE vehicles count as coverage.**
5. **A FREE vehicle below the energy threshold still counts as coverage.**
6. **No fourth status, and no sub-distinction within EN_ROUTE** between travelling towards a
   customer and travelling away from one.
7. **Vehicles may leave the service area.** They remain visible on the map, belong to no zone,
   and count towards no zone's coverage. Being outside the area is *not* an attention condition.

## Options considered

### Defining the service area (§1.12)

| Option | For | Against |
|---|---|---|
| **Hand-authored polygon — chosen** | Reads as Las Vegas to anyone familiar with it. No runtime fetch, no external service, works offline. Reviewable in a diff. Vehicles can be placed credibly inside a plausible operating area. | The boundary is invented and has no authority behind it. |
| Bounding box | Trivial to define. | A rectangle over Las Vegas contains desert, mountains and the airport. Those areas are *correctly* empty, so they would be flagged as low coverage permanently. This does not merely weaken the coverage feature, it inverts it. |
| Real administrative boundary | Authentic and sourced. | City limits are not a service area — no operator runs an entire municipality. Larger dependency, and the extra fidelity serves nothing we are building. |

### The coverage expectation (§1.7)

| Option | For | Against |
|---|---|---|
| **Named zones with per-zone minimums — chosen** | Operators reason in places that have names, and N4/N6 are handoff needs: "two cars short downtown" can be said aloud; a cell reference cannot. Per-zone minimums capture the operational reality that the Strip needs more vehicles than a residential edge. Small enough to put in a legend and to inspect. | Zone boundaries are invented. A vehicle just outside a zone does not count towards it, so edge effects are real and visible. |
| Uniform grid or hex bins with one global threshold | No invented place semantics, uniform arithmetic, scales without thought, familiar heatmap idiom. | "Cell G7 is low" is not actionable language. A single global threshold asserts every part of the map needs equal coverage, which is false for this geography. |
| Continuous density surface with no expectation | Needs no hardcoded numbers at all. | Answers "where are the vehicles", not "where is coverage low". Leaves the comparison to the operator, which is the work N5 delegates to us. Also inconsistent with the decision that an expectation is hard coded. |

### What counts as low (§1.8)

| Option | For | Against |
|---|---|---|
| **Below minimum, and separately zero — chosen** | A zone below target serves customers with degraded response; a zone at zero cannot serve a customer at all. Those provoke different escalations, so the distinction earns its place. | A third state in the legend. |
| A single "below minimum" state | Simpler, and superficially more consistent with ADR-0001's single energy band. | Conceals the only case that actually costs a customer a ride. |
| Continuous shading by ratio | More information per zone. | Harder to scan at a glance, and implies a precision that invented minimums do not possess. |

This appears to contradict ADR-0001 §1.2, which rejected a second energy band. It applies the same
test and reaches a different answer because the facts differ: the operator's response to 18% and
4% battery is identical, whereas the response to "short by two" and "none at all" is not. The
principle is *a distinction must change what the operator does*; only one of these two passes it.

### Which states constitute coverage (§1.9)

| Option | For | Against |
|---|---|---|
| **FREE only — chosen** | Coverage means "could serve a customer now". An EN_ROUTE vehicle is committed to a job; a WITH_CUSTOMER vehicle is in use. Neither is available. | Ignores imminent availability, so coverage reads as slightly worse than it operationally is. |
| FREE plus EN_ROUTE vehicles returning from a completed trip | Genuine operational thinking — a vehicle on its way to park is about to be available. | Requires the EN_ROUTE sub-distinction rejected in §1.11, and is predictive rather than observed. It would make coverage a forecast, which is a different feature from the one N5 describes. |

### Low-energy vehicles as coverage (§1.10)

| Option | For | Against |
|---|---|---|
| **They count — chosen** | A vehicle at 19% can still serve a customer. The energy threshold is a cue to plan a charge, not a declaration of unfitness. Keeps one problem in one feature instead of double-counting it across two. Avoids coverage flickering as batteries drift across the threshold. | Coverage may include vehicles an operator would rather not dispatch. |
| They are excluded | Coverage would mean "genuinely serviceable", which is arguably more honest. | We have no model of trip length or energy requirements — §4.2 excludes one — so declaring a 19% vehicle unable to do the job asserts knowledge we lack. Same reasoning as ADR-0001 §1.6. |

### An EN_ROUTE sub-distinction (§1.11)

| Option | For | Against |
|---|---|---|
| **No fourth state — chosen** | R2 names exactly three statuses; extending them diverges from a stated requirement. The only payoff is the predictive coverage §1.9 declined. Direction of travel is inferable from the route's destination on inspection. | The fleet summary cannot distinguish vehicles about to become available from vehicles about to become busy. |
| Add the distinction | Operationally useful, and the natural enabler if coverage should become forward-looking. | Builds state for a feature we were not asked for, and costs a legend entry on a map that must stay legible. |

### Vehicles outside the service area (§1.13)

| Option | For | Against |
|---|---|---|
| **Visible, zoneless, not an attention condition — chosen** | A customer driving a WITH_CUSTOMER vehicle can go anywhere; this is the direct consequence of the confirmed status model, so it will happen. The map must not lie about where a vehicle is. Keeping it out of the attention list preserves the closed two-item set ADR-0001 just recorded. | The operator sees a situation the UI does not explicitly characterise for them. |
| Make it a third attention condition | Arguably it *is* the N6 "trip has gone wrong" signal. | Reopening a closed list one round after closing it, for a case whose real behaviour we have not yet observed, is how a closed list becomes a dumping ground. Logged as the leading candidate instead. |
| Constrain the source so it cannot happen | Fewer cases to handle. | Discards the most realistic consequence of the teledriving model and makes the map quietly untrue. |

## Consequences

**Positive**

- **Coverage is an aggregate from the outset.** It is computed per zone, not per vehicle, which
  makes it the one operator-facing view whose cost does not grow with fleet size. See the scale
  section — this turns out to matter more than it first appears.
- The operator gets a stated expectation rather than a picture they must interpret. F7 requires
  the expectation be discoverable; per-zone minimums are nameable and displayable.
- Zone names give the operator language for handoff, which is what N4 and N6 actually need given
  the system cannot act.
- No new event types, no producer coordination: coverage is derived entirely from vehicle status
  and position, both of which we already hold.

**Negative / accepted costs**

- **Zones are our invention.** The brief says "areas"; a uniform grid is the more literal reading.
  This is the one place in this ADR where we have added a concept the brief does not mention, and
  it should be defended as such rather than presented as given.
- **Edge effects are visible.** A vehicle metres outside a zone boundary contributes nothing to
  it. With hand-drawn zones this will look wrong to an operator at least occasionally.
- **Vehicles belong to no zone in two cases** — outside the service area entirely, or, depending
  on how the zones tile the polygon, inside the area but between zones. Any code that assumes
  every vehicle has a zone will silently drop vehicles from counts. F8 requires the summary to
  reconcile with the map and treats disagreement as a defect, so this needs handling explicitly
  rather than by assumption. **This is the most likely correctness bug arising from this ADR.**
- **Coverage changes on status transitions, not only on movement.** Because only FREE counts, a
  vehicle being assigned to a job changes zone coverage without moving a metre. Any optimisation
  that recomputes coverage only when positions change will be wrong.
- Assigning a vehicle to a zone is a point-in-polygon test per vehicle. Negligible at ~100; see
  scale.
- Coverage understates near-term availability, by §1.9 and §1.11 together.

## What would make us revisit this

- **Operators talking about coverage in terms we do not model** — time-of-day demand, event
  traffic on the Strip, airport arrivals. Static per-zone minimums are the first thing that fails
  a real operation, because demand is not static.
- **Edge effects generating real confusion**, which argues for overlapping zones, or for
  distance-based rather than containment-based counting.
- **Coverage being wanted as a forecast** rather than an observation → revisits §1.9 and §1.11
  together, and the EN_ROUTE sub-distinction becomes the enabler.
- **Vehicles leaving the service area turning out to be common or serious** → promotes §1.13 to a
  third attention condition, which also revisits ADR-0001 §1.5's "distinguish them all visually"
  answer.
- **More than one service area**, or a fleet spanning cities. Every decision here assumes exactly
  one polygon and one operator responsible for all of it.
- **Real zone definitions** arriving from operations, which would replace the hand-authored
  geometry without changing anything else in this ADR.

## At ~1000 vehicles

- **This is the feature that scales best, and the reason is structural.** Coverage output is
  proportional to the number of zones, not the number of vehicles. Zone count is a property of the
  geography and does not grow when the fleet does. So the payload to the client, and the operator's
  cognitive load, stay constant while the fleet grows tenfold.
- **That makes zones the aggregation primitive ADR-0001 needs.** ADR-0001 concluded that at ~1000
  vehicles per-vehicle attention marks must become aggregation — counts and density by area with
  drill-down. Zones already are that structure. "Four low-battery vehicles in Downtown" is the same
  shape of answer as "Downtown is two cars short", and it needs no new concept. The two ADRs
  converge rather than conflict, which is a reason to prefer zones over a grid beyond the
  operator-language argument.
- **Per-zone minimums must scale with the fleet**, and probably not uniformly. They are absolute
  counts, so a tenfold fleet makes every zone look comfortably covered until the numbers are
  revised. A minimum expressed as a share of fleet size would scale automatically but would stop
  meaning "enough cars to serve this area", which is what makes it useful. Absolute counts that
  need revising are the right trade; this needs stating so nobody assumes the numbers are
  fleet-relative.
- **Zone assignment becomes worth thinking about.** Point-in-polygon for 1000 vehicles at telemetry
  rate is still small, but it is per-position-update work that grows linearly. The obvious
  mitigations — recompute only when a vehicle crosses a boundary, or precompute a coarse grid to
  polygon lookup — are available and unremarkable. This is a known-cheap optimisation, not a
  redesign.
- **More zones become desirable**, and a flat list of them stops being scannable somewhere beyond
  a few dozen. Zone hierarchy — districts containing zones — is the extension, and it preserves
  everything above.
- **§1.10 gets more consequential.** At 1000 vehicles, a larger absolute number of low-energy
  vehicles are counted as coverage, so a zone can meet its minimum on paper while being staffed by
  vehicles nobody wants to dispatch. The mitigation is to surface the composition of a zone's
  coverage rather than to change the counting rule.
