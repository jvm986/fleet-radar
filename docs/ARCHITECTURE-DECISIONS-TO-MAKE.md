# Fleet Radar — Architecture Decisions To Make

Every open **technical** decision, as a question. No options, no recommendations, no answers —
those come from the walkthrough, and each settled group is written up as an ADR in `docs/adr/`.

**What happened to the product decisions.** This document began as a single register covering
product and architecture together. Its first three sections — domain semantics, scope boundaries,
and operator information design — are now settled and live in `PRODUCT-SPEC.md`: the decisions
themselves in §2–§5, and the reasoning, costs, revisit triggers and scale notes in §7. They are not
repeated here. What remains is everything that will become an ADR.

**How this is organised.** Sections are ordered by dependency: a question appears in the earliest
section where it can be asked without presupposing an answer we have not yet reached. Section 1 comes
first because the stack constrains everything after it.

**Numbering is stable within a section.** New questions are appended and take the next number;
nothing renumbers. Numbers are references for the ADRs, not a priority order.

**Tags.** *(spec)* marks a question that exists because `PRODUCT-SPEC.md` requires something of the
implementation — these are the ones where a wrong technical answer breaks a stated product
guarantee. *(permission)* marks one where the brief explicitly allowed us something and we are
choosing deliberately rather than accepting by default.

## Planned ADRs

One per section, unless a section proves to be two decisions or two sections prove to be one.

| § | Area | ADR |
|---|---|---|
| 1 | Stack, repository and contract | `0001` |
| 2 | Map and rendering | `0002` |
| 3 | Inbound event model | `0003` |
| 4 | Ingest and state | `0004` |
| 5 | The read path | `0005` |
| 6 | Frontend architecture | `0006` |
| 7 | The simulated event source | `0007` |
| 8 | Scale posture at ~1000 vehicles | `0008` |
| 9 | Testing and verification | `0009` |
| 10 | The submission | `0010` |

---

## 1. Stack, repository and contract

The foundational choices. Everything below depends on these, which is why they come first even
though several of them read as tooling trivia.

- **1.1** — What language and runtime does the backend use?
- **1.2** — What framework, if any, builds the UI?
- **1.3** — How are backend and frontend arranged in the repository?
- **1.4** — What tooling and package manager?
- **1.5** — How is the contract between backend and frontend kept from drifting?
- **1.6** — What must a reviewer install before they can run it?
- **1.7** — How many commands does it take to get it running?
- **1.8** — What configuration is exposed, and how? *(spec)*
- **1.9** — Where do the thresholds and the service-area geometry live, such that they are
  inspectable and changeable in one place? *(spec)*

---

## 2. Map and rendering

Depends on §1 for the framework, and on `PRODUCT-SPEC.md` §7.5 and §7.6 for what has to be drawn.

- **2.1** — What draws the map?
- **2.2** — Where does the base map imagery come from, and what does that mean for running the
  system without internet access?
- **2.3** — What draws the vehicles, given how many there are and how often they move?
- **2.4** — How does the operator move around the map, and are they held within the service area?
- **2.5** — How does a vehicle's position on screen change between one report and the next?
- **2.6** — What happens visually when many vehicles occupy the same small area?
- **2.7** — How are faint routes and the emphasised route drawn, such that ten of the former do not
  overwhelm one of the latter? *(spec)*
- **2.8** — How are the service area boundary and its zones drawn? *(spec)*
- **2.9** — How is per-zone coverage state drawn without competing with the vehicles? *(spec)*
- **2.10** — How is a route's destination marked? *(spec)*
- **2.11** — How does the map behave as the detail panel opens and closes, given the selected vehicle
  must stay in view and the fleet must not appear to jump? *(spec)*
- **2.12** — How are hover and selection hit-testing handled at ~100 overlapping markers? *(spec)*
- **2.13** — How is the map rendered when the basemap is unavailable? *(spec)*

---

## 3. Inbound event model

Depends on §1 for where the contract lives. `PRODUCT-SPEC.md` settled that signals arrive as
individual events, at-least-once and unordered; these settle what that entails.

- **3.1** — What distinct kinds of event arrive?
- **3.2** — What does one telemetry event carry, given signals arrive individually?
- **3.3** — What identifies an event, such that the same event arriving twice can be recognised as
  the same event? *(spec)*
- **3.4** — Given events are unordered, how does the backend establish which of two observations of
  the same signal happened later? *(spec)*
