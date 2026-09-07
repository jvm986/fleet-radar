# ADR-0002 — Map and rendering

- **Status:** Accepted
- **Date:** 2026-09-07
- **Related:** `PRODUCT-SPEC.md` F1, F2, F3, F7, §7.5, §7.6; ADR-0001
- **Amends:** `PRODUCT-SPEC.md` F2 — destination marking narrowed to the emphasised route

## Context

`PRODUCT-SPEC.md` §7.5 and §7.6 settled what the map must convey: one directional marker per
vehicle carrying heading by orientation and status by colour plus fill; energy as a binary flag;
attention as an additive halo and badge drawn above everything; all routes faint with the selected
one emphasised; zones always drawn with coverage as a toggleable layer; hover for labels; and four
distinct ways of knowing nothing, none of which may render as a blank map.

This ADR decides how that is drawn, and by what. ADR-0001 already fixed React and TypeScript in the
client, and established that vehicle updates should bypass the framework's render cycle — that
constraint is discharged here.

## Decision

1. **MapLibre GL JS** renders the map.
2. **Base map imagery comes from key-free hosted vector tiles.** Internet access at run time is
   assumed.
3. **Vehicles are one symbol layer over one GeoJSON source**, styled by data-driven expressions.
   `icon-allow-overlap` is set explicitly.
4. **Pan and zoom are free within generous maximum bounds**, with a minimum zoom and a reset
   control. The operator is *not* confined to the service area.
5. **Positions are rendered as reported. No interpolation between reports.**
6. **No clustering.**
7. **Routes are two line layers over one source** — all routes faint in the lower layer, the
   selected vehicle's route emphasised in the upper one.
8. **The service area boundary and zone boundaries are lines with labels, no fill**, drawn beneath
   vehicles and routes.
9. **Coverage is a per-zone fill in which only problem zones receive any ink.** Zones meeting their
   expectation are not filled at all. Below-minimum and zero-available are differentiated by pattern
   as well as colour.
10. **A destination marker is drawn only for the emphasised route.**
11. **The camera uses MapLibre's padding** to account for the detail panel, rather than manual
    offset arithmetic.
12. **Hit-testing uses `queryRenderedFeatures` over a small pixel box, topmost feature wins.**
13. **If the basemap fails to load, everything else still renders** on a plain background with a
    non-blocking notice.

## Options considered

### What draws the map (§2.1)

| Option | For | Against |
|---|---|---|
| **MapLibre GL JS — chosen** | GPU vector rendering with data-driven styling, so a hundred vehicles are rows in a source rather than a hundred objects. Marker rotation is a first-class property, which matters because every vehicle carries heading. No API key. Goes to 1000 vehicles without a rewrite, which is what makes the scale story credible rather than aspirational. | Heavier API surface; styling expressions have a real learning curve and read less obviously than imperative code. |
| Leaflet | Simplest API, vast ecosystem, quickest to something on screen. | DOM and canvas markers: acceptable at 100, painful at 1000, and per-marker rotation is awkward. Would need replacing precisely when the scale question is asked. |
| deck.gl layered over MapLibre | Best-in-class for large datasets and GPU-side updates. | Overkill at 100 vehicles and an additional abstraction to justify. The thing to reach for *if* symbol layers stop coping, not before. |

⚠️ **Amended in implementation: MapLibre is pinned to 5.x, not the latest.** `maplibre-gl@6.7.0` — the
current `latest` — renders background layers and nothing else here: no tile requests are issued, `load`
never fires, and no error is raised. Reproduced with a fifteen-line map containing none of this project's
code, in both the dev server and a production build, on a real GPU with working workers; 5.24.0 renders
correctly. The choice above is unaffected. What is worth recording is how it got in: taking `latest` from
a package manager is not a decision, and the failure mode it bought was a blank map with a clean console.

### Base map imagery (§2.2)

| Option | For | Against |
|---|---|---|
| **Key-free hosted vector tiles — chosen** | No signup, no API key, nothing for a reviewer to configure. Real street context, so "is that vehicle on a road" is a meaningful question. | A run-time third-party dependency with rate limits, which can break independently of our code. |
| A PMTiles extract served by our own backend | Genuinely offline and immune to a dead tile host. Cheaper under this stack than it would have been otherwise, since `go:embed` is already in use for geometry and Go serves range requests trivially. One self-contained artefact. | Tens of megabytes of binary asset in the repository, plus a PMTiles client dependency. |
| No base map at all | No dependency whatsoever. | Loses street context entirely, and with it the ability to judge whether a vehicle is anywhere sensible. |

The offline question was raised explicitly and resolved: "must run locally" means *runs on your
machine*, not air-gapped, so internet access at run time is an accepted assumption. §2.13 keeps the
failure graceful anyway, because the cost of doing so is small and the alternative is a cosmetic
dependency taking the radar down.

### What draws the vehicles (§2.3)

