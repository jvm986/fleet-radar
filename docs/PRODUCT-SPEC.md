# Fleet Radar — Product Spec

Status: **signed off.** The ambiguities in the brief and the gaps this document opened were
resolved on 2026-09-07; §6 records the answers for traceability. Everything still undecided is
a design decision, not a requirement, and lives in `DECISIONS-TO-MAKE.md`.

Source material: `docs/Full-Stack_Engineer_at_Vay_-_Take-Home_Coding.pdf` (the brief) and
`docs/email.txt` (the covering email).

---

## 1. What this is

A web-based **Fleet Radar**: a single operational view that lets one fleet operator answer
questions about a fleet of remotely-operated EVs in real time, the way a flight radar lets a
controller answer questions about aircraft.

The brief states the goal is to demonstrate full-stack capability, event-driven thinking,
real-time state handling, pragmatic architecture, and operator-focused UX thinking — with
clarity over polish. It also says the user story is there to *inspire design thinking* and
"does not need to be fully implemented", but that the solution should reflect
operator-oriented decision making. This spec therefore treats the operator needs as the
things the product must make **answerable**, and separates them from the functional
requirements the product must **literally do**.

**Fleet Radar observes; it does not act.** The operator uses it to reach a judgement and then
acts elsewhere — by radio, phone, or a dispatch tool that is not this one. The system issues
no commands and emits no events outward. This is a decided constraint (§6.1.2) and it is what
makes N4 and N6 a question of information quality rather than of workflow.

### 1.1 The operator's needs (verbatim from the brief)

| ID | Need |
|----|------|
| **N1** | Understand where cars are located |
| **N2** | See what each car is currently doing |
| **N3** | Identify vehicles that may need charging |
| **N4** | Send a field agent in case of an issue |
| **N5** | Spot areas with low vehicle coverage |
| **N6** | Support a customer trip if something goes wrong |

Given the observe-only constraint, N4 and N6 are served by the operator being able to *spot*
the vehicle that needs a field agent or the trip that has gone wrong, and to get enough about
it to hand off. Neither dispatch nor customer contact is part of this system.

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

- Fleet size ~100 vehicles, operating in **Las Vegas**, within a single realistic service area.
- ~10 vehicles simultaneously EN_ROUTE at any given time.
- Must run locally.
- Design should be discussable at ~1000 vehicles; no implementation of that required.

### 1.4 Stated permissions (not instructions)

The brief says in-memory storage is *acceptable*, a simulated event source is *sufficient*,
no authentication is *required*, and technology choices are *fully open*. These are removals
of constraint, not directives. Each remains an open question in `DECISIONS-TO-MAKE.md`.

---

## 2. Domain model

Described as **the information the operator needs in order to act** — deliberately not as a
schema. Field names, how information is grouped into messages or records, and the granularity
of any timestamp are design decisions and are out of scope for this document.

### 2.1 The vehicle

For any single vehicle, the operator needs to know:

- **Which vehicle this is** — a handle stable and human-speakable enough to use on a radio,
  in a ticket, or when telling a colleague "go look at that one".
- **Where it is** — precisely enough to place it on a map, and to tell whether it is on a
  road, in a depot, or somewhere it should not be.
- **Which way it is pointing** — a stationary vehicle's heading tells the operator which way
  it will leave; a moving vehicle's heading tells them whether it is going where they expect.
- **What it is doing right now** — one of the three states in R2, which mean:
  - **FREE** — parked and available. Nobody is driving it.
  - **EN_ROUTE** — a remote driver is driving it along a planned route, either towards a
    customer or away again once the customer is done.
  - **WITH_CUSTOMER** — the customer is driving it themselves. This is why there is no route
    to display: the vehicle's path is the customer's choice, not a plan the system holds.
- **How much energy it has left** — as a proportion of capacity, because that is what R2
  specifies.
