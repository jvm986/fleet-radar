# ADR-0001 — Attention conditions and their thresholds

- **Status:** Accepted
- **Date:** 2026-09-07
- **Decides:** `DECISIONS-TO-MAKE.md` §1.1, §1.2, §1.3, §1.4, §1.5, §1.6
- **Related:** `PRODUCT-SPEC.md` §2.1, §2.4, F5, F6

## Context

The brief's operator needs reference an "issue" (N4: *send a field agent in case of an issue*)
and a charging judgement (N3: *identify vehicles that may need charging*), but the required
vehicle state (R2) contains no representation of either. Nothing in the event stream tells us a
vehicle has a problem. Both must therefore be **derived** from state we already hold.

`PRODUCT-SPEC.md` §2.4 fixed the set of conditions at exactly two — low energy and stale
information — and excluded any others. This ADR settles what those conditions mean numerically
and how they reach the operator's eye.

Two constraints shape every choice below:

- **The system observes; it does not act.** The operator cannot dispatch, reassign, or annotate
  from this view. So a distinction that does not change what the operator *does next* earns
  nothing, and costs legend space on a map that must stay readable at ~100 markers.
- **Alarm fatigue is the characteristic failure of operator tooling.** An indicator that fires
  on a third of the fleet is wallpaper. Thresholds are therefore chosen to keep the flagged set
  small enough to be acted on, not merely to be technically defensible.

## Decision

1. **A vehicle is a charging candidate below 20% energy.** A single named constant.
2. **One band, not tiers.** There is no separate "critical" level. The exact percentage is
   available on inspection.
3. **A vehicle's information is stale after two consecutive missed reports**, expressed as a
   multiple of the expected reporting interval rather than as an absolute duration.
4. **A stale vehicle stays on the map** at its last known position, with its values still shown
   but visibly marked as untrustworthy, alongside how long it has been silent.
5. **The two conditions are visually distinguishable on the map** — a single attention treatment
   that carries which condition applies, rather than a generic warning badge or two additional
   colours.
6. **Staleness subsumes low energy.** A vehicle that is both is presented as stale; both reasons
   appear on inspection.

## Options considered

### The energy threshold (§1.1)

| Option | For | Against |
|---|---|---|
| **20% — chosen** | Matches the consumer-EV mental model of "low battery", so it needs no explanation to a new operator. Leaves genuine reserve for a teledriver to reposition the vehicle to a charger under its own power rather than recovering it. In a 100-vehicle fleet a handful sit below it, so the feature demonstrably works. | Arbitrary, as the spec acknowledges. |
| 15% | Fewer flags; less noise. | Thin margin to act on. In a running system almost nothing would ever appear flagged, so the feature would look untested. |
| 30% | More lead time. | Flags roughly a third of the fleet simultaneously. Destroys the signal value of the indicator. |

### Number of bands (§1.2)

| Option | For | Against |
|---|---|---|
| **One band — chosen** | The operator's response to 18% and to 4% is identical: get it charged, or send someone. A tier that does not change behaviour is decoration. Keeps the legend and the marker vocabulary small. | Loses the genuine urgency difference between 18% and 4%. |
| Low / critical tiers | 4% really is more urgent than 18%, and an operator triaging a dozen flagged vehicles wants to know which to take first. | With observe-only and no charging locations modelled, we cannot support a differentiated response, so the tier communicates urgency we have given the operator no way to act on. Triage order is better solved by ranking than by colour. |

### The staleness threshold (§1.3)

| Option | For | Against |
|---|---|---|
| **Two missed reports, relative to expected interval — chosen** | Encodes the actual meaning ("we have missed reports in a row"). Remains correct if the reporting rate changes, because it is defined against that rate rather than against the wall clock. Reacts quickly, which matters because staleness is the field-agent signal. | Tolerates only one interval of jitter before falsely flagging. See consequences. |
| Three missed reports | More tolerant of transport jitter and scheduling hiccups. | Slower to surface the condition the operator most needs to act on. |
| One missed report | Fastest possible detection. | Any ordinary jitter strobes vehicles in and out of stale. Unusable. |
| A fixed duration in seconds | Simpler to state in the UI and to reason about when debugging. | Couples the threshold to a reporting rate that is not yet decided (§5.11). A hardcoded value silently becomes wrong the moment the rate changes — the kind of latent bug that survives a long time because nothing fails loudly. |

### Presentation of a stale vehicle (§1.4)

| Option | For | Against |
|---|---|---|
| **Keep, mark, and show silence duration — chosen** | The vehicle that has gone silent is precisely the vehicle that may need a field agent, so it is the *most* interesting vehicle on the map, not the least. Silence duration is what distinguishes "brief dropout" from "something is wrong". | Displays values that are known to be unreliable, which must be unambiguously marked or it misleads. |
| Remove from the map | Only trustworthy data is ever displayed. | Hides N4's central case. A vehicle that vanishes reads as a software bug rather than as information, and an operator cannot act on something they cannot see. |

