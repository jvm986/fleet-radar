import type { FeatureCollection } from "geojson";
import type {
  ExpressionSpecification,
  GeoJSONSource,
  LayerSpecification,
  Map as MapLibreMap,
} from "maplibre-gl";
import type { Route, Vehicle, ZoneCoverage } from "../contract.generated";
import {
  attentionMarks,
  coveragePatterns,
  hatchImage,
  markerSize,
  pixelRatio,
  statusIcons,
  vehicleImage,
} from "./icons";

/**
 * The whole visual encoding lives here, in one file, because it is expressed as MapLibre style
 * expressions rather than as drawing code — a syntax a reviewer may be reading unfamiliar, and worth
 * keeping next to the rules it implements rather than leaving to be inferred (ADR-0002 consequences).
 *
 * A hundred vehicles are rows in a data source, not a hundred objects: one `setData` per tick updates
 * the whole fleet, and none of it passes through React. That is what makes React's re-render model
 * irrelevant here rather than merely tolerable (ADR-0001 §1.2, ADR-0006 §6.3).
 */

export const sources = {
  vehicles: "fleet-vehicles",
  routes: "fleet-routes",
  destinations: "fleet-destinations",
  zones: "fleet-zones",
  serviceArea: "fleet-service-area",
  hover: "fleet-hover",
} as const;

export const layers = {
  coverage: "fleet-coverage",
  zoneOutline: "fleet-zone-outline",
  zoneLabel: "fleet-zone-label",
  serviceArea: "fleet-service-area-outline",
  routesFaint: "fleet-routes-faint",
  routeEmphasised: "fleet-route-emphasised",
  destination: "fleet-destination",
  selection: "fleet-selection",
  halo: "fleet-attention-halo",
  vehicles: "fleet-vehicles",
  badge: "fleet-attention-badge",
  hover: "fleet-hover-label",
} as const;

/** labelFont is what the tile provider serves. If the glyphs fail to load the shapes still draw, which
 * is the graceful half of ADR-0002 §2.13. */
const labelFont = ["Noto Sans Bold"];

const emptyCollection = { type: "FeatureCollection" as const, features: [] };

/** haloFor and badgeFor read the dominant condition — the one the map shows. Both conditions are named
 * in the panel; the map shows only the one that governs, because staleness invalidates the energy
 * reading rather than ranking above it (PRODUCT-SPEC §7.1). */
const haloFor: ExpressionSpecification = [
  "match",
  ["get", "attention"],
  "LOW_BATTERY",
  attentionMarks.LOW_BATTERY.halo,
  "STALE",
  attentionMarks.STALE.halo,
  "transparent",
];

export function install(map: MapLibreMap): void {
  for (const { id, colour, fill } of Object.values(statusIcons)) {
    map.addImage(id, vehicleImage(colour, fill), { pixelRatio });
  }
  for (const { id, colour, spacing } of Object.values(coveragePatterns)) {
    map.addImage(id, hatchImage(colour, spacing), { pixelRatio });
  }

  for (const id of Object.values(sources)) {
    map.addSource(id, { type: "geojson", data: emptyCollection });
  }
  for (const layer of definitions()) {
    map.addLayer(layer);
  }
}