| Option | For | Against |
|---|---|---|
| **One data-driven symbol layer — chosen** | One source update per tick regardless of fleet size. Everything §7.5 requires — rotation from heading, colour and fill from status, halo as a separate layer beneath, flagged vehicles raised — is expressible declaratively. Hit-testing comes free. Discharges ADR-0001's requirement that vehicle updates bypass React. | Style expressions are less immediately readable than imperative marker code, so the encoding rules live in a syntax a reviewer may not know. |
| One DOM marker per vehicle | Trivially inspectable in devtools; hover and click are ordinary DOM events. | A hundred DOM nodes mutated continuously is jank-prone, and a thousand is not viable. Would need replacing for the scale story. |
| A custom canvas or WebGL layer | Complete control over drawing and update cost. | Hand-rolled hit-testing and substantially more code, for nothing MapLibre does not already provide at this scale. |

⚠️ **MapLibre hides colliding symbols by default.** Left at the default, vehicles in dense areas would
silently disappear from the map — a direct violation of F1's requirement that individual vehicles stay
distinguishable, presenting as a data bug rather than as a styling default. `icon-allow-overlap` must
be set explicitly. Recorded because this is the kind of default that ships unnoticed and is then
diagnosed in the wrong layer.

### Navigation constraints (§2.4)

| Option | For | Against |
|---|---|---|
| **Free within generous bounds, plus minimum zoom and a reset control — chosen** | Keeps out-of-area vehicles reachable, which is required: the spec allows a customer to drive a vehicle anywhere. Stops the operator getting lost at world scale, and gives them a way back. | The bounds are arbitrary and have to be chosen. |
| Clamped to the service area polygon | The operator can never lose the fleet. | **Directly contradicts the decision that vehicles may leave the service area** — an out-of-area vehicle would be visible on no reachable viewport. |
| No constraints at all | Simplest. | The operator can pan into empty ocean with no orientation cue and no route back. |

### Movement between reports (§2.5)

| Option | For | Against |
|---|---|---|
| **Render as reported — chosen** | Honest: the map shows what was observed and nothing else. At roughly one report per second a city vehicle moves around fifteen metres, which is a few pixels at working zoom, so the jumps are barely perceptible. Less code and no per-frame animation loop. | Motion is discrete rather than smooth, which reads as less polished. |
| Interpolate between reported positions | Smoother, more like a radar. | **Invents position data**, and a vehicle that goes stale would keep gliding across the map unless the animation were explicitly suspended — contradicting F6's requirement that a stale vehicle's values read as untrustworthy. Same principle that rejected the ETA: do not render what we do not know. |

### Dense areas (§2.6)

| Option | For | Against |
|---|---|---|
| **No clustering — chosen** | Preserves F1's requirement that individual vehicles remain distinguishable and selectable. At a hundred vehicles across a city, zoom is sufficient. | Vehicles genuinely overlap in tight areas, relying on zoom and the hit-test tie-break to resolve. |
| Cluster below a zoom threshold | Standard practice, and the right answer at 1000. | Replaces vehicles with counts, and would swallow the flagged vehicles §7.5 requires to be always visible and on top. Hides more than it helps at this size. |

### Route emphasis (§2.7)

| Option | For | Against |
|---|---|---|
| **Two line layers over one source — chosen** | Guarantees the emphasised route draws above every faint one. | Two layers to keep in step with one source. |
| One layer with data-driven width and opacity | Fewer layers, one place for the styling. | **Z-order cannot be controlled per feature within a layer**, so the emphasised route could be drawn underneath a faint one — the exact failure the emphasis exists to prevent. |

### Zones, coverage, and destinations (§2.8, §2.9, §2.10)

Boundaries are drawn as lines rather than fills because the coverage layer already provides zone
shading, and two fills would compound into a wash over the whole map. Zone labels are required
because zone names are the operator's language for handoff.

For coverage, the alternative was shading every zone by its state. **Only inking problem zones was
chosen because healthy zones then cost nothing visually**: in a well-covered city the layer is nearly
invisible and the map stays clean, with ink appearing exactly where the operator needs to look. Colour
is paired with pattern because the colour-alone prohibition applies here as everywhere.

Destination markers are drawn only for the emphasised route. For faint routes the vehicle's own
heading already conveys direction of travel, so ten destination markers would be clutter carrying
information nobody is reading. This also adopts ADR-0003's scale conclusion now, at no cost.

⚠️ **This amends `PRODUCT-SPEC.md` F2**, which stated destination identifiability unconditionally. The
criterion is narrowed to the emphasised route, on the grounds that identifying which end is the goal
matters when tracing a route and not when reading where work is concentrated. Amended rather than
reinterpreted silently.

⚠️ **Amended in implementation: the zones were enlarged, and their minimums come from measurement rather
than from area.** As first drawn, the five districts held 30 of the road network's 82 intersections — so
two thirds of available vehicles were in no zone, and coverage described a minority of the fleet rather
than the city. That is not what `PRODUCT-SPEC.md` §2.5's "subdivided into named zones" should mean.
Enlarged, they hold 60, and the gaps between them are still real, so a vehicle inside the service area and
in no zone remains reachable — the case §7.2 called the likeliest correctness bug in the design.

