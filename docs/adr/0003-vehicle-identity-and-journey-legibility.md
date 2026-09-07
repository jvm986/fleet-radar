# ADR-0003 — Vehicle identity, and making a journey legible

- **Status:** Accepted
- **Date:** 2026-09-07
- **Decides:** `DECISIONS-TO-MAKE.md` §1.14, §1.15, §1.16
- **Related:** `PRODUCT-SPEC.md` §2.1, §2.2, F2, F3; ADR-0001, ADR-0002

## Context

Two loosely related questions close out the product semantics.

**Identity.** N4 and N6 are *handoff* needs: because the system observes and does not act
(`PRODUCT-SPEC.md` §6.1.2), the operator's output is always a sentence said to somebody else.
Whatever identifies a vehicle has to survive being spoken aloud. But identity is also the key
events are addressed to, which is a machine concern with different requirements.

**Journey legibility.** F2 draws the route of every remotely-driven vehicle. A polyline alone
does not say which end is the destination, nor how far through the journey the vehicle is — and
`PRODUCT-SPEC.md` §2.2 lists both as things the operator needs. The temptation is to compute an
ETA, which the system has no basis for.

## Decision

1. **A vehicle has two identifiers: a UUID and a human-readable label.** The UUID is the
   identity — it is what events are addressed to, what the client keys on, and what never
   changes. The label is what the operator reads and says.
2. **Journey progress is conveyed by marking the route's destination and letting the vehicle's
   position on the route speak for itself.** No percentage, no progress bar, no ETA.
3. **There is no route-deviation detection.** A vehicle away from its route is visible because
   both are drawn; the system makes no claim about it.

## Options considered

### Vehicle identity (§1.14)

| Option | For | Against |
|---|---|---|
| **UUID as identity, plus a human label — chosen** | Separates the machine concern from the human one, so each can be right on its own terms. The wire contract never depends on a display string, which means the label can be changed, corrected or re-badged without touching identity or invalidating anything already delivered. A UUID is also the natural partition key if the stream is ever really Kafka (§5.13), and the natural basis for duplicate detection (§5.3). | Two identifiers to keep consistent, and a standing question in every piece of code and every log line about which one is appropriate. |
| A single human-readable call sign | One identifier, nothing to keep in sync, speakable, sorts naturally. | Makes a display string load-bearing on the wire. Renaming a vehicle then becomes a data-migration problem rather than a label change. |
| UUID only | Unambiguous and unique. | Unusable for the one thing the operator must do with an identifier — tell somebody else. Fails N4 and N6 directly. |
| Fabricated registration plate | What a field agent actually reads off the vehicle on arrival. | Invented plates add realism serving nothing here, and fleets track their own asset identifiers internally regardless. |

An earlier draft of this decision argued for a single identifier on grounds of restraint. The
wire-contract argument is stronger: coupling a delivered event's addressing to a human-facing
string is the kind of decision that is cheap now and expensive to unpick, and the second
identifier costs only discipline.

### Conveying journey progress (§1.15)

| Option | For | Against |
|---|---|---|
| **Destination marker plus the vehicle's position on the route — chosen** | The operator's questions are spatial — *is it near the customer, has it barely set off* — and a marker on a line answers them directly. The destination marker supplies the one thing a bare polyline cannot: which end is the goal. No projection arithmetic, no invented precision. | Progress is read off the map rather than stated, so it cannot be filtered or sorted on. |
| Fade the travelled portion, emphasise the remainder | Clearer still, and encodes direction of travel in the line itself rather than in a separate marker. | Requires projecting the vehicle's position onto the polyline to know where "travelled" ends. Worth adopting as a refinement *if* the map proves ambiguous in practice; not worth pre-empting. |
| Numeric percentage or progress bar | Precise, trivial to compute from a point-along-line fraction. | "60% complete" is operationally meaningless — 60% of what distance, through what traffic. Asserts a precision the system does not have, and sits in a panel rather than on the map where the operator is looking. |
| An estimated time of arrival | What operators genuinely want to know. | Requires speed and traffic modelling the system does not have, and `PRODUCT-SPEC.md` §4.2 excludes feasibility reasoning. A confidently wrong ETA is worse than none, because the operator will plan against it. |