- **3.5** — Whose clock do event timestamps come from, and what happens when it disagrees with the
  backend's?
- **3.6** — What does a route event carry, and what marks a route as no longer current?
- **3.7** — What is a route, geometrically?
- **3.8** — Is a vehicle having stopped reporting something the stream tells us, or something we
  infer? *(spec)*
- **3.9** — How does the backend learn that a vehicle exists?
- **3.10** — Does a vehicle ever leave the fleet, and if so how does the backend learn that? *(spec)*
- **3.11** — What does the backend do with an event it cannot make sense of? *(spec)*
- **3.12** — Is there an age beyond which an event should no longer be applied?
- **3.13** — How often does a vehicle report, and does that depend on what it is doing? *(spec)*
- **3.14** — What is the wire format of an event?
- **3.15** — How much of Kafka's actual shape — keys, partitions, offsets, consumer groups,
  compaction — does the design need to reckon with for "as if Kafka were the source of truth" to
  mean anything?
- **3.16** — Are events about a single vehicle related to each other in any way the backend can rely
  on?

---

## 4. Ingest and state

Depends on §3 for what is being consumed.

- **4.1** — Is the backend one process?
- **4.2** — Where does current fleet state live while the system runs? *(permission)*
- **4.3** — What is the relationship between the code that consumes events and the code that serves
  the operator?
- **4.4** — How is concurrent access to fleet state handled?
- **4.5** — Which parts of what the operator sees are computed once for everyone, and which per
  client? *(spec)*
- **4.6** — *Decided early in ADR-0001: in the backend.* §1.8 and §1.9 could not be answered without
  it, so the dependency ran opposite to this document's ordering. What remains open here is how that
  derivation is structured and when it runs, not where it happens.
- **4.7** — What does the backend do when events arrive faster than it can process them?
- **4.8** — What does the backend serve before it has learned the fleet, and how does it know when it
  has? *(spec)*
- **4.9** — After a restart, how does the backend come to know the fleet again? *(spec)*
- **4.10** — How does the backend decide a vehicle has gone stale, given nothing arrives to tell it
  so? *(spec)*
- **4.11** — Where is a vehicle's zone determined, and what triggers recomputing it, given coverage
  changes on status transitions as well as on movement? *(spec)*
- **4.12** — How are vehicles belonging to no zone represented, so that they cannot be silently
  dropped from a total? *(spec)*

---

## 5. The read path

Depends on §4 for what state exists. `PRODUCT-SPEC.md` set the 250 ms budget; these settle how it is
met.

- **5.1** — What does one unit of output to the frontend describe? *(spec)*
- **5.2** — *Decided early in ADR-0004: publishing is tick-based.* §4.4's concurrency model depends on
  it, so it could not wait. What remains open here is the tick interval and how it relates to the
  250 ms budget.
- **5.3** — How does a newly connected client obtain the fleet as it currently stands? *(spec)*
- **5.4** — Does the client ever ask the backend for anything, or only receive?
- **5.5** — What carries data from backend to browser?
- **5.6** — How is route geometry delivered, given every remotely-driven vehicle's route is drawn but
  only one is emphasised? *(spec)*
- **5.7** — How does the client detect that its connection has failed, within the freshness budget?
  *(spec)*
- **5.8** — What happens when a failed connection comes back? *(spec)*
- **5.9** — What happens to a client that cannot keep up with the rate of change?
- **5.10** — What guarantees that the operator is never shown a partial fleet as though it were
  complete? *(spec)*
- **5.11** — Is filtering applied before data leaves the backend, or after it arrives? *(spec)*
- **5.12** — Does one mechanism carry fleet state, coverage state and the summary figures, or several?
  *(spec)*
- **5.13** — How does the client learn the thresholds the legend has to state? *(spec)*

---

## 6. Frontend architecture

Depends on §5 for what arrives and §2 for what is drawn.

- **6.1** — How is fleet state held in the browser?
- **6.2** — What causes the UI to re-render, and at what granularity?
- **6.3** — Is any of the operator's view reflected in the URL?
- **6.4** — How does the UI keep up when a hundred vehicles are changing continuously?
- **6.5** — How is the 250 ms budget observed rather than assumed? *(spec)*
- **6.6** — Where does styling come from?
- **6.7** — How does the frontend know what a vehicle looked like a moment ago, if it needs to?
- **6.8** — What does the frontend do with state for a vehicle it did not expect?
- **6.9** — Where does the boundary sit between state the map owns and state the framework owns?
  *(spec)*
