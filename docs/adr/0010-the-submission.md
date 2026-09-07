# ADR-0010 — The submission

- **Status:** Accepted
- **Date:** 2026-09-07
- **Decides:** `ARCHITECTURE-DECISIONS-TO-MAKE.md` §10.1–§10.8
- **Related:** `PRODUCT-SPEC.md` §5; ADR-0008, ADR-0009

## Context

The brief's deliverables are backend code, frontend code, instructions to run locally, and a short
README covering how to run it, the architecture and data flow at ~100 vehicles, and the key tradeoffs.
The covering email adds that reviewers will look for simplicity and functional correctness, will
penalise dead or inexplicable code, expect the traces of AI tool use to be shared, and will assess
understanding of the solution independently of how it was produced.

There are also two follow-up sessions — an hour of code review and live coding, and an hour of system
design — so the submission is the input to a conversation rather than a terminal artefact.

## Decision

1. **The README is an entry point, not a summary.** It does not restate the spec or the ADRs.
2. **Everything ships**: the product spec, the architecture decisions register, and all ten ADRs, with
   a stated reading order marking the ADRs as depth-on-demand.
3. **One ASCII diagram** traces a single event from source to pixel, with a paragraph per hop.
4. **Five tradeoffs are named explicitly**, plus observe-only as the framing product decision.
5. **Three predictable critiques are prepared for**, not just the happy path.
6. **The system design hour leads with ADR-0008's ranking and its measurements.**
7. **Deferred work is presented as a table of choices with triggers**, linked rather than restated.
8. **A deliberate pre-submission pass** looks for anything not traceable to a spec feature.

## Options considered

### What the README contains (§10.1)

| Option | For | Against |
|---|---|---|
| **Entry point: run it, look at this, here's the shape, here's what we traded, read on if you want — chosen** | Serves the reader with ten minutes who has read nothing, and gets them to the running system fast. Satisfies the brief's three mandated items without duplicating anything. | Requires the deeper documents to be genuinely navigable, since the README will not carry them. |
| A self-contained README covering everything | One document to read. | Would duplicate the spec and ADRs, and duplicated documents diverge. |
| A minimal README, just run instructions | Shortest. | Ignores two of the brief's three mandated items. |

The **"what to look for"** section is load-bearing rather than courtesy. Most of what this system does
correctly is invisible: a reviewer watching the map sees dots moving and has no way to know that
duplicates were suppressed, that a superseded observation was discarded, or that the summary counted the
vehicles sitting outside every zone. Two minutes of guided observation — watch a vehicle go stale and
recover, watch one cross the energy threshold, watch a customer-driven vehicle leave the service area,
read the log for `discarded: duplicate` and `discarded: superseded` — converts the invisible into the
observed.

### Whether the documents ship (§10.2)

⚠️ **The risk is real and worth naming rather than assuming away: ten documents against a small codebase
can read as over-documentation**, and the covering email explicitly warns against bells and whistles.

Shipping them anyway, for three reasons:

- The email states that reviewers will assess understanding of key decisions, architecture, tradeoffs and
  extensibility. The ADRs *are* that, in the form it was asked for.
- The README marks them as depth-on-demand with a reading order, so nobody is obliged to read all of it.
- **The documents only look disproportionate if the code is thin.** The mitigation is therefore that the
  implementation must be substantial and clean — which places the obligation on the code, not on
  trimming the reasoning.

A fourth point is available but is evidence rather than argument: the transcript shows these were
produced in dialogue, and that several were **changed by challenge** — the initial ADRs were folded into
the spec when they turned out to be product decisions, registration replay was reinstated after being
removed, the storage port was introduced from outside, and two earlier claims of mine were corrected in
place. Documents that record being wrong are harder to mistake for generated filler.

### Describing the architecture (§10.3)

One diagram tracing a single event end to end, with a paragraph per hop:

```
simulator ──JSON bytes──▶ consumer iface ──▶ validate ──▶ projection ──▶ store
                                                                          │
                                          ┌───────── 200ms tick ──────────┘
                                          ▼
                                       derive ──▶ snapshot ──SSE──▶ module store ──▶ map
                                                                              └──▶ React
```

**ASCII rather than Mermaid**, specifically because delivery is a zip rather than a hosted repository:
Mermaid renders on GitHub and nowhere else, and a diagram that renders as source is worse than one that
renders as text. A single traced event is chosen over a component diagram because the brief asks for
*data flow*, and because tracing one event is also the thing to be able to do from memory in the
follow-up session.

### The named tradeoffs (§10.4)

Selected as the ones a reviewer would otherwise have to ask about:

1. **Go over full-stack TypeScript** — the shared-contract property was lost and paid for with
   generated types. Chosen because it is the language Vay runs.
2. **Full snapshots over deltas** — bandwidth spent to buy self-healing simplicity and correct
   slow-client behaviour.
3. **Backend derivation over client-side** — driven by staleness needing to be correct while
   disconnected, not by performance.
4. **SSE over WebSocket** — matched to a deliberately unidirectional system.
5. **Derive rather than cache**, for zone membership and staleness — recomputation cost accepted in
   exchange for having no invalidation logic to get wrong.

Plus **observe-only** as the framing product tradeoff, because it is upstream of most of the others.

### Preparing for the follow-up sessions (§10.5, §10.6)

**Three critiques are predictable and are worth answers rather than improvisation:**

- *Why an interface with one implementation?* The `FleetStore` port. ADR-0004 gives the answer — it owns
  concurrency — and that answer has to still be true once the code exists. If it is not, the port should
  collapse into a struct before submission.
- *Why is the simulator this large?* Road graph, state machine, assignment scheduler, energy model,
  delivery layer, ticker. ADR-0007 justifies each; the risk is that it is the largest component in a
  submission whose subject is the backend.
- *Why this much documentation?* Answered above.

**The likely live-code asks are all small by design** — add a fourth attention condition, add a filter,
change a threshold, handle a new event type. Knowing precisely which files each touches is worth more
than rehearsing an explanation.

**The system design hour leads with ADR-0008's ranking**, backed by measurement rather than reasoning:
the technical envelope reaches 1000 vehicles and the human envelope does not. Then the Kafka story —
what was modelled (topics, vehicle-keyed partitioning, at-least-once, compaction semantics behind
registration replay) against what was not (offsets, consumer groups, rebalancing, dead-letter). The
sharpest available question is *why design for unordered delivery when Kafka orders within a partition*,
and ADR-0003 has the answer: ordering is never guaranteed across topics. Then multi-operator territories
as the honest gap.

### Making omissions read as choices (§10.7)

ADR-0008 §8.9's deferral table pairs every deferred change with the trigger that would prompt it, and
`PRODUCT-SPEC.md` §4.2 lists what is excluded by decision. The README links to both. A reader can
disagree with a *trigger*, which is not possible with a silent omission — that is the point of the form.

### The slop pass (§10.8)

One rule, taken from `PRODUCT-SPEC.md` §5: **anything in the repository that does not serve a feature in
§3 is a defect.** A file-by-file read asking "which acceptance criterion does this serve?", hunting
specifically for:

- comments restating the code;
- the `FleetStore` port having grown methods nobody calls;
- unused exports, dead constants, leftover scaffolding;
- defensive handling of cases that cannot occur;
- doc comments on trivial functions;
- the generated TypeScript not being unmistakably marked as generated.

`staticcheck` and `go vet` catch some of this mechanically; the read-through catches the rest. Rejected
alternative: relying on tooling alone, which finds unused code but not code that is used and pointless.

## Consequences

**Positive**

- A reviewer with ten minutes reaches a running system and knows what to look at.
- The reasoning behind every contested decision is available without being imposed.
- The invisible behaviour — which is most of the interesting behaviour — is made observable.
- The follow-up sessions have prepared answers for the weakest points, not just the strongest.
- Confidentiality is handled as an explicit step rather than left to chance.

**Negative / accepted costs**

- **The documentation-to-code ratio is a genuine risk** and is mitigated rather than eliminated.
- **The README depends on the deeper documents being navigable**, so a poor reading order undermines the
  whole structure.
- **Stripping the transcript is a manual step** that has to actually happen, and it is the kind of step
  that gets skipped at the end of a task.
- **Three known-weak points are being submitted knowingly**: the one-implementation port, the size of the
  simulator, and the manual verification of most operator-facing criteria.
- The ASCII diagram is less pretty than a rendered one.

## What would make us revisit this

- **Delivery becoming a hosted repository** rather than a zip, which makes Mermaid worthwhile and changes
  how the documents are navigated.
- **The code turning out small** relative to the documentation, which would argue for consolidating the
  ADRs rather than shipping ten.
- **The `FleetStore` port failing its own justification** once written, which means removing it before
  submission rather than defending it afterwards.
- **The 1000-vehicle run producing an unflattering result**, which changes what the system design hour
  leads with — but is reported either way, per ADR-0008.

## At ~1000 vehicles

Not applicable in the usual sense; the submission is not a running system. The one connection is that
**ADR-0008's claims must be measured before this README is written**, because the scale section quotes
numbers. If the measurement contradicts the ranking, both ADR-0008 and the README's scale section change
together — and the honest version of that is reporting what happened, not what was predicted.
