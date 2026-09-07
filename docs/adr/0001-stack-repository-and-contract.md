# ADR-0001 — Stack, repository shape, and the cross-language contract

- **Status:** Accepted
- **Date:** 2026-09-07
- **Decides:** `ARCHITECTURE-DECISIONS-TO-MAKE.md` §1.1–§1.9, and §4.6 ahead of its section
- **Related:** `PRODUCT-SPEC.md` §5, §7.4, §7.5, §7.6, F6, F8, F9
- **Constrains:** §3.14 (wire format), §5.5 (transport), §5.12, §5.13, §6.11

## Context

The brief leaves technology choices fully open and requires only that the solution run locally.
Everything downstream depends on this decision, which is why it is first.

One substantive constraint comes from outside engineering merit: **Vay runs Go in its backend**, and
demonstrating the language the team actually uses is worth more in a take-home than internal stack
symmetry. That is a deliberate, stated reason rather than a default.

A second constraint emerged mid-decision. Two of these questions — where thresholds live, and where
the service-area geometry lives — turned out to be unanswerable without first settling whether
derived information is computed in the backend or the browser (§4.6). The register had ordered §1
before §4 on the assumption that the stack constrains everything; here the dependency ran the other
way. §4.6 is therefore decided in this ADR rather than deferred to ADR-0004.

## Decision

1. **The backend is Go.** Current stable Go, single module.
2. **The frontend is React with TypeScript, built by Vite.**
3. **One repository, three concerns kept visibly separate:** a Go module for the backend, a Vite app
   for the web client, and a contract expressed as Go types with TypeScript generated from them.
4. **Go types are the single source of truth for the contract.** TypeScript types are generated from
   them and the generated output is committed.
5. **Prerequisites are the Go toolchain, Node LTS, and `make`.** Nothing else — no Docker, no
   database, no broker, no globally installed CLIs.
6. **`make dev` is the single command** to go from a fresh clone to a running system. `make test`,
   `make check` and `make generate` accompany it.
7. **Derived information is computed in the backend** — attention conditions, zone assignment,
   per-zone coverage state, and whole-fleet summary counts. **Filtering stays in the client**, being
   per-viewer view state.
8. **Thresholds live in one Go constants module and their values are sent to the client** for
   display. The client holds no copy of its own.
9. **The service area and zone geometry is one checked-in GeoJSON file**, embedded into the binary
   with `go:embed` and served to the client for drawing.

## Options considered

### Backend language (§1.1)

| Option | For | Against |
|---|---|---|
| **Go — chosen** | The language Vay runs, so the submission demonstrates relevant capability rather than adjacent capability. Real parallelism and goroutines, so the ingest path scales without contortion. First-class Kafka consumer libraries, which turns "structured as if Kafka were the source of truth" from a shape we mimic into a client we would swap in. Makes concurrent access to fleet state a real design question with real answers. | Loses the shared-contract property entirely. Two languages, two toolchains, two test runners, two lint configurations in a small repository. |
| TypeScript on Node | The contract could be a shared package — no codegen, no drift, one toolchain, one install. Fastest path to a coherent full-stack whole. | Single-threaded for CPU work, so the honest scale answer would have been "at 1000 vehicles move the consumer to Go or the JVM", with a seam to explain. Demonstrates a stack Vay does not run in its backend. |
| Kotlin / JVM | Strongest Kafka ecosystem of the three. | Heavy for a local prototype; slowest to a running system; also not Vay's backend language. |
| Python / FastAPI | Quick to write. | Weakest for a sustained high-frequency push loop, weakest typing, no contract sharing. |

The shared-contract argument is what made TypeScript the recommendation before Vay's stack was
weighed. It is a real cost and is recorded as such rather than glossed: see §1.5 for how it is paid.

### Frontend framework (§1.2)

