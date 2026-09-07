# ADR-0009 — Testing and verification

- **Status:** Accepted
- **Date:** 2026-09-07
- **Related:** every preceding ADR; `PRODUCT-SPEC.md` §3 acceptance criteria, §4.2, §5
- **Imposes:** injectable clocks in the backend derivation, the client watchdog, and the simulator

## Context

The covering email says reviewers will look for functional correctness and for a good understanding of
the solution, and explicitly calls out dead or inexplicable code. There is no CI (`PRODUCT-SPEC.md`
§4.2), so whatever gate exists is one a reviewer runs by hand.

Most of what this system does is invisible. A reviewer watching the map sees dots moving; they cannot
see that a duplicate was suppressed, that a superseded observation was discarded, or that a summary
count included the vehicles sitting outside every zone. Testing has to target that gap, and the README
has to make some of it observable.

## Decision

1. **Test what fails silently. Do not test what fails visibly.**
2. **Go table-driven unit tests, plus a small integration suite driving the backend through the
   consumer interface with serialised bytes.** No end-to-end browser suite.
3. **A permutation-invariance test** is the primary evidence for ADR-0003's ordering model.
4. **"Events are the only writer" is an architectural claim evidenced by a type signature**, not by a
   test that pretends to prove absence.
5. **Each of the four ways of knowing nothing has a test.**
6. **A "does the demo still demonstrate?" test** asserts that every operator-facing state actually
   occurs in a simulated run.
7. **The freshness budget is asserted for ingest→publish only**, and instrumented beyond that.
8. **Operator-facing acceptance criteria are largely verified by a written manual checklist**, and this
   is stated as a gap rather than implied to be automated.
9. **`gofmt`, `go vet`, `staticcheck`, `tsc --noEmit` and Biome, all behind `make check`** — which
   also fails if the generated TypeScript is stale.
10. **`make dev`, `make test`, `make check`**, plus a "what to look for" section in the README.
11. **A run at 1000 vehicles before submission**, with results reported and ADR-0008 amended if
    contradicted.
12. **Backend derivation, the client watchdog and the simulator all take a clock as a dependency.**

## Options considered

### What to test (§9.1)

The selection rule is *test what fails silently*, and it partitions the system cleanly:

| Fails visibly — untested | Fails silently — tested |
|---|---|
| Map styling, marker shapes, colours | Event ordering and duplicate suppression |
| CSS and layout | Attention thresholds at their boundaries |
| Panel and legend appearance | Zone assignment, especially the no-zone case |
| Simulator plausibility | Coverage states, and summary counts reconciling with the fleet |
| | Malformed-event handling |
| | Warm-up and the completeness claim |

A wrong marker colour is obvious within seconds. A summary count that quietly omits out-of-area
vehicles is invisible until somebody reconciles by hand — and `PRODUCT-SPEC.md` F8 calls that a defect.
The rejected alternative, a uniform coverage target across the codebase, would have spent most of its
effort on the left column.

### Test shape (§9.2)

| Option | For | Against |
|---|---|---|
| **Unit tests plus a byte-level integration suite — chosen** | Because ADR-0007 put serialisation on the simulator/ingest boundary, one integration test exercises validation, projection, derivation and the read path together. Unit tests carry the boundary cases. | The integration tests are coarse: a failure points at the whole pipeline rather than a line. |
| Add an end-to-end browser suite | Would cover the operator-facing criteria. | Brittle, slow, needs a harness — and what it verifies is precisely what fails visibly. A flaky suite also teaches a team to ignore failures. |
| Unit tests only | Fastest to write and run. | Nothing would exercise serialisation, the discard path, or the interaction between projection and derivation. |

### Evidence for the ordering model (§9.3)

**Permutation invariance is the test, because it is the claim.** ADR-0003 asserts that applying a set of
events in any order, with arbitrary duplication, yields identical state. So: take a sequence, shuffle it
many ways, duplicate events arbitrarily, and assert every run ends in the same state.