function definitions(): LayerSpecification[] {
  return [
    // Only problem zones get any ink. In a well-covered city the layer is nearly invisible and the map
    // stays clean, with shading appearing exactly where the operator needs to look (ADR-0002 §2.9).
    {
      id: layers.coverage,
      type: "fill",
      source: sources.zones,
      filter: ["in", ["get", "coverage"], ["literal", ["BELOW_MINIMUM", "NONE_AVAILABLE"]]],
      paint: {
        "fill-pattern": [
          "match",
          ["get", "coverage"],
          "NONE_AVAILABLE",
          coveragePatterns.NONE_AVAILABLE.id,
          coveragePatterns.BELOW_MINIMUM.id,
        ],
        "fill-opacity": 0.32,
      },
    },

    // Boundaries are lines rather than fills, because the coverage layer above already shades zones and
    // two fills would compound into a wash over the whole map (ADR-0002 §2.8).
    {
      id: layers.zoneOutline,
      type: "line",
      source: sources.zones,
      paint: { "line-color": "#6b7280", "line-width": 1.2, "line-dasharray": [3, 2] },
    },
    {
      id: layers.serviceArea,
      type: "line",
      source: sources.serviceArea,
      paint: { "line-color": "#374151", "line-width": 2 },
    },
    {
      // Zone names are drawn because they are the operator's language for handing a vehicle over:
      // "two cars short downtown" can be said on a radio (PRODUCT-SPEC §2.5).
      id: layers.zoneLabel,
      type: "symbol",
      source: sources.zones,
      layout: {
        "text-field": ["get", "name"],
        "text-font": labelFont,
        "text-size": 12,
        "text-letter-spacing": 0.08,
        "text-transform": "uppercase",
      },
      paint: {
        "text-color": "#374151",
        "text-halo-color": "rgba(255, 255, 255, 0.9)",
        "text-halo-width": 1.5,
      },
    },

    // Two line layers over one source rather than one layer with data-driven width, because z-order
    // cannot be controlled per feature within a layer — so the emphasised route could otherwise be
    // drawn underneath a faint one, which is the exact failure the emphasis exists to prevent
    // (ADR-0002 §2.7).
    {
      id: layers.routesFaint,
      type: "line",
      source: sources.routes,
      filter: unselected(""),
      layout: { "line-cap": "round", "line-join": "round" },
      paint: { "line-color": "#1f2937", "line-width": 2, "line-opacity": 0.45 },
    },
    {
      id: layers.routeEmphasised,
      type: "line",
      source: sources.routes,
      filter: selected(""),
      layout: { "line-cap": "round", "line-join": "round" },
      paint: { "line-color": "#b45309", "line-width": 3.5, "line-opacity": 0.95 },
    },
    {
      // A destination marker only for the emphasised route: identifying which end is the goal matters
      // when tracing one route, and ten of them would be clutter carrying information nobody is
      // reading. For the faint routes the vehicle's own heading conveys direction (ADR-0002 §2.10).
      id: layers.destination,
      type: "circle",
      source: sources.destinations,
      filter: selected(""),
      paint: {
        "circle-radius": 5,
        "circle-color": "#ffffff",
        "circle-stroke-width": 3,
        "circle-stroke-color": "#b45309",
      },
    },

    {
      // The operator has to be able to tell which vehicle is selected at all times, and selection is
      // theirs: nothing but the operator clears it — not a status change, not going stale, and not a
      // filter that would exclude it (PRODUCT-SPEC F3, §7.6).
      id: layers.selection,
      type: "circle",
      source: sources.vehicles,
      filter: ["==", ["get", "vehicleId"], ""],
      paint: {
        "circle-radius": markerSize * 0.85,
        "circle-color": "rgba(17, 24, 39, 0.06)",
        "circle-stroke-width": 2,
        "circle-stroke-color": "#111827",
      },
    },

    // The halo sits beneath the marker, so attention is additive rather than substitutive: the marker
    // keeps saying what the vehicle is doing (PRODUCT-SPEC §7.5).
    {
      id: layers.halo,
      type: "circle",
      source: sources.vehicles,
      filter: ["!=", ["get", "attention"], "NONE"],
      paint: {
        "circle-radius": markerSize * 0.7,
        "circle-color": "rgba(255, 255, 255, 0.55)",
        "circle-stroke-width": 3,
        "circle-stroke-color": haloFor,
      },
    },
    {
      id: layers.vehicles,
      type: "symbol",
      source: sources.vehicles,
      layout: {
        "icon-image": [
          "match",
          ["get", "status"],
          "FREE",
          statusIcons.FREE.id,
          "EN_ROUTE",
          statusIcons.EN_ROUTE.id,
          "WITH_CUSTOMER",
          statusIcons.WITH_CUSTOMER.id,
          statusIcons.FREE.id,
        ],
        // Heading is degrees clockwise from north, and the marker is drawn pointing north.
        "icon-rotate": ["get", "heading"],
        "icon-rotation-alignment": "map",
        // Load-bearing rather than cosmetic: MapLibre hides colliding symbols by default, so without
        // this vehicles in dense areas silently disappear — presenting as a data bug rather than as a
        // styling default (ADR-0002 §2.3).
        "icon-allow-overlap": true,
        "icon-ignore-placement": true,
        // Flagged vehicles are drawn last, so the one vehicle the operator needs to see is never
        // underneath a healthy one. It also makes the hit-test tie-break useful: topmost wins, so in a
        // pile-up the operator selects the vehicle that warrants attention (ADR-0002 §2.12).
        "symbol-sort-key": ["case", ["==", ["get", "attention"], "NONE"], 0, 1],
      },
      paint: {
        // A selected vehicle the filter excludes stays drawn, faded, because a filter narrows what the
        // operator is looking at and does not overrule what they have asked to watch
        // (PRODUCT-SPEC F4).
        "icon-opacity": ["case", ["get", "outsideFilter"], 0.4, 1],
      },
    },
    {
      id: layers.badge,
      type: "symbol",
      source: sources.vehicles,
      filter: ["!=", ["get", "attention"], "NONE"],
      layout: {
        "text-field": ["get", "badge"],
        "text-font": labelFont,
        "text-size": 12,
        "text-offset": [1.1, -1.0],
        "text-allow-overlap": true,
        "text-ignore-placement": true,
      },
      paint: {
        "text-color": "#111827",
        "text-halo-color": haloFor,
        "text-halo-width": 2.5,
      },
    },
    {
      // Labels on hover only. A hundred overlapping labels is unreadable, but naming a vehicle has to
      // be cheap, because handing one over needs its label and should not require committing to a
      // selection (PRODUCT-SPEC §7.5).
      id: layers.hover,
      type: "symbol",
      source: sources.hover,
      layout: {
        "text-field": ["get", "text"],
        "text-font": labelFont,
        "text-size": 12,
        "text-offset": [0, -1.8],
        "text-allow-overlap": true,
        "text-ignore-placement": true,
      },
      paint: {
        "text-color": "#111827",
        "text-halo-color": "rgba(255, 255, 255, 0.95)",
        "text-halo-width": 2,
      },
    },
  ];
}