### Route deviation (§1.16)

| Option | For | Against |
|---|---|---|
| **No detection — chosen** | The route and the vehicle are both drawn, so a vehicle off its line is *visible* without the system asserting anything. Consistent with ADR-0002 §1.13: draw the truth, do not claim a diagnosis we cannot justify. | Looks like an omission to anyone expecting deviation alerting, and leaves the closest available proxy for N6 unexploited. |
| Detect deviation beyond a tolerance and flag it | The nearest thing in the system to "something went wrong with this trip", which is N6's actual trigger. | Requires choosing a distance tolerance and a position-noise model, neither of which we can justify, over simulated position data where being off-route is an artefact rather than a signal. Would also be a third attention condition, having declined a third condition twice already. |

## Consequences

**Positive**

- Identity is stable and machine-appropriate; presentation is human-appropriate. Neither
  compromises for the other.
- Keying on a UUID gives §5.3 (duplicate recognition) and §5.13 (partitioning) a natural answer
  rather than requiring one to be invented later.
- Journey legibility costs no computation beyond drawing what we already receive, and makes no
  claim the system cannot support.
- The attention list stays closed at two conditions across three consecutive rounds, which is
  what makes it a definition rather than a habit.

**Negative / accepted costs**

- **Two identifiers is a standing discipline.** The client must key on the UUID and display the
  label; getting this backwards produces bugs that only appear when a label changes, which in a
  demo is never — so the correctness of it will not be exercised by running the system. Logs and
  error messages need to carry the label to be useful to a human and the UUID to be useful to a
  developer.
- The operator cannot filter or sort by journey progress, because it is not a value anywhere —
  it is an emergent property of the picture. If that is ever wanted, §1.15 has to be revisited
  first.
- **No deviation detection means the system is silent on its most operationally interesting
  event.** A vehicle stopped off-route for a long time is exactly what N4 exists for, and the
  operator will only catch it by looking. Accepted on the grounds that a threshold we invent is
  worse than an honest absence, but it is a real gap rather than a costless one.
- Destination markers add one map object per remotely-driven vehicle. Immaterial at ~10; see
  scale.

## What would make us revisit this

- **Labels needing to be unique across cities or fleets**, which changes the label scheme but,
  by design, nothing else.
- **The map reading ambiguously about direction of travel** → adopt the travelled/remaining fade
  from §1.15's second option.
- **Real telemetry replacing simulated positions**, which turns off-route from an artefact into a
  signal and makes deviation detection justifiable. This is the single most likely trigger for
  reopening §1.16, and it would arrive alongside a defensible noise model.
- **Operators asking which of several en-route vehicles will arrive first** → this is an ETA
  request, and it needs speed data before it can be answered rather than guessed.
- **Anything wanting to filter or sort on progress**, per the consequence above.

## At ~1000 vehicles

- **The label scheme must be sized for the fleet, and this is a real trap.** A three-digit label
  runs out at 1000 vehicles — precisely the number the brief asks us to be ready for. Either the
  scheme carries enough digits from the start or it needs re-badging exactly when the fleet reaches
  the discussed scale. The UUID is unaffected, which is part of why separating them is right: the
  label can be widened without touching identity or any delivered event.
- **The UUID-as-key decision pays off rather than costs.** Partitioning an event stream by vehicle
  UUID is what makes ingest horizontally scalable at all (§10.2), and a UUID distributes across
  partitions evenly by construction where a sequential label would not.
- **Destination markers become clutter.** Roughly 100 concurrent journeys means 100 destination
  markers, which competes with the vehicles themselves. The fix is to draw the destination marker
  only for the emphasised route rather than for all of them — the same
  faint-versus-emphasised structure F2 already establishes for the routes, extended to their
  endpoints. No new concept required.
- **§1.16's absence gets harder to defend.** Spotting a vehicle off its route by eye is plausible
  at ten concurrent journeys and impossible at a hundred. Automated deviation detection moves from
  "an invented threshold we do not need" to "the only way this is noticeable at all". Scale is
  therefore an independent trigger for revisiting §1.16, separate from telemetry quality.
- **§1.15 is otherwise unaffected**, being per-vehicle presentation with no dependence on fleet
  size.