### Distinguishing the two conditions (§1.5)

| Option | For | Against |
|---|---|---|
| **Distinguishable — chosen** | The two imply different work: low energy is scheduling, silence is a possible fault and the field-agent case. Collapsing them forces a click to learn which kind of problem it is, on exactly the vehicles where the operator is most time-pressed. | One more thing in the legend. |
| A single combined indicator | Simpler legend; one shape to scan for. | The argument is strong at five conditions and weak at two, where the information is nearly free. |

### Both conditions at once (§1.6)

| Option | For | Against |
|---|---|---|
| **Staleness subsumes energy — chosen** | Not a priority call but a logical one: a stale energy reading is not a fact. The vehicle may be at 0%, or plugged in and charging. Displaying "low battery" for a vehicle we have not heard from asserts something we do not know. | The operator loses the at-a-glance cue that this silent vehicle was also low when last heard from — mitigated by listing both reasons on inspection. |
| A third combined visual state | Represents the situation completely. | Costs a legend entry for a rare combination, and its content would be misleading for the reason above. |

## Consequences

**Positive**

- "An issue" is a closed, defined, two-item list rather than an open-ended concept. A third
  condition is a change to a list, not a new abstraction.
- Both conditions are computed from state we already hold, so no new event type, no new
  upstream dependency, and no coordination with the producer.
- Nothing in the UI claims knowledge the system lacks: no route-feasibility judgement, no
  charging locations, no assertion about a vehicle that has gone quiet.

**Negative / accepted costs**

- **Two missed reports tolerates only one interval of jitter.** This propagates a constraint
  onto decisions not yet made: the reporting cadence (§5.11) must be regular, and the event
  source (§9.3, §9.8) must not introduce delivery jitter beyond one interval unless it intends
  to trip staleness. If out-of-order or delayed delivery is simulated aggressively, false
  staleness is the expected symptom. Recorded here so it is diagnosed rather than rediscovered.
- Staleness is derived from **absence**, which makes it the only piece of operator-visible state
  not caused by an event. Something must evaluate the passage of time (§6.11). Every other value
  in the system is a pure function of consumed events; this one is not, and that asymmetry has
  to be built deliberately.
- A single energy band gives the operator no triage order among flagged vehicles.
- The threshold values are arbitrary and will be wrong for a real operation. They are stated in
  the UI (F5) so the operator knows what line is being drawn on their behalf.

## What would make us revisit this

- **Operators asking "which of these low vehicles first?"** → add ranking, not a second band.
  If ranking proves insufficient, revisit §1.2.
- **False staleness in normal operation** → the jitter tolerance is too tight; move to three
  intervals, or make the cadence more regular. Revisits §1.3.
- **Real energy data** showing 20% is not enough reserve to reach a charger in the Las Vegas
  service area under load. Revisits §1.1.
- **Charging locations or route energy being modelled**, which would turn "may need charging"
  from a threshold into a feasibility question and would likely reintroduce tiers.
- **A third attention condition** — off-route, implausible telemetry, a customer support request
  (currently descoped) — at which point the "distinguish them all visually" answer in §1.5 stops
  scaling and a combined indicator with a reason list becomes correct.
- **The system gaining the ability to act.** Every "does it change what the operator does?"
  argument above is conditioned on observe-only.

## At ~1000 vehicles

- **§1.1 (20%) holds as a definition but breaks as a workflow.** A tenfold fleet flags 100–200
  vehicles. The threshold is still the right line; the assumption that a flagged set is small
  enough to eyeball is what fails.
- **§1.2's "use ordering, not tiers" stops being sufficient.** Ordering assumes a list a human
  can read. At this size, flagging has to become aggregation — counts and density by area,
  drill-down on demand — rather than per-vehicle marks. This is the first place the attention
  design needs genuine rework, and it is a presentation change, not a change to the conditions.
- **§1.3 gets cheaper, not more expensive.** A relative threshold needs no central clock
  coordination; staleness is evaluated per vehicle from its own last-seen time. Whether that
  evaluation stays a single sweep over all vehicles is a §6.11 concern, but the *definition*
  scales unchanged.
- **§1.4 (keep stale vehicles visible) becomes more valuable, and harder.** With 1000 vehicles a
  handful of silent ones are invisible in the noise, which strengthens the case for keeping them
  but means they need to survive whatever aggregation §1.2 forces. The likely answer is that
  attention-worthy vehicles are exempt from clustering — they are the reason the operator is
  looking at all.
- **§1.5 and §1.6 are unaffected.** Both are per-vehicle presentation logic with no dependence on
  fleet size.