| Option | For | Against |
|---|---|---|
| **React + Vite + TypeScript — chosen** | Reviewers read it fluently. Mature map library bindings. Vite gives a fast dev loop with no configuration to explain. | React's re-render model is a poor fit for a hundred entities changing continuously. |
| Svelte | Fine-grained reactivity genuinely suits high-frequency updates better. | Fewer reviewers fluent; thinner map ecosystem. |
| Vanilla TypeScript | No framework, no re-render problem at all. Viable for something this map-dominant. | Hand-rolled DOM for the panel, filters and summary costs more than the framework does, and reads as idiosyncratic. |

The re-render objection is largely neutralised by ADR-0002's rendering approach: **vehicle updates go
to a map data source, not through component state**, so the framework only handles the panel, filters
and summary, which are low-frequency. That is what makes React's weakness irrelevant here rather than
merely tolerable.

### The cross-language contract (§1.5)

| Option | For | Against |
|---|---|---|
| **Go types as source, TypeScript generated, output committed — chosen** | One source of truth. The direction of generation matches the direction of authority: the backend consumes events and derives state, the client consumes the backend's output. Generator runs via `go run`, so no prerequisite beyond the Go toolchain already required. Committed output means the web build needs nothing extra, and drift is *visible* as a diff when regenerating. | One-directional. Go-shaped optionality (pointers, `omitempty`) leaks into the TypeScript. Generated code in the tree must be unmistakably marked, and regeneration must be part of `make check` or it rots silently. |
| Neutral schema — Protobuf, JSON Schema or TypeSpec — generating both | Language-neutral and the correct answer for a system with several consumers. Gives runtime validation on both sides. | Requires `protoc` and plugins, contradicting the "Go and Node, nothing else" prerequisite. Introduces a third definition language for a two-consumer system. |
| Hand-write both sides, add a contract test | No tooling at all; both sides idiomatic. | Drift is possible by construction and a test catches only what it covers. The option that fails silently. |

⚠️ This constrains §3.14: generating TypeScript interfaces from Go structs presumes a JSON-shaped
wire format. Choosing Protobuf or another binary encoding later would reopen this decision.

### Where derived information is computed (§4.6, taken early)

| Option | For | Against |
|---|---|---|
| **Backend — chosen** | Staleness is correct while disconnected by construction. All viewers see identical coverage and counts. Fleet-level aggregation happens once rather than once per browser. Coherent: whole-fleet summary counts are a server-side aggregate by definition, so deriving coverage anywhere else splits one concern across two machines. | The backend holds product policy, not just projection, so a threshold change is a backend change. The contract carries derived and presentation-adjacent concepts alongside raw domain state. |
| Browser | Backend stays a pure event → state → stream pipe, which is cleaner to review. Smaller payloads. | Staleness would need explicit suspension while disconnected, and getting it wrong produces the exact false claim `PRODUCT-SPEC.md` §7.6 forbids: a hundred healthy vehicles marked stale because of one failed socket. Duplicates aggregation across viewers. |

**Staleness is what settles this.** It is the only operator-visible state derived from *absence*
rather than from an event, so something must evaluate the passage of time. If that something is the
client, a dropped connection floods the map with false staleness. If it is the backend, a
disconnected client simply receives nothing further and the flags freeze — the required behaviour,
free. And once the backend must hold the reporting cadence and the staleness rule, the marginal cost
of it also holding the 20% line is nil. The "thin backend that knows no thresholds" position is not
available in pure form.

### Thresholds and geometry (§1.8, §1.9)

Both follow from §4.6. The client renders what it is told rather than holding its own copy, because
with two languages a generated TypeScript copy of the constants could be edited independently and go
stale — at which point **the legend would lie about what the system is actually applying.**
`PRODUCT-SPEC.md` requires the legend to state the thresholds, so this is a correctness requirement,
not tidiness. Serving the geometry rather than duplicating it is the same argument: the backend needs
it for zone assignment, the client needs it for drawing, and two copies is one too many.

Alternatives considered and declined: environment variables and a `.env` file, which would imply the
values vary per environment when there is one environment, and add a setup step that can be got
wrong; and geometry as a runtime-loaded file, which buys a failure mode for a file that never changes.

### Prerequisites and the run command (§1.6, §1.7)

