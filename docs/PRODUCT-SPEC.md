# Fleet Radar — Product Spec

Status: **signed off.** The brief's ambiguities and the gaps this document opened were resolved on
2026-09-07 (§6). The product decisions that followed, with their reasoning, are in §7. Technical
decisions live in `docs/adr/`; open questions live in `DECISIONS-TO-MAKE.md`.

Source material: `docs/Full-Stack_Engineer_at_Vay_-_Take-Home_Coding.pdf` (the brief) and
`docs/email.txt` (the covering email).

---

## 1. What this is

A web-based **Fleet Radar**: a single operational view that lets one fleet operator answer
questions about a fleet of remotely-operated EVs in real time, the way a flight radar lets a
controller answer questions about aircraft.

The brief states the goal is to demonstrate full-stack capability, event-driven thinking,
real-time state handling, pragmatic architecture, and operator-focused UX thinking — with clarity
over polish. It also says the user story is there to *inspire design thinking* and "does not need
to be fully implemented", but that the solution should reflect operator-oriented decision making.
This spec therefore treats the operator needs as the things the product must make **answerable**,
and separates them from the functional requirements the product must **literally do**.

**Fleet Radar observes; it does not act.** The operator uses it to reach a judgement and then acts
elsewhere — by radio, phone, or a dispatch tool that is not this one. The system issues no commands
and emits no events outward. Every operator need is therefore a question of information quality,
and every proposed distinction in the UI is tested against whether it changes what the operator
*does next*.

### 1.1 The operator's needs (verbatim from the brief)

| ID | Need |
|----|------|
| **N1** | Understand where cars are located |
| **N2** | See what each car is currently doing |
| **N3** | Identify vehicles that may need charging |
| **N4** | Send a field agent in case of an issue |
| **N5** | Spot areas with low vehicle coverage |
| **N6** | Support a customer trip if something goes wrong |

Given observe-only, N4 and N6 are served by the operator being able to *spot* the vehicle that
needs a field agent or the trip that has gone wrong, and to get enough about it to hand off.

### 1.2 The functional requirements (verbatim from the brief)

| ID | Requirement |
|----|-------------|
| **R1** | Displays approximately 100 vehicles on a map |
| **R2** | Each vehicle has live state: location (lat/lng), direction / heading, battery percentage, status (FREE, WITH_CUSTOMER, EN_ROUTE) |
| **R3** | Vehicles in EN_ROUTE status display the route they are following |
| **R4** | The UI updates as vehicle state changes |
| **R5** | Includes simple operator-oriented UX decisions (legend, selection, filtering, stale indicator, etc.) — our choice |
| **R6** | Backend structured as if Kafka events are the source of truth; vehicle telemetry and route assignments/updates both arrive as events; data flow modelled accordingly |

### 1.3 Given conditions

- Fleet size ~100 vehicles, operating in **Las Vegas**, within a single service area.
- ~10 vehicles simultaneously EN_ROUTE at any given time.
- Must run locally.
- Design should be discussable at ~1000 vehicles; no implementation of that required.

### 1.4 Stated permissions, and what we chose

The brief said in-memory storage was *acceptable*, a simulated event source *sufficient*, no
authentication *required*, and technology choices *fully open*. These were treated as removals of
constraint rather than directives, and each was decided explicitly: no persistence (§7.4), no
authentication (§7.4), a simulated source hidden from the client (§6.2.5). Technology choices
remain open in `DECISIONS-TO-MAKE.md`.

---

## 2. Domain model

Described as **the information the operator needs in order to act** — deliberately not as a
schema. Field names, how information is grouped into messages or records, and the granularity of
any timestamp are technical decisions and are out of scope for this document.

### 2.1 The vehicle

For any single vehicle, the operator needs to know:

- **Which vehicle this is.** Two identifiers serve two different purposes. A stable machine
  identity, which events are addressed to and which never changes; and a short human-readable
  label the operator can read off the screen and say aloud on a radio. Because the system cannot
  act, the operator's output is always a sentence said to somebody else, so speakability is
  functional rather than cosmetic.
- **Where it is** — precisely enough to place it on a map, and to tell whether it is on a road, in
  a depot, or somewhere it should not be.
- **Which way it is pointing** — a stationary vehicle's heading tells the operator which way it
  will leave; a moving vehicle's heading tells them whether it is going where they expect.
- **What it is doing right now** — one of the three states in R2, which mean:
  - **FREE** — parked and available. Nobody is driving it.
  - **EN_ROUTE** — a remote driver is driving it along a planned route, either towards a customer
    or away again once the customer is done.
  - **WITH_CUSTOMER** — the customer is driving it themselves. This is why there is no route to
    display: the vehicle's path is the customer's choice, not a plan the system holds.
- **How much energy it has left** — as a proportion of capacity, because that is what R2 specifies.
- **How current this knowledge is** — every other item above is a *last known* value, not a fact.
  The operator must be able to tell "this vehicle is parked" from "this vehicle stopped telling us
  anything a while ago". Without this the rest is untrustworthy, which undermines every need.
- **Whether it currently warrants attention**, and why — see §2.4.

The status definitions are the confirmed reading of the brief (§6.1.1). They matter beyond naming:
a WITH_CUSTOMER vehicle's next position is genuinely unpredictable, and no planned path for it
exists to be shown or reasoned about.