- **6.10** — How is per-vehicle staleness prevented from advancing while the view is disconnected?
  *(spec)*
- **6.11** — What persists layer visibility, and what guarantees that no filter ever persists?
  *(spec)*

---

## 7. The simulated event source

Depends on §3 for what it must emit and §4 for what consumes it. `PRODUCT-SPEC.md` settled that it is
hidden from the operator, which is what makes several of these load-bearing.

- **7.1** — Does the simulator run inside the backend process? *(permission)*
- **7.2** — What exactly crosses the boundary from simulator to ingest, and is it identical to what a
  real broker would deliver? *(spec)*
- **7.3** — How does the simulator decide where vehicles are, and how they move?
- **7.4** — Where do the routes it assigns come from?
- **7.5** — How does it model energy consumption?
- **7.6** — What causes a vehicle to change state?
- **7.7** — How do the low-energy and stale-vehicle situations come about, given the operator cannot
  cause them and must still be able to see them? *(spec)*
- **7.8** — How does the simulator produce duplicate and out-of-order delivery? *(spec)*
- **7.9** — Is a run of the simulation reproducible?
- **7.10** — Does the fleet exist from the first instant, or come into being as events arrive?
  *(spec)*
- **7.11** — How much of the simulator's behaviour is configurable, and by whom? *(spec)*
- **7.12** — How does a WITH_CUSTOMER vehicle move, given it has no route the system knows about, and
  how does it come to leave the service area? *(spec)*
- **7.13** — Does the simulator ever produce a vehicle that behaves wrongly, and should it?
- **7.14** — How does it keep its reporting cadence regular enough not to trip staleness falsely,
  given the threshold tolerates only one interval of jitter? *(spec)*

---

## 8. Scale posture at ~1000 vehicles

Depends on §1–§7, because this is an analysis of those decisions rather than a separate design. The
brief asks us to be ready to discuss this; the spec excludes implementing it.

- **8.1** — Which part of the design is expected to give way first as the fleet grows tenfold?
- **8.2** — What changes about how events are ingested?
- **8.3** — What changes about how state is held?
- **8.4** — What changes about what is sent to the browser?
- **8.5** — What changes about what the map draws?
- **8.6** — Does the operator still want the whole fleet on one screen at that size, and if not, what
  takes its place?
- **8.7** — Does one operator still supervise the whole fleet?
- **8.8** — What must be built now, at a hundred, for the thousand-vehicle story to be credible
  rather than aspirational?
- **8.9** — What do we deliberately not do now, and how do we make that read as a choice?
- **8.10** — How would we find out whether any of these claims are true?
- **8.11** — Does the meaning of coverage change when the fleet is ten times denser?
- **8.12** — Does the single-process assumption survive, and what breaks first if it does not?

---

## 9. Testing and verification

Depends on everything above for what there is to test.

- **9.1** — What in this system must have automated tests for us to trust it?
- **9.2** — What kinds of test do we write, and where do they live in relation to the code?
- **9.3** — How is duplicate and out-of-order delivery handling tested? *(spec)*
- **9.4** — How is the freshness budget tested? *(spec)*
- **9.5** — How is operator-facing behaviour verified?
- **9.6** — What type checking, linting and formatting do we hold ourselves to, and is it enforced?
- **9.7** — What can a reviewer run to convince themselves the system does what we claim?
- **9.8** — How do we demonstrate that events are the only writer of state, rather than asserting it?
  *(spec)*
- **9.9** — How are the four ways of knowing nothing tested, given three of them are hard to provoke
  by hand? *(spec)*

---

## 10. The submission

Depends on everything. The brief specifies deliverables; the covering email specifies what reviewers
will look for, including the traces of AI tool use.

- **10.1** — What belongs in the README, given these documents exist?
- **10.2** — Do the spec, this register, and the ADRs ship with the submission?
- **10.3** — How is the architecture and data flow described for a reader who will not run it?
- **10.4** — Which tradeoffs do we name as the key ones, of all the ones we made?
- **10.5** — How is the session transcript captured and shared?
- **10.6** — What do we prepare for the code review and live coding hour?
- **10.7** — What do we prepare for the system design hour?
- **10.8** — How do we make deliberate omissions read as decisions rather than as gaps?
- **10.9** — What in the repository would a reviewer reasonably read as AI slop, and how do we find
  out before they do?
