# Fleet Radar — Decisions To Make

Every open decision, as a question. No options, no recommendations, no answers — those come
from the walkthrough, and land in `docs/adr/`.

**How this is organised.** Sections are ordered by dependency: a question appears in the
earliest section where it can be asked without presupposing an answer we have not yet reached.
That is why, for example, transport and rendering questions sit well below the information-design
questions that determine what needs transporting and rendering at all.

**Numbering is stable within a section.** New questions get appended to their section and take
the next number; nothing renumbers. Numbers are references for the ADRs, not a priority order.

**Scope.** Questions marked *(spec)* exist only because `PRODUCT-SPEC.md` raised them — they are
not in the brief. Questions marked *(permission)* are ones where the brief explicitly allowed us
something and we are choosing deliberately rather than accepting by default.

---

## 1. Remaining product semantics

The spec settled what the product is. These settle what its terms mean. Almost everything below
depends on these.

- **1.1** — At what energy level does a vehicle become a charging candidate, and what should that
  level be reasoned from? *(spec)*
- **1.2** — How many distinct degrees of energy concern does the operator need to tell apart? *(spec)*
- **1.3** — After how long without a report does a vehicle's information stop being trustworthy,
  and what should that duration be derived from? *(spec)*
- **1.4** — What should the operator still be able to see and conclude about a vehicle whose
  information has gone stale?
- **1.5** — Does the operator need to tell the two attention conditions apart on the map itself,
  or only know that something is wrong? *(spec)*
- **1.6** — What does the operator see for a vehicle that is both low on energy and stale at the
  same time? *(spec)*
- **1.7** — What does the hard-coded coverage expectation consist of?
- **1.8** — What makes coverage in a part of the service area low?
- **1.9** — Which vehicle states constitute coverage?
- **1.10** — Does a vehicle that is available but low on energy count as coverage?
- **1.11** — Does the operator need to tell a remote driver taking a vehicle *to* a customer from
  one taking a vehicle *away* afterwards, given the brief gives both the same status?
- **1.12** — What defines the Las Vegas service area, and how faithful to a real operating area
  does it need to be?
- **1.13** — Can a vehicle be outside the service area, and does that mean anything to the operator?
- **1.14** — How is a vehicle identified, given the operator has to be able to say it out loud?
- **1.15** — How should the operator perceive how far through its journey a remotely-driven vehicle
  is?
- **1.16** — What does the operator need to understand about a vehicle whose reported position is
  not on its route?

---

## 2. Scope boundaries still open

The spec lists these as proposed exclusions (§4.3) awaiting sign-off. Each is a decision about
what this system is not, and several change the architecture rather than just the feature list.

- **2.1** — Does fleet state survive a backend restart? *(permission)*
- **2.2** — Is any history of past fleet state retained? *(permission)*
- **2.3** — Is there authentication, or any notion of who the operator is? *(permission)*
- **2.4** — How many operators can view the fleet at once?
- **2.5** — What happens when one operator opens a second browser tab? *(spec)*
- **2.6** — Does the view need to work anywhere other than a desktop browser at operator-desk size?
- **2.7** — What accessibility standard, if any, do we hold ourselves to?
- **2.8** — Are there locales, languages or unit systems to support beyond one?
- **2.9** — Does anything need to survive being deployed, or is running locally the whole target?
- **2.10** — What operational visibility does the running system owe a developer debugging it?
- **2.11** — Does the system need to behave sensibly if the event source stops entirely? *(spec)*

---

## 3. Operator information design

Depends on §1 for what the terms mean and §2 for who is looking. This is where the spec's
acceptance criteria become a screen.

- **3.1** — What does the operator see in the first moment the page loads, before they interact
  with anything?
- **3.2** — How is a vehicle's state made visible on the map?
- **3.3** — How is a vehicle's energy level made visible on the map, if at all?
- **3.4** — How is a vehicle's heading made visible?
- **3.5** — How is a vehicle that warrants attention made to stand out from a hundred that do not?
- **3.6** — How does the operator find a specific vehicle whose identifier they already have?
- **3.7** — What happens to the selection when the selected vehicle changes state? *(spec)*
- **3.8** — What happens to the selection when an active filter would hide the selected vehicle? *(spec)*
- **3.9** — What happens to the selection when the selected vehicle stops being reported? *(spec)*
- **3.10** — Where does the fleet summary sit in relation to the map, and can the operator act on it?
- **3.11** — Does the operator need the fleet as a list as well as a map?
- **3.12** — How does the operator tell one vehicle having gone quiet from the entire view having
  gone stale? *(spec)*
- **3.13** — How is coverage presented in relation to the vehicles themselves?
- **3.14** — Can filters be combined, and what should the operator expect when they are? *(spec)*
- **3.15** — How does the operator learn what thresholds the UI is applying on their behalf? *(spec)*
- **3.16** — How does the operator tell "no data has arrived yet" from "the fleet is genuinely
  empty"? *(spec)*