function selected(routeID: string): ExpressionSpecification {
  return ["==", ["get", "routeId"], routeID];
}

function unselected(routeID: string): ExpressionSpecification {
  return ["!=", ["get", "routeId"], routeID];
}

/** focus marks the selected vehicle and moves the route emphasis to it. Selection flows from React to
 * the map and never the other way: the map is told what is selected and never decides it
 * (ADR-0006 §6.1). */
export function focus(map: MapLibreMap, vehicleID: string, routeID: string): void {
  map.setFilter(layers.selection, ["==", ["get", "vehicleId"], vehicleID]);
  map.setFilter(layers.routesFaint, unselected(routeID));
  map.setFilter(layers.routeEmphasised, selected(routeID));
  map.setFilter(layers.destination, selected(routeID));
}

/** setCoverageVisible turns the shading off. Shaded zones do compete with reading individual markers, so
 * the layer can be turned off — and that preference persists, which is safe because it cannot make the
 * fleet look smaller than it is (PRODUCT-SPEC F7). */
export function setCoverageVisible(map: MapLibreMap, visible: boolean): void {
  map.setLayoutProperty(layers.coverage, "visibility", visible ? "visible" : "none");
}

/** geometry is the checked-in service area and its zones, served in config. The client draws what it is
 * given rather than holding its own copy (ADR-0001 §1.9). */
interface GeometryFeature {
  type: "Feature";
  properties: { kind: string; name?: string; zoneId?: string };
  geometry: { type: "Polygon"; coordinates: [number, number][][] };
}

interface GeometryCollection {
  features: GeometryFeature[];
}

export function zoneFeatures(serviceArea: unknown): GeometryFeature[] {
  return (serviceArea as GeometryCollection).features.filter(
    (feature) => feature.properties.kind === "zone",
  );
}

