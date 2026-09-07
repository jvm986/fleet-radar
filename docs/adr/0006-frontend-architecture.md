# ADR-0006 — Frontend architecture

- **Status:** Accepted
- **Date:** 2026-09-07
- **Decides:** `ARCHITECTURE-DECISIONS-TO-MAKE.md` §6.1–§6.11
- **Related:** `PRODUCT-SPEC.md` F1, F3, F4, F6, F8, §5, §7.5, §7.6; ADR-0001, ADR-0002, ADR-0005
- **Amends:** `PRODUCT-SPEC.md` §4.2 and F3 — the selected vehicle may be carried in a link

## Context

ADR-0001 chose React on the explicit grounds that **vehicle updates would bypass the framework's
render cycle**, which is what made React's re-render model irrelevant rather than merely tolerable.
That claim is discharged here or not at all.

ADR-0005 established that the client receives complete snapshots on a 200 ms tick, requests nothing,
and renders only what it is told. Much of this ADR is therefore consequence rather than choice — which
is a good sign, and worth stating plainly rather than dressing up as fresh decisions.

## Decision

1. **Fleet state lives in a module-level store outside React.** React components subscribe to slices
   via `useSyncExternalStore`.
2. **One boundary rule: snapshot → map and React; interaction state → React → map.**
3. **The map never re-renders.** It is updated imperatively with one `setData` per snapshot.
4. **Components re-render only when their own slice changes.**
5. **No previous vehicle state is retained.**
6. **There is no unexpected-vehicle case.** The snapshot is authoritative and is rendered faithfully.
7. **The selected vehicle is carried in the URL. Nothing else is.**
8. **Layer visibility persists in local storage. Nothing else does**, and a test asserts it.
9. **Hand-written CSS modules. No styling framework, no component library.**
10. **The freshness budget is asserted for the backend portion and instrumented for the client
    portion**, with a publish timestamp carried in each snapshot.
11. **Staleness freezing while disconnected requires no client code.**

## Options considered

### Where fleet state lives (§6.1, §6.9)

| Option | For | Against |
|---|---|---|
| **Module store + `useSyncExternalStore` — chosen** | Discharges ADR-0001's premise that vehicle updates bypass React, which is the whole reason React was a safe choice. A React primitive, so no dependency. | Two state mechanisms coexist, so the boundary between them has to be explicit rather than assumed. |
| React state at the top of the tree | One mechanism, conventional. | Re-renders the tree five times a second and couples map updates to the render cycle — reintroducing precisely the weakness ADR-0001 claimed was neutralised. |
| Zustand or Redux | Familiar, good tooling, selector support built in. | A dependency for an application with four stateful surfaces, and the map would still need to opt out of it. |

The boundary is stated as one rule because it is the thing most likely to erode: **snapshot flows to
both the map and React; interaction state flows from React to the map, never the reverse.** Selection is
the only value crossing in that direction — the map is told which vehicle is emphasised and never
decides it.

### Rendering and re-render granularity (§6.2, §6.4)

The map is imperative and outside React. The summary re-renders when counts change, which is often, but
it renders a handful of numbers. The detail panel re-renders on its vehicle's slice. Legend and filters
re-render on interaction only, since `config` is static.

Building a feature collection of roughly a hundred features per tick is a few hundred allocations a
second — negligible, and the same cost ADR-0002 identified as the first rendering concern at ten times
the fleet size.

### Retained state and reconciliation (§6.7, §6.8)

Both are consequences rather than decisions. No previous state is kept because there is no
interpolation (ADR-0002), no history (`PRODUCT-SPEC.md`), and snapshots carry absolute values. There is
no unexpected-vehicle case because each snapshot is complete: the client never merges, never remembers,
and never reconciles. The only capability a previous snapshot would enable is animated marker
transitions, explicitly declined in ADR-0002.

### URL state (§6.3)

| Option | For | Against |
|---|---|---|
| **Selection only — chosen** | Makes a vehicle **shareable**, which is exactly the handoff need behind N4 and N6: "look at LV-042" becomes a link rather than a spoken identifier. A URL is explicit and inspectable, unlike invisible local storage. | Requires amending a spec exclusion that had bundled selection with filters. |
| Nothing in the URL | Simplest; leaves the spec untouched. | Loses sharing, which is the one genuinely operator-facing benefit available here. |
| Selection and filters | Fully shareable view. | **A colleague could be sent a link that silently hides most of the fleet** — the "operator concludes the fleet is four vehicles" failure, now arriving from outside and with no reason for the recipient to suspect it. |

⚠️ This amends `PRODUCT-SPEC.md` §4.2, which had said filters and selection never survive a load. The
amendment rests on a principle rather than an exception:

> **A link that selects a vehicle adds information. A link that filters removes it.**

That distinction is why selection may be explicit and shareable while filters may be neither persisted
nor linkable.

### Local storage (§6.11)