### 2.2 The journey a vehicle is on

Only a vehicle under remote control (EN_ROUTE) has a journey the system knows about. For such a
vehicle the operator needs to know:

- **Where it is trying to get to** — identifiable on the map, so the two ends of a drawn path are
  not ambiguous.
- **The path it intends to take** — so it can be drawn (R3), and so the operator can see whether
  the vehicle is actually on it.
- **How far through the journey it is** — the difference between "just set off" and "nearly there"
  changes what the operator does about a problem. This is a spatial judgement made from the
  vehicle's position along its drawn path, not a figure the system asserts.

A journey may be revised while in progress; the operator needs the current intent, not the
original one.

### 2.3 The fleet as a whole

- **The shape of the fleet right now** — how much is busy, available, warranting attention, or
  gone quiet. A property of the whole fleet, unaffected by what the operator has chosen to look
  at (§6.2.6).
- **How the fleet is distributed** across the geography they are responsible for, measured against
  a fixed expectation of where vehicles ought to be available — the substance of N5.

### 2.4 A vehicle that warrants attention

The brief's needs reference an "issue" (N4) but R2's state model contains no such concept. A
vehicle warrants attention when **either** of exactly two things is true, and no others:

- **Its energy is low** — below **20%** of capacity.
- **Its information has gone stale** — it has missed **two consecutive expected reports**, so
  nothing else the operator can see about it can be trusted.

Both are derived from state the system already holds; neither is reported to us as a problem. The
two are distinguishable on the map, because they imply different work: low energy is scheduling,
silence is a possible fault and the field-agent case. Where both apply, the vehicle presents as
stale — not because staleness ranks higher but because it *invalidates* the energy reading.

This list is closed. Adding a third condition is a change to a defined list rather than the
introduction of a new concept, which is the point of defining it this way.

### 2.5 The service area, and coverage within it

The operator is responsible for a **single service area** in Las Vegas, subdivided into a small
number of **named zones**. Zones exist because the operator's output is spoken: "two cars short
downtown" can be said on a radio, where a grid reference cannot.

For each zone the operator needs to know **whether it currently has enough available vehicles**,
against a fixed expectation of how many that zone needs. Zones differ — a busy central zone needs
more than a residential edge. A zone is in one of three conditions: meeting its expectation, below
it, or with nothing available at all. The third is distinguished from the second because a zone
below target serves customers with degraded response while a zone at zero cannot serve them at all,
and those provoke different escalations.

Only vehicles that could serve a customer *now* constitute coverage: parked and available ones. A
vehicle being driven — by a remote driver or by a customer — is committed and is not coverage. A
low-energy available vehicle still counts, because the energy threshold is a cue to plan a charge,
not a declaration that the vehicle cannot do a job.

Vehicles may leave the service area, because a customer driving one may go anywhere. Such a vehicle
remains visible and belongs to no zone.

### 2.6 Concepts referenced by the needs but absent from vehicle state

R2 defines the complete live state of a vehicle and contains no representation of the following,
yet N3, N4, N5 and N6 each depend on one. Recorded with how each was resolved, because the
resolution is the reason several features are as thin as they are.

- **A charging opportunity.** Resolved: "may need charging" is a judgement about the energy figure
  against a fixed threshold. It is not a claim about whether the vehicle can finish what it is
  doing, and the UI must not imply otherwise.
- **An issue.** Resolved: exactly low energy or stale information (§2.4), surfaced through
  indicators and filters, with no prompting and no workflow.
- **A field agent.** Resolved: out of scope — observe only. The system's job ends at making the
  vehicle findable and describable.
- **A customer trip.** Resolved: a trip *is* the WITH_CUSTOMER state. No customer, origin,
  destination, or expected end. A customer requesting support is a plausible future event source
  and is descoped.
- **A service area and expected coverage.** Resolved: §2.5.

---

## 3. Features

### F1 — Live fleet map

*Serves: N1, N2, N3 · R1, R2, R5*

The map occupies the full window. Nothing else holds a permanent claim on screen space.

**Acceptance criteria**
- Given the fleet is being reported, the operator sees every vehicle positioned on a map of the
  Las Vegas service area.
- Given a vehicle's reported position changes, its position on the map changes to match.
- **Each vehicle is drawn as a single directional marker oriented to its heading**, so which way
  it is pointing is readable for every vehicle, parked or moving.
- **Status is carried by colour together with a fill treatment.** The two are redundant channels
  for the same fact, so status remains readable without relying on colour.
- No distinction anywhere on the map is conveyed by colour alone.
- Given a vehicle's state and energy are known, the operator can tell — from the map alone,
  without selecting it — what it is doing and whether it warrants attention.
- **Energy is encoded on the map only as flagged or not flagged.** The exact figure is not on the
  map, and energy is not shaded, ramped, or labelled per vehicle.
- **A vehicle warranting attention gains a halo and a badge identifying which condition applies**,
  added to its marker rather than replacing any part of it, so its status stays readable.
- **Attention-worthy vehicles are drawn above all others**, so a flagged vehicle is never
  obscured by a healthy one.