- **How current this knowledge is** — every other item above is a *last known* value, not a
  fact. The operator must be able to tell the difference between "this vehicle is parked" and
  "this vehicle stopped telling us anything a while ago". Without this the other information
  is untrustworthy, which undermines every need. (R5 hints at the same thing with "stale
  indicator".)
- **Whether it currently warrants attention**, and why — see §2.4.

The status definitions above are the confirmed reading of the brief (§6.1.1). They matter
beyond naming: because a WITH_CUSTOMER vehicle is being driven by a customer, its next
position is genuinely unpredictable, and no planned path for it exists to be shown or
reasoned about.

### 2.2 The journey a vehicle is on

Only a vehicle under remote control (EN_ROUTE) has a journey the system knows about. For such
a vehicle the operator needs to know:

- **Where it is trying to get to.**
- **The path it intends to take** — so it can be drawn on the map (R3), and so the operator
  can see whether the vehicle is actually on it.
- **How far through the journey it is** — the difference between "just set off" and "nearly
  there" changes what the operator does about a problem.

A journey may be revised while in progress; the operator needs to see the current intent, not
the original one.

### 2.3 The fleet as a whole

The operator does not only reason vehicle-by-vehicle. They need:

- **The shape of the fleet right now** — how much of it is busy, how much is available, how
  much warrants attention, how much has gone quiet. This is a property of the whole fleet and
  is not affected by what the operator has chosen to look at (§6.2.6).
- **How the fleet is distributed across the geography** they are responsible for, measured
  against a fixed expectation of where vehicles ought to be available — which is the
  substance of N5.

### 2.4 A vehicle that warrants attention

The brief's needs reference an "issue" (N4) but R2's state model contains no such concept. A
vehicle warrants the operator's attention when **either** of exactly two things is true
(§6.1.5):

- **Its energy is low** — below a fixed threshold.
- **Its information has gone stale** — it has stopped reporting for longer than a fixed
  duration, so nothing else the operator can see about it can be trusted.

These are the whole of "an issue" in this system. Both are derived from state the system
already holds; neither is reported to us as a problem. Adding a third condition later is a
change to a defined list rather than the introduction of a new concept, which is the point of
defining it this way.

### 2.5 Concepts referenced by the needs but absent from vehicle state

R2 defines the complete live state of a vehicle and contains no representation of the
following, yet N3, N4, N5 and N6 each depend on one. Recorded with how each was resolved,
because the resolution is the reason several features are as thin as they are.

- **A charging opportunity.** N3 asks which vehicles "may need charging". There is no charging
  status and no notion of where a vehicle could charge. Resolved: "may need charging" is a
  judgement about the energy figure against a fixed threshold (§6.1.3). It is not a claim about
  whether the vehicle can finish what it is doing, and the UI must not imply otherwise.
- **An issue.** Resolved: exactly low energy or stale information (§2.4), surfaced through
  indicators and filters, with no prompting and no workflow (§6.1.5).
- **A field agent.** N4's response is to send one. Resolved: out of scope — observe only
  (§6.1.2). The system's job ends at making the vehicle findable and describable.
- **A customer trip.** N6 is about supporting one. Resolved: a trip *is* the WITH_CUSTOMER
  state (§6.1.6). There is no customer, origin, destination, or expected end. A customer
  requesting support is a plausible future event source and is explicitly descoped.
- **A service area and expected coverage.** N5 asks where coverage is *low*, which is only
  meaningful against an expectation. Resolved: a single realistic Las Vegas service area, hard
  coded (§6.1.4, §6.1.7). What the expectation consists of is an open decision.

---

## 3. Features

Each feature lists the needs and requirements it serves. Acceptance criteria are observable,
and avoid specifying mechanisms that remain open decisions.

### F1 — Live fleet map

*Serves: N1, N2, N3 · R1, R2, R5*

The whole fleet on one map of its Las Vegas service area, with each vehicle's current
situation readable without interaction.

**Acceptance criteria**
- Given the fleet is being reported, the operator sees every vehicle in it positioned on a map
  of the service area it operates in.
- Given a vehicle's reported position changes, its position on the map changes to match.
- Given a vehicle's heading is known, the map conveys which way that vehicle is pointing.
- Given a vehicle's state and energy level are known, the operator can tell — from the map
  alone, without selecting the vehicle — what it is doing and whether it warrants attention.
- A legend is present and accounts for every visual distinction the map makes. Anything the
  map encodes that the legend does not explain is a defect.
- Given ~100 vehicles are displayed, individual vehicles remain distinguishable and
  selectable, and the operator can tell dense areas from sparse ones.
- The service area is visible on the map, so the operator can see the boundary they are
  responsible for.

### F2 — Route visibility

*Serves: N2, N6 · R3*

Every vehicle under remote control shows the path it is following. Selection controls
*emphasis*, not existence: unselected routes are drawn faintly so the fleet's intent is
legible as a whole without the map becoming a bowl of spaghetti, and the selected vehicle's
route is drawn prominently so it can actually be followed.

**Acceptance criteria**
- Given a vehicle is EN_ROUTE, its route is drawn on the map.
- Given no vehicle is selected, all EN_ROUTE routes are drawn de-emphasised — light enough
  that ~10 of them together do not compete with the vehicles themselves, but present enough
  that the operator can see where the fleet's active work is concentrated.
- Given a vehicle is selected and it is EN_ROUTE, its route is visually distinct from all
  other routes on the map and can be traced end to end.
- Given a route update arrives for a vehicle, its displayed path changes to match, whether or
  not it is selected.
- Given a vehicle stops being EN_ROUTE, its route stops being displayed.
- Given the operator selects a FREE or WITH_CUSTOMER vehicle, no route is displayed for it,
  and the reason is evident rather than looking like missing data — a WITH_CUSTOMER vehicle
  has no planned path because the customer is driving.
- Given a filter hides a vehicle, its route is hidden with it. A route on the map always
  belongs to a vehicle the operator can see.

### F3 — Vehicle inspection

*Serves: N2, N4, N6 · R2, R5*

The operator can single out one vehicle and see everything known about it.

**Acceptance criteria**
- Given the operator selects a vehicle, they see its identity, position, heading, state, energy
  level, how current that information is, its route if it has one, and any reason it warrants
  attention.
- How current the information is is shown for a selected vehicle at all times, whether good or
  bad (§6.2.7).
- Given a selected vehicle's state changes, the displayed detail changes with it, without the
  operator having to reselect or refresh.
- The operator can tell at all times which vehicle is selected, and can clear the selection.
- Given a vehicle is selected, the operator can obtain enough about it to act outside this
  system — to name it to someone else, or to say where to go.

### F4 — Narrowing the fleet

*Serves: N3, N4, N5 · R5*

100 vehicles is more than an operator can hold at once. They must be able to reduce the map to
the vehicles that currently matter.

**Acceptance criteria**
- The operator can restrict the map to vehicles in a given state.
- The operator can restrict the map to vehicles whose energy level warrants attention.
- The operator can restrict the map to vehicles whose information has gone stale.
- Vehicles that do not match an active filter are hidden from the map, not merely
  de-emphasised (§6.2.2).
- Given a filter is active, that fact is visible, the operator can tell how much of the fleet
  is hidden by it, and can clear it in one action.
- Given a filter is active and a vehicle's state changes such that it now matches or stops
  matching, the map updates accordingly.

### F5 — Vehicles needing energy attention

*Serves: N3 · R2, R5*

**Acceptance criteria**
- The operator can identify, without inspecting vehicles one at a time, which vehicles are
  candidates for charging.
- A vehicle qualifies by its energy level being below a fixed threshold. The threshold is
  stated in the UI, not implicit, so the operator knows what they are looking at.
- Given a vehicle crosses that threshold in either direction, the view reflects it.
- The UI does not imply the system knows whether a vehicle can complete what it is doing, or
  where it could charge. It knows neither.

### F6 — Trustworthy freshness

*Serves: all needs; without it none of them can be relied on · R5*

**Acceptance criteria**
- Given a vehicle stops being reported for longer than a fixed threshold, the operator is made
  aware of that vehicle specifically, and its other displayed values are visibly marked as no
  longer trustworthy.
- Given a vehicle that had gone stale is reported again, it visibly returns to normal.
- How current a vehicle's information is is surfaced only when it is a problem, except for the
  selected vehicle, where it is always shown (§6.2.7).
- Given the connection between the operator's browser and the backend is lost, the operator is
  told that the *whole view* is stale, and is not left looking at a frozen map that appears
  live. This is distinct from a single vehicle going stale and must be distinguishable from it.
- The operator is never shown a partial fleet presented as a complete one.

### F7 — Reading coverage

*Serves: N5 · R5*

**Acceptance criteria**
- The operator can identify parts of the service area with few or no available vehicles,
  without counting map pins by eye.
- The view distinguishes *available* vehicles from vehicles that are present but not available,
  because a vehicle being driven does not constitute coverage.
- Given the distribution of the fleet changes, the view reflects it.
- Coverage is assessed against a fixed, hard-coded expectation of the service area, and what
  that expectation is is discoverable by the operator rather than a mystery.

### F8 — Fleet situation summary

*Serves: N1, N2, N3, N5 · R5*

An always-visible answer to "how is the fleet doing right now", so the operator can orient
without interrogating the map.

**Acceptance criteria**
- The operator can see, at a glance, how the fleet divides across states, how many vehicles
  need energy attention, and how many have gone stale.
- These figures describe the whole fleet and do not change when a filter is applied (§6.2.6);
  the relationship between the summary and the filtered map is evident rather than confusing.
- The figures change as the fleet changes.
- The figures reconcile with the unfiltered map; a figure that disagrees with it is a defect.

### F9 — Event-driven ingest and live propagation

*Serves: R4, R6; enables every other feature*

The backend treats an inbound event stream as the source of truth for fleet state, and the
operator's view follows that state as it changes. Signals arrive as individual events; the
backend composes them into something the frontend can consume (§6.1.8).

**Acceptance criteria**
- Fleet state held by the backend is derived only from consumed events. There is no other
  writer of vehicle state.
- Telemetry signals and route assignment/update events are all handled, and the state the
  operator sees reflects them all.
- Given the same event is delivered more than once, the state the operator sees is identical to
  having received it once. Delivery is at-least-once (§6.2.4).
- Given events arrive out of order, the operator is never shown an older observation replacing
  a newer one. Delivery is unordered (§6.2.4).
- Given an event is consumed, the resulting change is visible to a connected operator within
  250 ms, without operator action.
- Given the event stream delivers events for a vehicle the backend has not seen before, that
  vehicle appears.
- Given the backend restarts, the operator's view reflects reality rather than silently showing
  a partial fleet as complete.
- The path from event to screen is documented well enough that a reader can trace one event
  end to end.

### F10 — Event source for local operation

*Serves: R6; makes R1, R3, R6 and the given conditions demonstrable*

Something must produce the event stream for the system to run locally. It is not visible to the
operator and it is not controllable from the UI (§6.2.5) — from the client's point of view this
is a live fleet.

**Acceptance criteria**
- Running the system locally produces a fleet of approximately 100 vehicles in Las Vegas, with
  approximately 10 EN_ROUTE at any time, without manual setup beyond the documented run
  instructions.
- Vehicle movement, energy drain, state transitions and route assignments are all expressed as
  events, so that F9's "events are the only writer" criterion is genuinely exercised.
- Vehicle behaviour is plausible enough that the map reads as a fleet and not as noise:
  EN_ROUTE vehicles move along their stated path; parked vehicles do not teleport;
  WITH_CUSTOMER vehicles move without a planned path.
- The source produces, unprompted, the situations the operator features exist to handle — at
  minimum a vehicle that goes stale and a vehicle whose energy becomes a problem — so those
  features can be demonstrated rather than described. Since the operator cannot trigger these,
  they must occur on their own within a reasonable observation window.
- The source exercises at-least-once and unordered delivery, so F9's guarantees are tested by
  the running system rather than only asserted.
- The source is separable from the backend's ingest path, such that replacing it with a real
  broker does not require the ingest path to change.

---

## 4. Out of scope

### 4.1 Excluded by the brief

- Running an actual Kafka broker. The brief states a simulated event source is sufficient.
- Full implementation of the user story. The brief states it need not be fully implemented.
- Visual polish. The brief states it is not important and that clarity and usability are what
  will be evaluated.
- Any implementation aimed at ~1000 vehicles. The brief asks only that the design be
  discussable at that scale.

### 4.2 Excluded by decision

Signed off in §6. These are the ones where the brief left room and we chose to close it.

- **Any outbound action, command, or dispatch.** No sending a field agent, no contacting a
  customer, no reassigning a vehicle, no annotating one. Observe only (§6.1.2).
- **Field agents as entities** — not modelled, not tracked, not displayed (§6.1.2).
- **Trips as entities** — no customer, origin, destination, or expected completion. A trip is
  the WITH_CUSTOMER state and nothing more (§6.1.6).
- **Customer support requests** as an input to the system, and as a kind of issue (§6.1.6).
- **Any issue condition other than low energy and stale information** (§2.4, §6.1.5).
- **Any judgement about whether a vehicle can finish its journey on its remaining energy**, and
  any notion of charging locations (§6.1.3).
- **Operator control over the simulation** — no injecting faults, pausing, or replaying from the
  UI (§6.2.5).

### 4.3 Proposed exclusions — still need your sign-off

Open questions in `DECISIONS-TO-MAKE.md`, listed here so the shape of the intended scope is
visible in one place. Kept separate from §4.1 so the brief's permissions are not laundered into
settled scope.

- Authentication, authorisation, and any notion of operator identity.
- Durable storage, and any history or audit of past fleet state.
- Multiple concurrent operators, shared selection, or handover between operators.
- Real road geometry and routing against a real road network.
- Deployment, containerisation, CI, and anything that is not "runs locally per the README".
- Mobile and small-screen layouts; accessibility beyond not actively breaking it.
- Internationalisation, and any locale handling beyond a single one.
- Rate limiting, backpressure onto the producer, and abuse protection.
- Production observability: metrics, tracing, structured log shipping, alerting.

---

## 5. Non-functional expectations

- **Freshness: 250 ms** from an event being ingested to the change being visible to a connected
  operator (§6.2.3).
- **Stale-vehicle threshold:** a fixed, hard-coded duration of silence after which a vehicle's
  information is treated as no longer trustworthy (§6.2.3). The value is an open decision.
- **Legibility under load.** ~100 vehicles and ~10 routes on screen at once must remain readable
  and interactive.
- **Honesty about uncertainty.** The view must never present stale or partial state as current
  and complete. This is a hard expectation, not a nice-to-have: an operator who cannot trust the
  radar cannot use it, and F6 exists because of it.
- **Traceability.** A reader must be able to follow one event from source to pixel. The brief's
  deliverables ask for exactly this, and the covering email says the reasoning will be examined
  independently of any AI use.
- **Restraint.** The covering email calls out dead or inexplicable code, and prefers clean code
  over extra features. Anything in the repo that does not serve a feature in §3 is a defect.

---

## 6. Resolved questions

Recorded because the reasoning matters as much as the answer, and because the brief's silences
are where most of the design risk sat.

### 6.1 Ambiguities in the brief

1. **What the three statuses mean.** *Confirmed:* EN_ROUTE means a remote driver is driving the
   car along a planned route, to a customer or away again; WITH_CUSTOMER means the customer is
   driving it themselves, which is precisely why there is no route to display; FREE means parked
   and available. This resolves the apparent contradiction in R3 and explains why ~10 of ~100 are
   EN_ROUTE.
2. **Whether the operator acts, or only observes.** *Observe only.* N4 and N6 are served by
   information quality; no outbound commands exist.
3. **What "may need charging" means.** A fixed, arbitrary threshold on the energy figure. Not a
   route-feasibility judgement.
4. **What "low coverage" is relative to.** A hard-coded expectation.
5. **What "an issue" is.** Exactly low battery or stale data. Surfaced through indicators and
   filters — no prompting, no workflow.
6. **What a "customer trip" is.** A trip is the WITH_CUSTOMER state. A customer requesting
   support would be one kind of issue, and is descoped.
7. **Where the fleet operates.** Las Vegas, a single realistic service area.
8. **Whether telemetry is periodic or change-driven, full-state or delta.** Signals arrive as
   individual events; the backend packages them for frontend consumption.

### 6.2 Gaps opened by the first draft of this spec

1. **Route display policy.** All EN_ROUTE routes are drawn, de-emphasised; the selected
   vehicle's route is highlighted.
2. **Filter semantics.** Non-matching vehicles are hidden.
3. **Freshness budget and stale threshold.** 250 ms freshness; stale threshold hard coded.
4. **Event delivery guarantees.** At-least-once, not ordered.
5. **Operator control of the simulation.** None — the simulation is hidden from the client.
6. **Summary scope.** The summary describes the whole fleet, not the filtered subset.
7. **Freshness display.** Surfaced only when bad, except for the selected vehicle, where it is
   always shown.

### 6.3 Answers that opened further decisions

Not resolved here, and deliberately not given a quiet reading. Carried into
`DECISIONS-TO-MAKE.md`.

- §6.1.4 establishes that the coverage expectation is hard coded, but not **what it is an
  expectation of**, nor what makes coverage "low".
- §6.1.3 establishes an arbitrary energy threshold; **the value, and whether there is more than
  one band**, are undecided.
- §6.2.3 establishes a hard-coded stale threshold; **the value** is undecided.
- §2.4 defines two issue conditions; **whether they are surfaced as one combined indicator or as
  two distinct ones** is an information-design decision and is undecided.
- §6.1.8 establishes that the backend packages signals for the frontend; **what a unit of
  packaged output describes, and what triggers one**, are undecided and are the core of the
  read-path design.
- §6.2.4 establishes at-least-once and unordered delivery, which requires a means of
  **recognising a duplicate and of deciding which of two observations is newer**. Undecided.
- §6.2.5 removes operator control of the simulation, which means the situations F5 and F6 exist
  to surface must **arise on their own**. How is undecided.

---

## 7. Decisions I took without asking

Recorded per the ground rule. All are about this document, not about the product.

1. **Separating the brief's operator needs from its functional requirements**, and treating the
   needs as things the product must make answerable rather than as features to build. The
   brief's own framing supports it, but it is a choice.
2. **Splitting out-of-scope three ways** — excluded by the brief, excluded by decision, and
   still open — so that the brief's permissions do not get laundered into settled scope.
3. **Writing acceptance criteria that named their missing threshold** rather than inventing one,
   in the first draft. This left six features honestly incomplete until §6 was answered, which I
   judged better than a spec that looked finished and quietly encoded guesses.
4. **Dropping the operator-response feature** rather than keeping it as an empty section once
   observe-only was confirmed, and recording the exclusion in §4.2 instead. The trace of why it
   existed is in §2.5 and §6.1.2.
5. **Promoting "warrants attention" to its own part of the domain model** (§2.4) rather than
   leaving it implicit in the energy and freshness features. It is derived, not reported, and
   naming it as a defined two-condition list is what makes it extensible without becoming a
   dumping ground.
6. **Making route visibility and route emphasis separate concerns** in F2. Your instruction was
   about emphasis; treating it as emphasis rather than as visibility is what keeps R3 literally
   satisfied, and I want that reasoning on the record rather than inferred.

---

## 8. Traceability

Every operator need is served, and every functional requirement is met, by at least one feature.

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
| R5 Operator UX decisions | F1, F3, F4, F6, F7, F8 |
| R6 Event-driven backend | F9, F10 |