| Option | For | Against |
|---|---|---|
| **`make`, Go, Node — chosen** | Make is already present on macOS and Linux. Nothing to install beyond the two language toolchains. `make test` and `make check` come along for free. | Needs WSL on Windows. |
| `just` | Better syntax and fewer footguns than Make. | Requires installing `just`, contradicting §1.6. |
| Docker Compose | Uniform environment including Windows, one command. | Requires Docker running and is slower to start. Containerises a system whose entire storage story is "in memory", so the container would exist purely to orchestrate two processes — and it puts a build layer between the reviewer and the code they are here to read. |

Every prerequisite is another way a reviewer's run fails on something that is not our code.

## Consequences

**Positive**

- The scale story needs no caveat. There is no "at 1000 vehicles I would move off this runtime"
  seam to explain, because the runtime chosen is the one that answer would have moved to.
- Concurrency in the state store becomes an explicit design decision (§4.4) rather than something
  the event loop hid.
- Backend and frontend are genuinely separate artefacts, matching how the brief's deliverables are
  written.
- The legend cannot disagree with the logic, because there is one copy of every threshold and the
  client never owns it.
- Fleet-level derivation happens once, which is the right shape at any fleet size.

**Negative / accepted costs**

- **Two languages, two toolchains.** Two test runners, two linters, two formatting conventions, and
  a reviewer who needs both installed. This is the price of the Go decision and it is not small.
- **Generated code in the tree.** It must be obviously generated, and `make check` must fail if it
  is out of date, or the single-source-of-truth property quietly stops holding.
- **The contract carries derived concepts.** Attention flags, zone membership and coverage state are
  product conclusions, not raw domain facts, and they now travel on the wire alongside position and
  battery. That couples the wire format to product policy: changing what "warrants attention" means
  changes the contract.
- **Product policy lives in the backend.** Adjusting a threshold is a backend change and cannot be
  done in the frontend alone.
- **Make excludes Windows reviewers** without WSL.
- Go-shaped optionality leaks into the generated TypeScript, so the client sees nullability that
  reflects Go's encoding rather than the domain.

## What would make us revisit this

- **A second consumer of the events** — a mobile client, another service, an analytics sink. At that
  point a neutral schema with generated bindings for each consumer becomes the right answer and the
  Go-as-source decision is what gives way.
- **Thresholds needing to vary** per operator, per city, or at runtime. The constants module then
  becomes configuration, and sending values to the client stops being sufficient because the client
  would need to know they can change.
- **The frontend needing to change presentation policy without a backend release**, which argues for
  moving some derivation back to the client — and would require solving the disconnected-staleness
  problem explicitly.
- **A binary wire format** for payload reasons, which reopens §1.5 because Go-to-TypeScript type
  generation presumes JSON.
- **A Windows reviewer or contributor**, which replaces Make with something cross-platform.
- **The backend needing to serve more than one operator organisation**, at which point geometry
  stops being a checked-in constant.

## At ~1000 vehicles

- **Go is the reason this scales without a rewrite.** Ingest parallelises across goroutines, one per
  partition, with the vehicle identity from ADR-0003 as the partition key. Derivation can be
  parallelised across zones or vehicle ranges. Neither is available on a single-threaded runtime
  without introducing worker processes and the serialisation between them.
- **Backend derivation pays off precisely here.** A thousand point-in-polygon tests plus aggregation
  happen once per tick, not once per tick per connected browser. Under the browser-side alternative,
  the weakest machines in the system would each repeat the whole fleet's worth of geometry work.
- **Sending thresholds and geometry stays free.** Both are constant in size regardless of fleet size;
  more zones would grow the geometry slightly, and zone count is a property of the city, not the
  fleet.
- **The generated contract is unaffected**, as is Make.
- **The single Go module and single process is the thing that gives way** (§8.12). The likely
  progression is the simulated source moving out of the process first, then ingest partitioning
  across consumers, with the state store becoming the contended resource — which is why §4.4 is worth
  deciding deliberately now rather than by accident.
- **React remains viable** only because vehicle updates bypass the framework. If that ever stops
  being true, the framework becomes the bottleneck long before the backend does.