- Attention is never signalled by motion. Nothing blinks, pulses, or animates to draw the eye.
- **Given the operator hovers a vehicle, they see its label and, if flagged, the reason** —
  without having to select it. No vehicle labels are drawn on the map otherwise.
- A legend is present and accounts for every visual distinction the map makes. Anything the map
  encodes that the legend does not explain is a defect.
- Given ~100 vehicles are displayed, individual vehicles remain distinguishable and selectable,
  and the operator can tell dense areas from sparse ones.
- The service area boundary and its zones are visible, so the operator can see the geography they
  are responsible for.
- **On every load the operator sees the whole fleet:** no filter applied, nothing selected, the
  viewport framing the service area.
- Which map layers are shown is remembered between sessions; no filter ever is (F4).

### F2 — Route visibility

*Serves: N2, N6 · R3*

Every vehicle under remote control shows the path it is following. Selection controls *emphasis*,
not existence.

**Acceptance criteria**
- Given a vehicle is EN_ROUTE, its route is drawn on the map.
- Given no vehicle is selected, all EN_ROUTE routes are drawn de-emphasised — light enough that
  ~10 together do not compete with the vehicles, present enough to show where the fleet's active
  work is concentrated.
- Given a vehicle is selected and it is EN_ROUTE, its route is visually distinct from all others
  and can be traced end to end.
- A route's destination is identifiable, so which end of the path is the goal is unambiguous.
- The operator can judge how far through its journey a vehicle is from its position along the
  drawn route. No progress figure or arrival estimate is displayed.
- Given a route update arrives, the displayed path changes to match, whether or not it is selected.
- Given a vehicle stops being EN_ROUTE, its route stops being displayed.
- Given the operator selects a FREE or WITH_CUSTOMER vehicle, no route is displayed, and the
  reason is evident rather than looking like missing data.
- Given a filter hides a vehicle, its route is hidden with it. A route on the map always belongs
  to a vehicle the operator can see.

### F3 — Vehicle inspection

*Serves: N2, N4, N6 · R2, R5*

Detail arrives in a panel that slides in from the right when a vehicle is selected, displacing the
map rather than covering it. When nothing is selected the panel is absent and the map has the whole
window.

**Acceptance criteria**
- Given the operator selects a vehicle, they see its label, position, heading, state, energy
  level, how current that information is, its route if it has one, and any reason it warrants
  attention.
- **Given the panel opens, the selected vehicle remains visible on the map** and is not displaced
  beneath the panel or off the edge of the viewport by the map narrowing.
- Given the panel opens or closes, the map's scale and the operator's sense of place are
  preserved; the fleet does not appear to jump.
- How current the information is is shown for a selected vehicle at all times, whether good or bad
  (§6.2.7).
- Given a vehicle warrants attention for both reasons, both are stated here even though the map
  shows only the dominant one.
- Given a selected vehicle's state changes, the displayed detail changes with it, without the
  operator having to reselect or refresh.
- The operator can tell at all times which vehicle is selected, and can clear the selection.
- Given a vehicle is selected, the operator can obtain enough about it to act outside this
  system — to name it to someone else, or to say where to go.

### F4 — Narrowing the fleet

*Serves: N3, N4, N5 · R5*

**Acceptance criteria**
- The operator can restrict the map to vehicles in a given state.
- The operator can restrict the map to vehicles whose energy warrants attention.
- The operator can restrict the map to vehicles whose information has gone stale.
- Vehicles that do not match an active filter are hidden from the map, not de-emphasised.
- Given a filter is active, that fact is visible, the operator can tell how much of the fleet is
  hidden, and can clear it in one action. **This indicator is load-bearing, not decorative: it is
  what prevents a filtered map from being mistaken for the whole fleet.**
- **No filter survives a page load.** Every load begins with the whole fleet visible, so the
  operator can never inherit a hidden fleet from a previous session.
- Given a filter is active and a vehicle's state changes such that it now matches or stops
  matching, the map updates accordingly.

### F5 — Vehicles needing energy attention

*Serves: N3 · R2, R5*

**Acceptance criteria**
- The operator can identify, without inspecting vehicles one at a time, which vehicles are
  charging candidates.
- A vehicle qualifies below 20%. There is one band, not tiers. The threshold is stated in the UI,
  not implicit.
- The exact energy figure is available on inspection.
- Given a vehicle crosses the threshold in either direction, the view reflects it.
- The UI does not imply the system knows whether a vehicle can complete what it is doing, or where
  it could charge. It knows neither.

### F6 — Trustworthy freshness

*Serves: all needs; without it none of them can be relied on · R5*

**Acceptance criteria**
- Given a vehicle misses two consecutive expected reports, the operator is made aware of that
  vehicle specifically, and its other displayed values are visibly marked as untrustworthy.
- A stale vehicle remains on the map at its last known position, and how long it has been silent
  is shown.
- Given a vehicle that had gone stale is reported again, it visibly returns to normal.
- How current a vehicle's information is is surfaced only when it is a problem, except for the
  selected vehicle, where it is always shown.
- Given the connection between browser and backend is lost, the operator is told the *whole view*
  is stale, and is not left looking at a frozen map that appears live. This is distinguishable from
  a single vehicle going stale.
- Given the event source stops entirely, the fleet correctly becomes stale vehicle by vehicle, and
  this is distinguishable from a lost connection.