- **3.17** — How much of the screen belongs to the map, and what earns the rest?
- **3.18** — What, if anything, does the operator see about a vehicle without selecting it, beyond
  what the map encodes?

---

## 4. Map and geospatial

Depends on §3 for what has to be drawn.

- **4.1** — What draws the map?
- **4.2** — Where does the base map imagery come from, and what does that mean for running the
  system without internet access?
- **4.3** — What draws the vehicles, given how many there are and how often they move?
- **4.4** — What is a route, geometrically?
- **4.5** — Where does route geometry come from?
- **4.6** — How does the operator move around the map, and are they held within the service area?
- **4.7** — How does a vehicle's position on screen change between one report and the next?
- **4.8** — What happens visually when many vehicles occupy the same small area?
- **4.9** — How are faint routes and emphasised routes drawn such that ten of the former do not
  overwhelm one of the latter? *(spec)*
- **4.10** — How is the service area boundary itself drawn?

---

## 5. Inbound event model

Depends on §1 for what a vehicle's state means. The spec settled that signals arrive as individual
events, at-least-once, unordered — these settle what that actually entails.

- **5.1** — What distinct kinds of event arrive?
- **5.2** — What does one telemetry event carry, given signals arrive individually?
- **5.3** — What identifies an event, such that the same event arriving twice can be recognised as
  the same event? *(spec)*
- **5.4** — Given events are unordered, how does the backend establish which of two observations of
  the same signal happened later? *(spec)*
- **5.5** — Whose clock do event timestamps come from, and what happens when it disagrees with the
  backend's?
- **5.6** — What does a route event carry, and what marks a route as no longer current?
- **5.7** — Is a vehicle having stopped reporting something the stream tells us, or something we
  infer? *(spec)*
- **5.8** — How does the backend learn that a vehicle exists?
- **5.9** — What does the backend do with an event it cannot make sense of?
- **5.10** — Is there an age beyond which an event should no longer be applied?
- **5.11** — How often does a vehicle report, and does that depend on what it is doing?
- **5.12** — What is the wire format of an event, and where does its definition live?
- **5.13** — How much of Kafka's actual shape — keys, partitions, offsets, consumer groups,
  compaction — does the design need to reckon with for "as if Kafka were the source of truth" to
  mean anything?
- **5.14** — Are events about a single vehicle related to each other in any way the backend can
  rely on?

---

## 6. Backend: ingest and state

Depends on §5 for what is being consumed.

- **6.1** — What language and runtime does the backend use?
- **6.2** — Is the backend one process?
- **6.3** — Where does current fleet state live while the system runs? *(permission)*
- **6.4** — What is the relationship between the code that consumes events and the code that serves
  the operator?
- **6.5** — How is concurrent access to fleet state handled?
- **6.6** — Which parts of what the operator sees are computed once for everyone, and which are
  computed per client?
- **6.7** — Is derived information — attention conditions, coverage, summary figures — computed in
  the backend or in the browser? *(spec)*
- **6.8** — What does the backend do when events arrive faster than it can process them?
- **6.9** — What does the backend serve before it has seen any of the fleet? *(spec)*
- **6.10** — After a restart, how does the backend come to know the fleet again? *(spec)*
- **6.11** — How does the backend decide a vehicle has gone stale, given nothing arrives to tell it
  so? *(spec)*

---

## 7. The read path: backend to browser

Depends on §6 for what state exists and §3 for what the operator needs from it. The spec settled
the 250 ms budget; these settle how it is met.

- **7.1** — What does one unit of output to the frontend describe? *(spec)*
- **7.2** — What causes a unit of output to be produced? *(spec)*
- **7.3** — How does a newly connected client obtain the fleet as it currently stands? *(spec)*
- **7.4** — Does the client ever ask the backend for anything, or only receive?
- **7.5** — What carries data from backend to browser?
- **7.6** — How is route geometry delivered, given every remotely-driven vehicle's route is drawn
  but only one is emphasised? *(spec)*
- **7.7** — How does the client detect that its connection has failed, within the freshness budget? *(spec)*
- **7.8** — What happens when a failed connection comes back? *(spec)*
- **7.9** — What happens to a client that cannot keep up with the rate of change?
- **7.10** — What guarantees that the operator is never shown a partial fleet as though it were
  complete? *(spec)*
- **7.11** — Is filtering applied before data leaves the backend, or after it arrives? *(spec)*
- **7.12** — Does the same mechanism carry both fleet state and the summary figures? *(spec)*

---

## 8. Frontend architecture

Depends on §7 for what arrives and §3 for what is rendered.