The minimums are then measured rather than reasoned. Intersection counts predicted availability badly,
because vehicles do not distribute evenly over a road network. Set against *observed* availability, about
a third of sampled moments have at least one district short: the layer stays quiet when the fleet is fine
and inks where it is not, which is exactly what "only inking problem zones" above was designed for. Set
from area instead, it would have inked nothing, ever, and the feature would have looked untested.

### Panel interaction and hit-testing (§2.11, §2.12)

Camera padding is used rather than manual offset arithmetic: setting the padding to the panel's width
makes every centring and fitting operation account for it automatically, so F3's "the fleet does not
appear to jump" is satisfied by the camera model rather than by bespoke compensation at each call
site.

Hit-testing queries a small pixel box rather than a point, because pixel-exact hits on a rotated icon
are frustrating to use. The tie-break is the topmost rendered feature, which produces a useful
coincidence: **because flagged vehicles are drawn above healthy ones, "topmost wins" means that in a
pile-up the operator selects the vehicle that needs attention.** One decision serving two purposes.
The alternative — nearest centre within a radius — is more predictable for heavily overlapping icons
and costs more code for no benefit at this density.

### Basemap failure (§2.13)

Everything else renders on a plain background with a non-blocking notice. Blocking would make a
cosmetic dependency fatal to a system that otherwise needs no network. ⚠️ This state must **not** be
conflated with the four ways of knowing nothing: here the data is perfectly current and only the
imagery is missing, which is the opposite situation and warrants the opposite reassurance.

## Consequences

**Positive**

- The rendering approach is the same at 100 and at 1000 vehicles, so the scale answer is "this
  already works" rather than "this would need replacing".
- Vehicle updates never touch React, which is what makes ADR-0001's framework choice safe.
- Declining interpolation removes an entire class of bug in which the map continues to animate
  state it no longer has.
- Coverage costs no visual weight when there is nothing wrong.
- The z-order rule earns its keep twice, in drawing and in selection.

**Negative / accepted costs**

- **A run-time dependency on a third-party tile host**, accepted knowingly. §2.13 limits the blast
  radius to lost context rather than a lost radar.
- **The visual encoding lives in MapLibre style expressions**, which is a syntax a reviewer may have
  to read unfamiliar. The encoding rules are therefore worth documenting near the style rather than
  left to be inferred.
- **`icon-allow-overlap` is load-bearing configuration.** Its absence is a silent correctness failure,
  not a visual preference.
- **Two route layers must stay consistent** with one source and one selection.
- **Discrete motion** reads as less polished than a smoothed alternative.
- Maximum bounds and minimum zoom are arbitrary numbers that will need tuning against the real
  service area.
- No clustering means dense areas rely on zoom, so an operator working a busy zone will zoom more
  than they otherwise would.

## What would make us revisit this

- **The tile host becoming unreliable, or a genuine offline requirement**, which promotes the PMTiles
  option — cheaper here than under most stacks because the backend already embeds and serves assets.
- **A reporting cadence low enough that discrete jumps become distracting**, which is the only real
  argument for interpolation, and would need explicit suspension on staleness as part of the change.
- **Density making individual selection impractical**, which introduces clustering — with flagged
  vehicles exempt, per §7.5.
- **Symbol-layer update cost becoming the bottleneck**, which points at feature-state updates, a
  custom layer, or deck.gl.
- **More than three coverage states**, at which point "only problem zones get ink" needs revisiting
  because the distinction between problems stops being binary.
- **A need for the operator to see recent movement**, which would reopen both interpolation and the
  product decision to keep no history.

## At ~1000 vehicles

- **The renderer does not change.** A symbol layer with a thousand features is unremarkable for
  MapLibre, and the styling expressions are identical.
- **The source update becomes the thing to watch.** Replacing a whole GeoJSON source containing a
  thousand features on every tick means serialising and parsing the entire fleet repeatedly. The
  mitigations are available and incremental — send only changed vehicles and mutate the source,
  move to feature-state updates for style-only changes, or adopt deck.gl — but this is the first
  place in the rendering path that needs work, and it is a client-side concern rather than a
  protocol one.
- **Clustering becomes necessary**, with flagged vehicles exempt so they remain individually visible.
  This is the same aggregation pressure identified independently in `PRODUCT-SPEC.md` §7.1, §7.2 and
  §7.5, arriving here for a fourth time — which is a reasonable signal that clustering with an
  attention exemption is the single most likely first extension of the whole design.
- **Declining interpolation ages well**: a hundred animated markers is a per-frame cost that a
  thousand would multiply, so the honest choice is also the cheap one.
- **Coverage rendering is unaffected**, because zone count is a property of the city rather than of
  the fleet, and "only problem zones get ink" scales with the number of problems rather than the
  number of vehicles.
- **Hit-testing gets harder** as overlap increases, and the topmost-wins tie-break becomes more
  valuable rather than less — though clustering will change what is being hit.
- **Around a hundred concurrent routes** the faint route layer becomes visually dense. Limiting
  destination markers to the emphasised route already anticipates this; the routes themselves may
  need to become a togglable layer in the same way coverage is.