- Given the backend has not yet learned the whole fleet, the operator is told the view is still
  filling rather than shown a partial fleet as though it were complete.

### F7 — Reading coverage

*Serves: N5 · R5*

**Acceptance criteria**
- The operator can see, per zone, whether it is meeting its expectation, below it, or has nothing
  available at all — without counting map pins by eye.
- Each zone's expectation is discoverable, so the operator knows what line is being drawn on their
  behalf.
- Only available vehicles count toward a zone. Vehicles being driven do not.
- Given the distribution or availability of the fleet changes, the view reflects it — including
  when a vehicle changes status without moving.
- Vehicles outside the service area, or otherwise belonging to no zone, are still visible on the
  map and are not silently dropped from any total the operator can cross-check.

### F8 — Fleet situation summary

*Serves: N1, N2, N3, N5 · R5*

The summary sits in a collapsible overlay along the top of the map, expandable for detail and
compact by default so it costs as little of the map as possible.

**Acceptance criteria**
- The operator can see, at a glance, how the fleet divides across states, how many vehicles need
  energy attention, and how many have gone stale.
- The summary is legible without being expanded, and expanding it is not required to answer "how
  is the fleet doing".
- These figures describe the whole fleet and do not change when a filter is applied; the
  relationship between summary and filtered map is evident rather than confusing.
- The figures change as the fleet changes.
- The figures reconcile with the unfiltered map; a figure that disagrees with it is a defect.

### F9 — Event-driven ingest and live propagation

*Serves: R4, R6; enables every other feature*

Signals arrive as individual events; the backend composes them into something the frontend can
consume (§6.1.8).

**Acceptance criteria**
- Fleet state held by the backend is derived only from consumed events. There is no other writer
  of vehicle state.
- Telemetry signals and route assignment/update events are all handled, and the state the operator
  sees reflects them all.
- Given the same event is delivered more than once, the resulting state is identical to having
  received it once. Delivery is at-least-once.
- Given events arrive out of order, the operator is never shown an older observation replacing a
  newer one. Delivery is unordered.
- Given an event is consumed, the resulting change is visible to a connected operator within
  250 ms, without operator action.
- Given the stream delivers events for a vehicle the backend has not seen before, that vehicle
  appears.
- Given the backend restarts, it rebuilds state from the stream, and the operator is told the view
  is filling rather than shown a partial fleet as complete.
- Events that are discarded — as duplicates, as superseded, or as uninterpretable — are logged with
  the reason, so that correct handling is observable rather than merely asserted.
- The path from event to screen is documented well enough that a reader can trace one event end to
  end.

### F10 — Event source for local operation

*Serves: R6; makes R1, R3, R6 and the given conditions demonstrable*

Not visible to the operator and not controllable from the UI (§6.2.5) — from the client's point of
view this is a live fleet.

**Acceptance criteria**
- Running the system locally produces ~100 vehicles in Las Vegas with ~10 EN_ROUTE at any time,
  without manual setup beyond the documented run instructions.
- Movement, energy drain, state transitions and route assignments are all expressed as events, so
  that F9's "events are the only writer" criterion is genuinely exercised.
- Behaviour is plausible enough that the map reads as a fleet, not as noise: EN_ROUTE vehicles move
  along their stated path; parked vehicles do not teleport; WITH_CUSTOMER vehicles move without a
  planned path and may leave the service area.
- The source produces, unprompted, the situations the operator features exist to handle — at
  minimum a vehicle that goes stale and a vehicle whose energy becomes a problem — within a
  reasonable observation window, since the operator cannot trigger them.
- The source exercises at-least-once and unordered delivery, so F9's guarantees are tested by the
  running system rather than only asserted.
- The source is separable from the backend's ingest path, such that replacing it with a real broker
  does not require the ingest path to change.

---

## 4. Out of scope

### 4.1 Excluded by the brief

- Running an actual Kafka broker. A simulated event source is sufficient.
- Full implementation of the user story.
- Visual polish. Clarity and usability are what will be evaluated.
- Any implementation aimed at ~1000 vehicles.

### 4.2 Excluded by decision

- **Any outbound action, command, or dispatch.** No sending a field agent, contacting a customer,
  reassigning a vehicle, or annotating one. Observe only.
- **Field agents as entities** — not modelled, tracked, or displayed.
- **Trips as entities** — no customer, origin, destination, or expected completion.
- **Customer support requests** as an input, and as a kind of issue.
- **Any attention condition other than low energy and stale information**, including route
  deviation and being outside the service area. Both are visible on the map; neither is diagnosed.
- **Any judgement about whether a vehicle can finish its journey on its remaining energy**, and any
  notion of charging locations.
- **Energy tiers.** One threshold, not a low/critical hierarchy.
- **A fourth vehicle status**, and any sub-distinction within EN_ROUTE between travelling towards a
  customer and away from one.
- **Arrival estimates and numeric journey progress.**
- **Predictive or forecast coverage.** Coverage is an observation of current availability.
- **Operator control over the simulation** — no injecting faults, pausing, or replaying.
- **Persistence of fleet state.** Nothing survives a backend restart; the stream is the recovery
  mechanism.