- **8.1** — What framework, if any, builds the UI?
- **8.2** — How is fleet state held in the browser?
- **8.3** — What causes the UI to re-render, and at what granularity?
- **8.4** — Is any of the operator's view reflected in the URL?
- **8.5** — Does any of the operator's view survive a page reload? *(spec)*
- **8.6** — How does the UI keep up when a hundred vehicles are changing continuously?
- **8.7** — How is the 250 ms budget observed rather than assumed? *(spec)*
- **8.8** — Where does styling come from?
- **8.9** — How does the frontend know what a vehicle looked like a moment ago, if it needs to?
- **8.10** — What does the frontend do with a vehicle it has state for but did not expect?

---

## 9. The simulated event source

Depends on §5 for what it must emit and §6 for what consumes it. The spec settled that it is
hidden from the operator, which is what makes several of these load-bearing.

- **9.1** — Does the simulator run inside the backend process? *(permission)*
- **9.2** — What exactly crosses the boundary from simulator to ingest, and is it identical to what
  a real broker would deliver? *(spec)*
- **9.3** — How does the simulator decide where vehicles are, and how they move?
- **9.4** — Where do the routes it assigns come from?
- **9.5** — How does it model energy consumption?
- **9.6** — What causes a vehicle to change state?
- **9.7** — How do the low-energy and stale-vehicle situations come about, given the operator cannot
  cause them and must still be able to see them? *(spec)*
- **9.8** — How does the simulator produce duplicate and out-of-order delivery? *(spec)*
- **9.9** — Is a run of the simulation reproducible?
- **9.10** — Does the fleet exist from the first instant, or come into being as events arrive? *(spec)*
- **9.11** — How much of the simulator's behaviour is configurable, and by whom? *(spec)*
- **9.12** — How does a WITH_CUSTOMER vehicle move, given it has no route the system knows about? *(spec)*
- **9.13** — Does the simulator ever produce a vehicle that behaves wrongly — off its route, or
  reporting implausible values — and should it?

---

## 10. Scale posture at ~1000 vehicles

Depends on §5–§9, because this is an analysis of those decisions rather than a separate design. The
brief asks us to be ready to discuss this; the spec excludes implementing it.

- **10.1** — Which part of the design is expected to give way first as the fleet grows tenfold?
- **10.2** — What changes about how events are ingested?
- **10.3** — What changes about how state is held?
- **10.4** — What changes about what is sent to the browser?
- **10.5** — What changes about what the map draws?
- **10.6** — Does the operator still want the whole fleet on one screen at that size, and if not,
  what takes its place?
- **10.7** — Does one operator still supervise the whole fleet?
- **10.8** — What must be built now, at a hundred, for the thousand-vehicle story to be credible
  rather than aspirational?
- **10.9** — What do we deliberately not do now, and how do we make that read as a choice?
- **10.10** — How would we find out whether any of these claims are true?
- **10.11** — Does the meaning of coverage change when the fleet is ten times denser?

---

## 11. Testing and verification

Depends on everything above for what there is to test.

- **11.1** — What in this system must have automated tests for us to trust it?
- **11.2** — What kinds of test do we write, and where do they live in relation to the code?
- **11.3** — How is duplicate and out-of-order delivery handling tested? *(spec)*
- **11.4** — How is the freshness budget tested? *(spec)*
- **11.5** — How is operator-facing behaviour verified?
- **11.6** — What type checking, linting and formatting do we hold ourselves to, and is it enforced?
- **11.7** — What can a reviewer run to convince themselves the system does what we claim?
- **11.8** — How do we demonstrate that events are the only writer of state, rather than asserting it? *(spec)*

---

## 12. Repository, toolchain and local operation

Depends on §6 and §8 for what is being packaged.

- **12.1** — How are backend and frontend arranged in the repository?
- **12.2** — What tooling and package manager?
- **12.3** — What must a reviewer install before they can run it?
- **12.4** — How many commands does it take to get it running?
- **12.5** — How is the contract between backend and frontend kept from drifting?
- **12.6** — Is there a commit history, and does it need to tell a story?
- **12.7** — What configuration is exposed, and how? *(spec)*

---

## 13. The submission

Depends on everything. The brief specifies deliverables; the covering email specifies what
reviewers will look for, including sharing the traces of AI tool use.

- **13.1** — What belongs in the README, given these documents exist?
- **13.2** — Do the spec, this register, and the ADRs ship with the submission?
- **13.3** — How is the architecture and data flow described for a reader who will not run it?
- **13.4** — Which tradeoffs do we name as the key ones, of all the ones we made?
- **13.5** — How is the session transcript captured and shared?
- **13.6** — What do we prepare for the code review and live coding hour?
- **13.7** — What do we prepare for the system design hour?
- **13.8** — How do we make deliberate omissions read as decisions rather than as gaps?
- **13.9** — What in the repository would a reviewer reasonably read as AI slop, and how do we know
  before they do?