Layer visibility only. The guarantee that no filter ever persists is currently upheld by nobody writing
one, which is exactly the kind of invariant that regresses the first time someone adds a convenience. A
test asserting local storage holds only the layer key after filters have been applied is cheap insurance
on a stated product guarantee.

### Styling (§6.6)

| Option | For | Against |
|---|---|---|
| **Hand-written CSS modules — chosen** | The whole UI surface is a summary bar, a panel, a legend and a filter control — a few hundred lines. No config, no build step to explain, scoped by default. | No design system, so consistency is maintained by hand. |
| Tailwind | Fast to write, consistent spacing for free. | Configuration to explain and styling embedded in the markup, in a repository being read for clarity. |
| A component library | Accessible components out of the box. | Brings opinionated visuals to a brief that states polish is not important, and a large dependency for four surfaces. |

### Measuring the freshness budget (§6.5)

| Option | For | Against |
|---|---|---|
| **Publish timestamp in the snapshot; backend portion asserted, client portion logged in dev — chosen** | Honest about what is verifiable. The deterministic half gets a real test; the non-deterministic half gets a measurement. | The end-to-end figure is never asserted, so the budget is evidenced rather than guaranteed. |
| A visible latency readout in the UI | Continuously visible. | `PRODUCT-SPEC.md` §4.2 excludes an operator-facing counters surface, and this is developer instrumentation, not operator information. |
| Assert end-to-end in a browser test | Would cover the whole path. | Timing through a real rendering pipeline is flaky, and a flaky assertion about a performance budget teaches a team to ignore it. |

⚠️ Recorded deliberately: **this budget cannot be fully tested and the claim should not pretend
otherwise.** Ingest-to-publish is deterministic and testable in Go. Publish-to-paint spans a process
boundary and a rendering pipeline, and is bounded from below by the 200 ms tick regardless. The claim
made in the README should be "backend asserted, client instrumented", not "verified".

## Consequences

**Positive**

- ADR-0001's premise is discharged with a React primitive rather than a dependency, so the framework
  choice stands on something real.
- The most performance-sensitive path in the client — the map — has no framework in it at all.
- Much of this ADR is consequence rather than decision, which means the earlier decisions were
  load-bearing in the right direction.
- Selection is shareable, which is the only place in the whole design where the client does something
  for handoff that the backend cannot.
- A product guarantee about persistence is protected by a test rather than by intent.

**Negative / accepted costs**

- **Two state mechanisms** in one application. The boundary rule is short, but it is a rule someone has
  to know, and violating it would be easy and would not fail loudly.
- **The map is imperative**, so map behaviour cannot be reasoned about by reading the component tree —
  it lives in effects and setters.
- **Selection now exists in two places** — React state and the URL — which must be kept in step.
- **The freshness claim is partly evidential.** Anyone reading the README needs the distinction made
  plainly or it overclaims.
- **No design system**, so visual consistency is manual and will drift if the surface grows.
- The summary re-renders at tick rate even when its numbers are unchanged, unless memoised on value
  rather than identity.

## What would make us revisit this

- **The client gaining a write path**, which would introduce optimistic state and immediately need a
  reconciliation model — the thing §6.8 currently gets for free.
- **Deltas replacing snapshots** at scale, which ends "no retained state" and "no reconciliation" in one
  move, and makes the module store meaningfully more complex than a mutable reference.
- **The UI surface growing** beyond a handful of components, at which point hand-written CSS and manual
  selector wiring stop paying and a store library or design system earns its place.
- **Animated transitions being wanted**, which needs previous state and reopens ADR-0002's interpolation
  decision.
- **Filters needing to be shareable**, which would require solving the hidden-fleet problem for
  recipients — most plausibly by making an inherited filter conspicuous on arrival.
- **The freshness budget being contested**, which would need a real end-to-end harness rather than dev
  logging.

## At ~1000 vehicles

- **The store and the boundary rule are unaffected.** Holding one snapshot in a module reference costs
  the same whatever its size.
- **`setData` per tick becomes the bottleneck**, as ADR-0002 predicted: serialising and parsing a
  thousand features five times a second dominates the client's work. The mitigations are ADR-0002's —
  feature-state updates for style-only changes, sending only changed vehicles, or a custom layer — and
  all of them require the client to hold previous state, which ends §6.7 and §6.8 together.
- **Keeping the map out of React matters far more**, not less. A thousand React-rendered markers would be
  unworkable, so the decision that makes React acceptable at a hundred vehicles is the same decision
  that makes it survivable at a thousand.
- **The summary and panel are unaffected**, since both render a bounded amount of information regardless
  of fleet size.
- **Client-side filtering becomes expensive** — filtering a thousand vehicles per tick before handing
  them to the map — and the obvious fix, filtering server-side, breaks ADR-0005's single-shared-snapshot
  property. That tension is recorded there rather than solved here.
- **URL selection and local storage are unaffected**, being constant in size.