- **History of any kind** — no position trails, no audit, no past state. Latest known only.
- **Authentication, authorisation, and operator identity.**
- **Shared state between operators** — no shared selection, filters, or handover. Multiple
  independent viewers are supported; coordination between them is not.
- **Cross-tab coordination.** A second tab is simply another viewer.
- **Responsive and mobile layouts.** Desktop only, at operator-desk width; degrade rather than
  break.
- **Any claimed accessibility conformance level.** The colour rule in F1 is a hard requirement;
  everything else is best effort.
- **Localisation.** One locale, English, metric units throughout.
- **Deployment, containerisation, and CI.** Runs locally only.
- **Production observability** — metrics, tracing, log shipping, alerting, and any
  operator-or-reviewer-facing counters surface. Logs are the whole of it.
- **Real road geometry and routing against a real road network.**
- **Rate limiting, backpressure onto the producer, and abuse protection.**
- **Continuous encoding of energy on the map** — no colour ramps, fill levels, or per-vehicle
  numeric labels. Flagged or not flagged.
- **Always-on vehicle labels.** Labels appear on hover and in the detail panel only.
- **Motion as a signalling channel.** Nothing blinks, pulses or animates to attract attention.
- **Persisting anything that could make the fleet look smaller than it is.** Filters and selection
  never survive a load; map layer visibility may.

---

## 5. Non-functional expectations

- **Freshness: 250 ms** from an event being ingested to the change being visible to a connected
  operator.
- **Staleness: two consecutive missed reports**, expressed relative to the expected reporting
  interval rather than as a wall-clock duration.
- **Legibility under load.** ~100 vehicles and ~10 routes on screen at once must remain readable
  and interactive. No distinction conveyed by colour alone.
- **Honesty about uncertainty.** The view must never present stale or partial state as current and
  complete. An operator who cannot trust the radar cannot use it.
- **Traceability.** A reader must be able to follow one event from source to pixel. Discarded
  events are logged with a reason.
- **Restraint.** The covering email calls out dead or inexplicable code and prefers clean code over
  extra features. Anything in the repo that does not serve a feature in §3 is a defect.

---

## 6. Resolved questions

### 6.1 Ambiguities in the brief

1. **What the three statuses mean.** EN_ROUTE means a remote driver is driving the car along a
   planned route, to a customer or away again; WITH_CUSTOMER means the customer is driving it
   themselves, which is why there is no route to display; FREE means parked and available. This
   resolves the apparent contradiction in R3 and explains why ~10 of ~100 are EN_ROUTE.
2. **Whether the operator acts, or only observes.** Observe only.
3. **What "may need charging" means.** A fixed threshold on the energy figure. Not a
   route-feasibility judgement.
4. **What "low coverage" is relative to.** A hard-coded expectation.
5. **What "an issue" is.** Exactly low battery or stale data, surfaced through indicators and
   filters.
6. **What a "customer trip" is.** The WITH_CUSTOMER state. Customer support requests are descoped.
7. **Where the fleet operates.** Las Vegas, a single service area.
8. **Whether telemetry is periodic or change-driven, full-state or delta.** Signals arrive as
   individual events; the backend packages them for frontend consumption.

### 6.2 Gaps opened by the first draft of this spec

1. **Route display policy.** All EN_ROUTE routes drawn, de-emphasised; the selected vehicle's
   highlighted.
2. **Filter semantics.** Non-matching vehicles are hidden.
3. **Freshness budget and stale threshold.** 250 ms freshness; stale threshold hard coded.
4. **Event delivery guarantees.** At-least-once, not ordered.
5. **Operator control of the simulation.** None — the simulation is hidden from the client.
6. **Summary scope.** Describes the whole fleet, not the filtered subset.
7. **Freshness display.** Surfaced only when bad, except for the selected vehicle.

---

## 7. Product decisions and their reasoning

The decisions themselves are already expressed in §2–§5. This section records *why*, what each
costs, and what would overturn it. Technical decisions are not here — they are in `docs/adr/`.

A single test recurs throughout, and is worth stating once: **a distinction earns its place only
if it changes what the operator does.** Because the system cannot act, an indicator that conveys
urgency the operator cannot respond to differently is decoration, and it costs legend space on a
map that must stay readable at a hundred markers. Alarm fatigue is the characteristic failure of
operator tooling.

### 7.1 Attention conditions and thresholds

**Energy threshold at 20%, one band.** 20% matches the consumer-EV mental model, so it needs no
explaining, and leaves genuine reserve for a teledriver to reach a charger under the vehicle's own
power. 15% would leave almost nothing flagged in a running system, making the feature look
untested; 30% would flag a third of the fleet and become wallpaper. One band rather than
low/critical tiers, because the operator's response to 18% and to 4% is identical — get it charged
or send someone — and with no charging locations modelled we cannot support a differentiated
response. *Cost:* no triage order among flagged vehicles. *Revisit if* operators ask which of
several low vehicles to take first — the answer is ranking, not a second colour.