export function serviceAreaFeatures(serviceArea: unknown): GeometryFeature[] {
  return (serviceArea as GeometryCollection).features.filter(
    (feature) => feature.properties.kind === "service-area",
  );
}

export function bounds(features: GeometryFeature[]): [[number, number], [number, number]] {
  let west = 180;
  let south = 90;
  let east = -180;
  let north = -90;

  for (const feature of features) {
    for (const ring of feature.geometry.coordinates) {
      for (const [longitude, latitude] of ring) {
        west = Math.min(west, longitude);
        east = Math.max(east, longitude);
        south = Math.min(south, latitude);
        north = Math.max(north, latitude);
      }
    }
  }
  return [
    [west, south],
    [east, north],
  ];
}

/** paintGeometry sets the boundaries once, and the coverage state every time it is published: coverage
 * is a property of the fleet, not of the geography, so it arrives with the snapshot. */
export function paintGeometry(
  map: MapLibreMap,
  zones: GeometryFeature[],
  area: GeometryFeature[],
  coverage: ZoneCoverage[],
): void {
  const states = new Map(coverage.map((zone) => [zone.zoneId, zone]));

  setData(map, sources.zones, {
    type: "FeatureCollection",
    features: zones.map((zone) => ({
      ...zone,
      properties: {
        ...zone.properties,
        coverage: states.get(zone.properties.zoneId ?? "")?.state ?? "MEETING",
      },
    })),
  });
  setData(map, sources.serviceArea, { type: "FeatureCollection", features: area });
}

/** paintFleet replaces the whole fleet, once per snapshot. Positions are drawn as reported and never
 * interpolated: a vehicle that has gone quiet would otherwise keep gliding across the map, which is
 * animating state we no longer have (ADR-0002 §2.5). */
export function paintFleet(
  map: MapLibreMap,
  vehicles: Vehicle[],
  routes: Map<string, Route>,
  excluded: (vehicle: Vehicle) => boolean,
): void {
  setData(map, sources.vehicles, {
    type: "FeatureCollection",
    features: vehicles.map((vehicle) => ({
      type: "Feature" as const,
      geometry: { type: "Point" as const, coordinates: vehicle.position },
      properties: {
        vehicleId: vehicle.vehicleId,
        label: vehicle.label,
        status: vehicle.status,
        heading: vehicle.heading,
        attention: vehicle.attention,
        badge: badgeFor(vehicle),
        routeId: vehicle.routeId,
        outsideFilter: excluded(vehicle),
      },
    })),
  });

  // A route is drawn only when it belongs to a vehicle on the map, so a route on screen always belongs
  // to a vehicle the operator can see. Geometry the client has not been sent simply draws nothing
  // (PRODUCT-SPEC F2, ADR-0005 §5.5).
  const drawn = vehicles
    .map((vehicle) => routes.get(vehicle.routeId))
    .filter((route): route is Route => route !== undefined);

  setData(map, sources.routes, {
    type: "FeatureCollection",
    features: drawn.map((route) => ({
      type: "Feature" as const,
      geometry: { type: "LineString" as const, coordinates: route.geometry },
      properties: { routeId: route.routeId },
    })),
  });
  setData(map, sources.destinations, {
    type: "FeatureCollection",
    features: drawn.map((route) => ({
      type: "Feature" as const,
      geometry: { type: "Point" as const, coordinates: route.destination },
      properties: { routeId: route.routeId },
    })),
  });
}

function badgeFor(vehicle: Vehicle): string {
  return vehicle.attention === "NONE" ? "" : attentionMarks[vehicle.attention].badge;
}

/** hoverLabel names one vehicle under the pointer, with its reason if it has one. */
export function hoverLabel(map: MapLibreMap, at: [number, number] | null, text: string): void {
  setData(map, sources.hover, {
    type: "FeatureCollection",
    features:
      at === null
        ? []
        : [
            {
              type: "Feature" as const,
              geometry: { type: "Point" as const, coordinates: at },
              properties: { text },
            },
          ],
  });
}

function setData(map: MapLibreMap, id: string, data: FeatureCollection): void {
  const source = map.getSource(id) as GeoJSONSource | undefined;
  source?.setData(data);
}
