# Architecture Decision Records

Ten ADRs covering every technical decision in Fleet Radar. Product decisions live in
`../PRODUCT-SPEC.md` (§2–§5 for the decisions, §7 for the reasoning).

Each ADR carries context, the options considered with their trade-offs, the decision, consequences
including accepted costs, what would make us revisit it, and how it holds up at ~1000 vehicles.

**On the section numbers.** A reference like §4.2 names one decision within an area of the design — area 1
the stack, 2 the map, 3 the event model, and so on, one area per ADR. Each ADR's subsection headings carry
the numbers it answers. Where an ADR cites `PRODUCT-SPEC.md` §7.x it names that document, which numbers
its sections independently.

**Decisions taken while writing the code** are folded into the ADR each one amends, marked
⚠️ **Amended in implementation**. They are not a separate document, because several of them contradict the
ADR they belong to, and a contradiction is only legible next to what it contradicts.

## If you read three

| | Why this one |
|---|---|
| [0003 — The inbound event model](0003-inbound-event-model.md) | Where the correctness of the whole system lives. Duplicate suppression needs no bookkeeping, and the reason why is the most load-bearing insight in the design. |
| [0005 — The read path](0005-the-read-path.md) | Why full snapshots over SSE, and how five separate failure modes collapse into one behaviour because of it. |
| [0008 — Scale posture](0008-scale-posture.md) | The ~1000 vehicle answer, ranked and measured. Short, because scale is recorded per-decision throughout. |

## All ten

| ADR | Decides |
|---|---|
| [0001](0001-stack-repository-and-contract.md) | Go backend, React frontend, repository shape, and how a contract survives two languages. Also settles that derivation happens in the backend, and why staleness forces it. |
| [0002](0002-map-and-rendering.md) | MapLibre with vehicles as one data-driven layer. No interpolation, no clustering, coverage inking only problem zones. Two rendering traps that fail silently. |
| [0003](0003-inbound-event-model.md) | Six event types, absolute values only, per-signal sequence ordering. Registration replay. Why unordered delivery is the accurate reading of Kafka rather than pessimism. |
| [0004](0004-ingest-state-and-derivation.md) | One process, one store port, mutex writes with per-tick snapshots. Makes "events are the only writer" a type signature. Closes both correctness bugs the spec predicted. |
| [0005](0005-the-read-path.md) | Full snapshots on a 200 ms tick over SSE. One message per generation. A watchdog rather than transport errors. |
| [0006](0006-frontend-architecture.md) | Fleet state outside React; one boundary rule. Selection shareable as a link, filters never. Honest about what the freshness budget can and cannot assert. |
| [0007](0007-simulated-event-source.md) | Road graph, teledriving state machine, serialised bytes across the ingest seam. The two fictions that exist so specified states are observable. |
| [0008](0008-scale-posture.md) | The technical envelope reaches 1000 vehicles; the human envelope does not. Deferred work with triggers. |
| [0009](0009-testing-and-verification.md) | Test what fails silently. Permutation invariance, and a test that the demo still demonstrates. Injectable clocks. |
| [0010](0010-the-submission.md) | README as entry point, what ships, what to prepare for — including the three critiques we expect. |

## Notes on reading these

**Four decisions were changed after being challenged**, and the ADRs record the change rather than the
tidied outcome:

- The first three ADRs were product decisions and were folded into the spec (see the commit history).
- Registration events were removed, then reinstated with replay once the bootstrap problem was understood
  (ADR-0003).
- The storage port arrived from outside the walkthrough (ADR-0004).
- Two of my own earlier claims were corrected in place: that persisting live telemetry would be unsafe
  (ADR-0004), and that simulated delivery delay would cause false staleness (ADR-0007, correcting
  `PRODUCT-SPEC.md` §7.1).

**Three decisions are cross-referenced from more than one ADR** because they were forced early by a
dependency running opposite to the register's ordering: backend derivation (§4.6, in ADR-0001), tick-based
publishing (§5.2, in ADR-0004), and the amendments to the product spec recorded in ADR-0002, ADR-0004 and
ADR-0006.