**Staleness at two consecutive missed reports, defined relative to the reporting interval.** The
relative definition encodes the actual meaning and stays correct if the cadence changes; a
hardcoded number of seconds silently becomes wrong the moment the reporting rate moves, which is
the kind of latent bug that survives a long time because nothing fails loudly. One missed report
would strobe the fleet in and out of stale on ordinary jitter. *Cost:* two intervals tolerates only
one interval of jitter, which constrains decisions not yet made — the reporting cadence must be
regular, and if the event source simulates delayed delivery aggressively, false staleness is the
expected symptom. Recorded so it is diagnosed rather than rediscovered. *Also:* staleness is
derived from **absence**, making it the only operator-visible state not caused by an event. Every
other value is a pure function of consumed events; this one requires something to evaluate the
passage of time, and that asymmetry has to be built deliberately.

**Stale vehicles stay on the map, marked, with their silence duration.** The vehicle that has gone
silent is precisely the vehicle that may need a field agent — it is the *most* interesting vehicle
on the map, not the least. Removing it would hide N4's central case, and a vehicle that vanishes
reads as a bug rather than as information. Silence duration is what separates a brief dropout from
something being wrong.

**The two conditions are distinguishable; staleness subsumes low energy.** They imply different
work, so collapsing them into one badge forces a click on exactly the vehicles where the operator
is most pressed. Where both apply, staleness wins — not as a priority call but a logical one: a
stale energy reading is not a fact. The vehicle may be at 0%, or plugged in and charging. Showing
"low battery" for a vehicle we have not heard from asserts something we do not know. Both reasons
appear on inspection. *Revisit if* a third condition arrives, at which point "distinguish them all
visually" stops scaling and a combined indicator with a reason list becomes correct.

**At ~1000 vehicles:** the 20% line remains the right definition but stops working as a workflow —
a tenfold fleet flags 100–200 vehicles, and the assumption that a flagged set can be eyeballed is
what fails. Per-vehicle marks must become aggregation, which §7.2's zones already supply. Stale
vehicles become more valuable and harder to see, so attention-worthy vehicles should be exempt from
whatever clustering that introduces — they are the reason the operator is looking. The relative
staleness definition scales unchanged and needs no clock coordination.

### 7.2 The service area and coverage

N5 is the vaguest need in the brief and the easiest to fake. A density heatmap looks like an answer
and is not one: it shows where vehicles *are*, leaving the operator to work out where they are
*missing*, which is the entire task N5 delegates to us.

**A hand-authored polygon, not a bounding box or an administrative boundary.** A rectangle over Las
Vegas contains desert, mountains and the airport — regions that are *correctly* empty, so they
would be flagged as low coverage permanently. That does not weaken the feature, it inverts it. Real
city limits are not a service area either; no operator runs a whole municipality. A hand-authored
polygon reads as Las Vegas, needs no runtime fetch, works offline, and is reviewable in a diff.

**Named zones with per-zone minimums, not a uniform grid.** Operators reason in places that have
names, and because the system cannot act, its output is always something said aloud. Per-zone
minimums also capture the operational truth that a central zone needs more vehicles than a
residential edge, which a single global threshold denies. *Cost:* zone boundaries are invented, and
edge effects are real and visible — a vehicle metres outside a zone contributes nothing to it.
⚠️ This is the one place we added a concept the brief does not mention: the brief says "areas", and
a uniform grid is the more literal reading. It should be defended as a choice, not presented as
given.

**Three coverage states, including zero as distinct.** This appears to contradict the single energy
band, and applies the same test to reach a different answer: a zone below target serves customers
with degraded response, while a zone at zero cannot serve them at all. Different escalation, so the
distinction passes the test that 4%-versus-18% battery failed.

**Only available vehicles count; low-energy ones still do.** Coverage means "could serve a customer
now", and a vehicle being driven is committed. Excluding low-energy vehicles would assert knowledge
we lack — we have no model of trip length or energy requirements — and would double-count one
problem across two features while making coverage flicker as batteries drift across 20%.

**No fourth status, and vehicles may leave the service area.** R2 names exactly three statuses;
extending them diverges from a stated requirement, and the only payoff would be the predictive
coverage we declined. Vehicles leaving the area follows directly from the confirmed status model —
a customer driving may go anywhere — so the map must not lie about it. It is not made an attention
condition: reopening a closed list for a case whose behaviour we have not yet observed is how a
closed list becomes a dumping ground.

**Two consequences worth flagging as implementation traps.** Vehicles can belong to *no* zone —
outside the area entirely, or between zones depending on how they tile — and any code assuming
every vehicle has a zone will silently drop vehicles from counts. F8 treats a figure that disagrees
with the map as a defect, so this needs handling explicitly. And because only available vehicles
count, **coverage changes on status transitions, not only on movement**: a vehicle being assigned
to a job changes its zone's coverage without moving a metre, so any optimisation keyed on position
changes will be wrong.

**At ~1000 vehicles:** this is the feature that scales best, structurally — coverage output is
proportional to zone count, which is a property of the geography and does not grow with the fleet.
So payload and cognitive load stay constant while the fleet grows tenfold. That also makes zones
the aggregation primitive §7.1 needs: "four low-battery vehicles in Downtown" is the same shape of
answer as "Downtown is two cars short", requiring no new concept. Per-zone minimums are absolute
counts and must be revised as the fleet grows — expressing them as a share of fleet size would
scale automatically but would stop meaning "enough cars to serve this area". Zone assignment is a
point-in-polygon test per vehicle, cheap at either scale and further reducible by recomputing only
on boundary crossings.