If that test ever fails it means a delta has been introduced somewhere — which is the single thing
ADR-0003 identified as breaking the model. The test therefore guards a stated constraint rather than an
implementation detail.

Alongside it, targeted cases: a route cleared before it was assigned, interleaved signals for one
vehicle, a superseded position arriving late, telemetry before registration.

### The single-writer guarantee (§9.8)

| Option | For | Against |
|---|---|---|
| **Point at the read-only interface; add a behavioural integration test — chosen** | The type signature is the actual guarantee. The integration test confirms state appears only as a consequence of events. Honest about what each contributes. | Neither rules out a future writer added *inside* the projection package. |
| A test asserting no other writer exists | Would look thorough. | **A test cannot prove absence.** It would be theatre, and the covering email penalises exactly that kind of code. |

Recorded plainly: this is an architectural property, demonstrated by inspection, with a behavioural test
as corroboration rather than proof.

### The four ways of knowing nothing (§9.9)

- **Filling** — integration test: before replay completes, the snapshot's lifecycle says so.
- **Empty** — no vehicles registered; the trivial case.
- **Disconnected** — client watchdog unit test: advance a fake clock with no snapshots arriving.
- **Filtered to nothing** — filter composition unit test.

### The test that is not in the register: does the demo still demonstrate? (§9.6)

ADR-0007 recorded that simulator tuning could silently make an operator-facing state unreachable, and
that this is a regression even though no product code changes. Drain rate, starting battery spread,
dropout probability and assignment rate are interdependent, and a change to one can stop a state
occurring.

So a test runs the simulator across a simulated period and asserts that **at least one vehicle goes
stale, one crosses the energy threshold, one leaves the service area, and one is awaiting a first
report.**

This is the only test here whose failure means *the submission no longer shows what it claims to show*.
It is also the only one that would catch that failure at all — nothing else in the suite depends on the
simulator's parameters.

### The freshness budget (§9.4)

Asserted: an event ingested before a tick appears in the **next** published snapshot, and per-event
processing sits well under the tick. That is the real property, because the tick bounds latency from
below regardless of how fast processing is.

Not asserted: anything past the process boundary, per ADR-0006. Instrumented in dev, and the README
claims "backend asserted, client instrumented".

### Operator-facing behaviour (§9.5)

Pure logic — filters, selectors, the mapping from state to presentation — gets unit tests. Summary and
panel get a handful of component tests against snapshot fixtures, which are cheap and catch wiring
errors that type checking does not.

⚠️ **Beyond that, the spec's acceptance criteria are verified by a human following a checklist derived
from them, and this is the real coverage gap in the submission.** It is defensible for a prototype whose
UI fails visibly, and it is a gap. The README states it rather than leaving a reader to assume the
criteria are automated.

### Tooling and the gate (§9.6, §9.7)

`make check` runs `gofmt`, `go vet`, `staticcheck`, `tsc --noEmit` in strict mode and Biome —
**and fails if the generated TypeScript is out of date.**

⚠️ **Amended in implementation: Biome replaces ESLint and Prettier, and pnpm replaces npm.** Biome is
one dev dependency doing what two did, with one config file rather than two and nothing to reconcile
between a formatter and a linter that disagree — which matters here for the same reason the
prerequisite list is short: every tool is another thing between a reviewer and the code. pnpm follows
from the same instinct, and is pinned through `packageManager` so corepack provides it rather than a
global install. Neither changes what the gate checks. ADR-0001's single-source-of-truth property
holds only while drift is detectable; without that gate it silently stops being true, which is the
failure mode that ADR flagged for itself.

⚠️ **Amended in implementation: CI arrived** — the case this ADR made for it below, and no more than
that. A GitHub Actions workflow runs `make check` and `make test` on pushes to `main` and on pull
requests. It supplies the toolchain and calls the same two targets a reviewer calls, so the Makefile
remains the single definition of the gate and CI cannot check something a local run does not. Nothing
about the tests themselves changed; the slower-tests-become-affordable half of the argument below has
not been spent.