### 7.3 Vehicle identity and journey legibility

**Two identifiers: a machine identity and a human label.** Separating them lets each be right on
its own terms, and keeps the wire contract from depending on a display string — so a label can be
corrected or re-badged without touching identity or invalidating anything already delivered. The
machine identity is also the natural basis for duplicate detection and for partitioning if the
stream ever really is Kafka. *Cost:* two identifiers is a standing discipline, and getting it
backwards produces bugs that only appear when a label changes — which in a demo is never, so
running the system will not exercise it. Logs need the label to be useful to a human and the
identity to be useful to a developer.

**A destination marker and position along the route, not a percentage or an ETA.** The operator's
questions are spatial, and a marker on a line answers them; the destination marker supplies the one
thing a bare polyline cannot, which is which end is the goal. "60% complete" is operationally
meaningless — 60% of what distance, through what traffic — and asserts precision we lack. An ETA is
what operators genuinely want and needs speed and traffic modelling we do not have; a confidently
wrong ETA is worse than none, because the operator will plan against it. *Cost:* progress is read
off the map, so it cannot be filtered or sorted on. *Revisit if* the map reads ambiguously about
direction of travel, at which point fading the travelled portion of the path is the refinement.

**No route-deviation detection.** Both the route and the vehicle are drawn, so a vehicle off its
line is visible without the system asserting anything — consistent with letting vehicles leave the
service area unremarked. Detecting deviation means choosing a distance tolerance and a
position-noise model, neither justifiable over simulated positions where being off-route is an
artefact rather than a signal. *Cost:* this is a real gap, not a costless one. A vehicle stopped
off-route is exactly what N4 exists for, and the operator will only catch it by looking. *Revisit
if* real telemetry replaces simulated positions, which turns the artefact into a signal and arrives
with a defensible noise model.

**At ~1000 vehicles:** the label scheme must be sized for the fleet from the start — a three-digit
label runs out at exactly 1000, the number the brief asks about, and would need re-badging at the
worst moment. The machine identity is unaffected, which is part of why separating them is right.
Destination markers become clutter at ~100 concurrent journeys and should follow the same
faint-versus-emphasised structure the routes already use. Deviation detection gets *harder to
decline*: spotting an off-route vehicle by eye is plausible at ten journeys and impossible at a
hundred, so scale is an independent trigger for revisiting it.

### 7.4 Scope boundaries

**No persistence.** In an event-sourced design the stream *is* the recovery mechanism, so
persistence would be a second answer to a question already answered, and it would compromise
"events are the only writer". The genuinely correct event-sourced recovery is replay from a retained
log — which is what real Kafka provides for free, and building it here would turn our simulated
source into a broker. ⚠️ **The real obligation this creates is not storage but honesty during
warm-up:** a freshly restarted backend holds a partial fleet, and F6 and F9 forbid presenting that
as complete. The warm-up state is a requirement, not a nicety.

**No history.** A bounded position trail is the tempting version, but heading already tells the
operator which way a vehicle was going, so the trail's main use is already served. Where history
genuinely earns its keep is post-incident review, which is out of scope.

**No authentication.** It adds no engineering interest here and would draw attention from what is
being evaluated. Worth being able to say where it would go — a gateway in front of the read path,
with event ingest staying internal.

**Many independent viewers, no shared state.** Nearly free if fleet state is server-owned, and it
forces a split that pays off: derived fleet state computed once for everyone, per-viewer selection
and filters kept client-side. Restricting to a single operator would tempt us into computing
filtered views server-side per connection, which is the worse design. Two browser windows side by
side is also the simplest way to demonstrate that updates are genuinely pushed rather than polled.
A second tab is just another viewer — no cross-tab coordination.

**Desktop only; no colour-only encoding; one locale, metric.** The colour rule is not compliance
theatre: colour-only encoding fails an operator on a bad monitor, under glare, or at 2am as surely
as it fails a colourblind one, so it is a legibility requirement that happens to also be an
accessibility one. Metric throughout sidesteps any assumption about where the operator physically
sits.

**Logs, and nothing more, for observability.** Discarded events are logged with a reason —
duplicate, superseded, uninterpretable — because without that, at-least-once and unordered handling
works silently or fails silently and nothing distinguishes the two. A counters surface was
considered and declined; reading logs is sufficient for a reviewer who runs the system.

**The event source stopping needs no special handling.** Every vehicle exceeds the staleness
threshold and the fleet correctly marks itself stale, which falls out of §7.1 for free. What matters
is that it stays distinguishable from a lost connection — source stopped means a healthy connection
and a hundred stale vehicles; connection lost means nothing can be trusted at all. That is a test
case, not a feature.

### 7.5 What the map encodes, and what surrounds it

**One directional marker per vehicle, carrying heading by orientation and status by colour plus
fill.** Heading belongs on the base marker rather than on a separate tick, because the spec treats a
parked vehicle's heading as meaningful — which way it will leave — so every vehicle needs it, and one
object per vehicle is cheaper than two. Status needs a second, non-colour channel or the hard colour
rule is broken; fill treatment supplies it and stays legible at small size where semantic glyphs — a
person, a steering wheel — would not. *Revisit if* fill states prove too subtle in practice, in which
case distinct outline shapes per status are the fallback, at the cost of orientation being harder to
read.