The README's **"what to look for"** section is doing real work rather than being courtesy: it turns
invisible correctness into something observable in about two minutes. Watch a vehicle go stale and come
back. Watch one cross the energy threshold. Watch a customer-driven vehicle leave the service area.
Read the log for `discarded: duplicate` and `discarded: superseded` — that is at-least-once and
unordered delivery being handled, and it is otherwise entirely invisible.

### Injectable clocks (§9.12, imposed)

Backend staleness derivation, the client watchdog and the simulator all naturally want to read the clock
directly. If any of them does, the corresponding test above cannot exist: staleness boundary cases,
the disconnected-view test, and the demo-still-demonstrates test all require controlling time.

So all three take a clock as a dependency. Recorded as a design constraint because it is cheap up front
and invasive to retrofit, and because it is not obvious from any earlier ADR that three separate
components need it for the same reason.

⚠️ **Amended in implementation: four components, not three — the projector takes a clock too.** For the
same reason as the others: ADR-0003 §3.5 asks for implausible observation timestamps to be logged, which is
a comparison against now, and a producer running ahead of the backend would push staleness permanently out
of reach while nothing on screen said so. Testing that requires controlling the projector's clock as well
as the deriver's. The count was low because the projection was not thought of as time-dependent; it reads
a timestamp and judges it, so it is.

## Consequences

**Positive**

- Test effort concentrates where failures are invisible, which is where this system's real risk sits.
- The two most important tests — permutation invariance and demo-still-demonstrates — each guard a
  *stated claim* rather than an implementation, so they stay meaningful under refactoring.
- The single-writer guarantee is claimed at the right strength, with the type system doing the work.
- `make check` protects the generated-contract property that ADR-0001 depends on.
- A reviewer can observe the invisible behaviour in a couple of minutes, guided.

**Negative / accepted costs**

- **Most operator-facing acceptance criteria are manually verified.** This is the largest gap and it is
  stated rather than hidden.
- **No end-to-end coverage at all**, so a wiring failure between a correct backend and a correct
  frontend would only be caught by a human running it.
- **Injectable clocks add a parameter to three components** that would otherwise be simpler.
- **The integration tests are coarse**, so a failure localises poorly.
- **The demo-still-demonstrates test is probabilistic in nature** and pinned by the seed, so it needs
  a fixed seed to be stable — which means it verifies the *seeded* run, not every run.
- `staticcheck` is a fourth tool, obtained as a pinned Go tool dependency rather than installed, but it
  is still one more thing in the loop.

## What would make us revisit this

- **The UI growing beyond a handful of surfaces**, at which point manual checklist verification stops
  scaling and E2E coverage starts to earn its cost.
- **A wiring failure actually shipping**, which is the empirical argument for a single smoke-level E2E
  test.
- **CI arriving**, which changes `make check` from a gate a reviewer runs into one that runs itself, and
  makes slower tests affordable. *(Since happened, in part: the gate runs itself, the suites are
  unchanged — see the amendment above.)*
- **The client gaining logic worth testing** — currently it holds almost none, which is why frontend
  coverage is thin. A write path would change that immediately.
- **The 1000-vehicle run needing to be repeatable** rather than one-off, which would make it a
  benchmark rather than a pre-submission task.

## At ~1000 vehicles

- **Permutation invariance is unaffected** — it is a property of the projection, tested on small
  sequences, and fleet size does not enter into it.
- **The integration suite would slow down** if run at scale, so it stays at a small fleet; the
  1000-vehicle case is a separate deliberate exercise (§9.10) rather than part of the suite.
- **The demo-still-demonstrates test becomes more valuable**, because a larger simulated fleet has more
  interdependent tuning and more ways for a state to stop occurring.
- **The freshness assertion tightens usefully**: at ten times the event rate, "processing well under the
  tick" is a real constraint rather than a formality, and the same test starts to earn its place as a
  performance guard.
- **Manual checklist verification degrades sharply**, since most criteria are about legibility and a
  human cannot eyeball a thousand markers — which is the same human ceiling ADR-0008 identified, arriving
  in the test strategy.