**Energy on the map is binary.** Encoding it continuously would fight the status colour, reduce a
hundred markers to shaded noise, and imply exactly the urgency gradient that choosing a single band
rejected. *Cost:* relative energy across the fleet is not visible at a glance, which is consistent
with having decided relative energy does not change the operator's action.

**Attention is additive and raised, never substitutive.** Recolouring a flagged marker would be the
loudest signal and would destroy status on precisely the vehicles where status matters most — a stale
FREE vehicle and a stale WITH_CUSTOMER vehicle are very different situations. Raising flagged
vehicles in z-order matters more than it appears: without it, the one vehicle the operator needs to
see can sit underneath a healthy one in a dense area, which would make the feature useless exactly
where it is needed. Motion is excluded because someone watches this for a whole shift.

**Hover for labels, not always-on labels.** A hundred overlapping labels is unreadable, but naming a
vehicle has to be cheap because handoff needs the label and should not require committing to a
selection. The considered alternative — labels only on flagged vehicles — targets the need precisely
and was declined because labels then shift as vehicles move and multiply when the flagged count
spikes.

**A full-window map, with a top summary overlay and a right detail panel that displaces rather than
covers.** A fixed panel costs known screen space; a floating overlay costs unknown information,
because which vehicles it hides changes as the fleet moves. The top summary is the one accepted
overlay, kept compact by default, on the grounds that it obscures a predictable strip rather than an
arbitrary set of vehicles. ⚠️ The detail panel displacing the map means the map resizes on selection,
which can carry the just-selected vehicle under the panel edge or off-screen — so keeping it in view
is an acceptance criterion (F3), not an implementation detail.

**Filters reset on load; layer visibility persists.** The line is that **anything capable of making
the fleet look smaller than it is must not survive a reload**, while display preferences may. This
matters because an operator who inherits a filter from yesterday and sees four vehicles may
reasonably conclude the fleet is four vehicles. Resetting filters is the primary defence; F4's
active-filter indicator is the secondary one, which is why it is specified as load-bearing.

**At ~1000 vehicles:** hover-for-label degrades, because at that density the pointer resolves to an
ambiguous cluster rather than to a vehicle. Binary energy encoding and additive attention both hold,
but z-ordering flagged vehicles above healthy ones stops being sufficient when a hundred flagged
vehicles overlap each other — which is the same pressure toward aggregation §7.1 and §7.2 already
identified, arriving through a third route. The full-window map and the panel structure are
unaffected.

---

## 8. Decisions taken without being asked

Recorded per the working ground rules.

1. **Separating the brief's operator needs from its functional requirements**, and treating the
   needs as things the product must make answerable rather than as features to build.
2. **Splitting out-of-scope into "excluded by the brief" and "excluded by decision"**, so the
   brief's permissions were not laundered into settled scope.
3. **Writing acceptance criteria that named their missing threshold** rather than inventing one, in
   the first draft — leaving six features honestly incomplete until §6 was answered.
4. **Dropping the operator-response feature** rather than keeping it as an empty section once
   observe-only was confirmed.
5. **Promoting "warrants attention" to its own part of the domain model** (§2.4) rather than
   leaving it implicit across the energy and freshness features. It is derived, not reported, and
   naming it as a closed list is what makes it extensible without becoming a dumping ground.
6. **Making route visibility and route emphasis separate concerns** in F2, which is what keeps R3
   literally satisfied rather than narrowed.
7. **Treating vehicles belonging to no zone, and coverage changing on status transitions, as
   spec-level acceptance criteria** (F7) rather than leaving them as implementation notes. Both are
   silent-wrong-answer failures rather than visible breakages, so they needed to be stated where
   they can be tested against.
8. **Keeping a fill treatment alongside colour for status** (F1, §7.5). "Arrow and colour" was
   approved, but arrow carries heading, which would leave status encoded by colour alone and break
   the hard rule agreed for it. The redundant channel is retained on that basis rather than as an
   addition on top of what was asked for.
9. **Reading "reset filters on reload" as scoped to filters**, leaving map layer visibility
   persisted, on the principle stated in §7.5.
10. **Requiring the selected vehicle to stay in view when the detail panel opens** (F3). This
    follows from the panel displacing the map rather than covering it, and it is the kind of thing
    that is obvious once broken and easy to omit until then.

---

## 9. Traceability

| Need / requirement | Features |
|---|---|
| N1 Where cars are | F1, F8 |
| N2 What each car is doing | F1, F2, F3, F8 |
| N3 May need charging | F1, F4, F5, F8 |
| N4 Send a field agent (spot and hand off) | F1, F3, F4, F6 |
| N5 Low coverage areas | F1, F4, F7, F8 |
| N6 Support a customer trip (spot and hand off) | F2, F3, F6 |
| R1 ~100 vehicles on a map | F1 |
| R2 Live vehicle state | F1, F3, F9 |
| R3 EN_ROUTE routes displayed | F2 |
| R4 UI updates as state changes | F9 |
| R5 Operator UX decisions | F1, F3, F4, F5, F6, F7, F8 |
| R6 Event-driven backend | F9, F10 |
