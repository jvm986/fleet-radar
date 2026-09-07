import {
  type IControl,
  Map as MapLibreMap,
  type MapMouseEvent,
  NavigationControl,
  type Point,
  type StyleSpecification,
} from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { useEffect, useRef } from "react";
import type { Route, Vehicle } from "../contract.generated";
import { type Filter, matches } from "../state";
import { getState, subscribe } from "../store";
import { attentionWording } from "../wording";
import * as fleet from "./layers";

/**
 * The map is imperative and lives outside React's render cycle. It subscribes to the store directly and
 * replaces its data source once per snapshot, so a hundred vehicles moving five times a second never
 * touch the component tree — which is what makes React a safe choice here rather than merely a tolerable
 * one (ADR-0001 §1.2, ADR-0006 §6.3).
 *
 * Only interaction travels the other way: the map is told what is selected and what is filtered, and
 * never decides either (ADR-0006 §6.1).
 */

/** basemapStyle is key-free hosted vector tiles: no signup and nothing for a reviewer to configure,
 * with real street context so "is that vehicle on a road" is a meaningful question (ADR-0002 §2.2). */
const basemapStyle = "https://tiles.openfreemap.org/styles/liberty";
const glyphs = "https://tiles.openfreemap.org/fonts/{fontstack}/{range}.pbf";

/** fallbackStyle keeps everything except the imagery. A cosmetic dependency must not take the radar
 * down, and this state must not be confused with any of the four ways of knowing nothing: here the data
 * is perfectly current and only the picture behind it is missing (ADR-0002 §2.13). */
const fallbackStyle: StyleSpecification = {
  version: 8,
  glyphs,
  sources: {},
  layers: [{ id: "background", type: "background", paint: { "background-color": "#eaeef2" } }],
};

/** basemapPatience is how long the tile host gets before the radar carries on without it. */
const basemapPatience = 6000;

/** Pan and zoom are free within generous bounds rather than clamped to the service area: a customer may
 * drive a vehicle anywhere, so an out-of-area vehicle has to stay reachable. The bounds and the minimum
 * zoom only stop the operator getting lost at world scale, and the reset control is the way back
 * (ADR-0002 §2.4). */
const maxBounds: [[number, number], [number, number]] = [
  [-120.5, 33.0],
  [-109.5, 39.4],
];
/** minZoom has to agree with maxBounds: at a zoom where the viewport is wider than the bounds, the
 * camera cannot satisfy the constraint, and the two settings fight each other. Eleven degrees of
 * longitude is wider than the viewport at zoom 9 on any ordinary display. */
const minZoom = 9;

/** hitBox is the pixel radius the pointer tests over. A point-exact hit on a rotated arrow is
 * frustrating to use (ADR-0002 §2.12). */
const hitBox = 5;

interface Props {
  selected: string | null;
  filter: Filter;
  showCoverage: boolean;
  /** panelWidth is fed to the camera as padding rather than compensated for at each call site, so every
   * centring and fitting operation accounts for the panel automatically (ADR-0002 §2.11). */
  panelWidth: number;
  onSelect: (vehicleId: string | null) => void;
  onBasemapUnavailable: () => void;
}

export default function FleetMap({
  selected,
  filter,
  showCoverage,
  panelWidth,
  onSelect,
  onBasemapUnavailable,
}: Props) {
  const container = useRef<HTMLDivElement | null>(null);
  const map = useRef<MapLibreMap | null>(null);
  const ready = useRef(false);
  const framed = useRef(false);

  // What the map is currently being told. Held in a ref because the paint function is created once and
  // has to see the latest without the effect being torn down and rebuilt.
  const view = useRef({ selected, filter });
  view.current = { selected, filter };

  useEffect(() => {
    if (container.current === null) {
      return;
    }

    const instance = new MapLibreMap({
      container: container.current,
      style: basemapStyle,
      center: [-115.15, 36.15],
      zoom: 10.5,
      maxBounds,
      minZoom,
    });
    map.current = instance;

    const paint = () => {
      if (!ready.current) {
        return;
      }
      const state = getState();

      if (state.config !== null) {
        const zones = fleet.zoneFeatures(state.config.serviceArea);
        const area = fleet.serviceAreaFeatures(state.config.serviceArea);
        fleet.paintGeometry(instance, zones, area, state.snapshot?.coverage ?? []);

        // On every load the operator sees the whole service area, framed once the geometry describing it
        // has arrived (PRODUCT-SPEC F1).
        if (!framed.current) {
          framed.current = true;
          instance.fitBounds(fleet.bounds(area), { padding: 48, animate: false });
        }
      }

      if (state.snapshot !== null) {
        draw(instance, state.snapshot.vehicles, state.routes, view.current);
      }
    };

    const install = () => {
      fleet.install(instance);
      ready.current = true;
      paint();
    };

    const patience = window.setTimeout(() => {
      if (!ready.current) {
        onBasemapUnavailable();
        instance.setStyle(fallbackStyle);
      }
    }, basemapPatience);

    // MapLibre reports its own trouble here — a failed tile, a missing glyph, an invalid expression.
    // None of it should be silent: the map going quiet with no explanation is the hardest failure to
    // diagnose in the whole client.
    instance.on("error", (event) => {
      console.error("map:", event.error?.message ?? event);
    });

    instance.on("style.load", () => {
      window.clearTimeout(patience);
      install();
    });

    instance.addControl(new NavigationControl({ showCompass: false }), "top-right");
    instance.addControl(
      new ResetControl(() => {
        const state = getState();
        if (state.config === null) {
          return;
        }
        instance.fitBounds(fleet.bounds(fleet.serviceAreaFeatures(state.config.serviceArea)), {
          padding: 48,
        });
      }),
      "top-right",
    );

    // Topmost feature wins, which produces a useful coincidence: because flagged vehicles are drawn
    // above healthy ones, in a pile-up the operator selects the vehicle that needs attention
    // (ADR-0002 §2.12).
    const at = (point: Point) =>
      instance.queryRenderedFeatures(
        [
          [point.x - hitBox, point.y - hitBox],
          [point.x + hitBox, point.y + hitBox],
        ],
        { layers: [fleet.layers.vehicles] },
      )[0];

    instance.on("mousemove", (event: MapMouseEvent) => {
      const found = at(event.point);
      instance.getCanvas().style.cursor = found === undefined ? "" : "pointer";
      if (found === undefined || found.geometry.type !== "Point") {
        fleet.hoverLabel(instance, null, "");
        return;
      }

      const reason = attentionWording[found.properties.attention as Vehicle["attention"]];
      fleet.hoverLabel(
        instance,
        found.geometry.coordinates as [number, number],
        reason === "" ? String(found.properties.label) : `${found.properties.label} · ${reason}`,
      );
    });

    instance.on("click", (event: MapMouseEvent) => {
      const found = at(event.point);
      onSelect(found === undefined ? null : String(found.properties.vehicleId));
    });

    const unsubscribe = subscribe(paint);
    return () => {
      window.clearTimeout(patience);
      unsubscribe();
      instance.remove();
      map.current = null;
      ready.current = false;
      framed.current = false;
    };
  }, [onSelect, onBasemapUnavailable]);

  // Interaction reaches the map here, and only here.
  useEffect(() => {
    const instance = map.current;
    if (instance === null || !ready.current) {
      return;
    }
    const state = getState();
    draw(instance, state.snapshot?.vehicles ?? [], state.routes, { selected, filter });
  }, [selected, filter]);

  useEffect(() => {
    const instance = map.current;
    if (instance !== null && ready.current) {
      fleet.setCoverageVisible(instance, showCoverage);
    }
  }, [showCoverage]);

  // The panel displaces the map, so the map resizes — which can carry the just-selected vehicle under the
  // panel edge or off screen. Keeping it in view is an acceptance criterion rather than an
  // implementation detail (PRODUCT-SPEC F3, ADR-0002 §2.11).
  useEffect(() => {
    const instance = map.current;
    if (instance === null || !ready.current) {
      return;
    }

    const padding = { top: 0, bottom: 0, left: 0, right: panelWidth };
    const vehicle = getState().snapshot?.vehicles.find(
      (candidate) => candidate.vehicleId === selected,
    );
    if (vehicle === undefined) {
      instance.easeTo({ padding, duration: 200 });
      return;
    }
    instance.easeTo({ center: vehicle.position, padding, duration: 300 });
  }, [panelWidth, selected]);

  return <div className="map" ref={container} />;
}

/**
 * draw hands the fleet to the map. Vehicles the filter excludes are hidden rather than de-emphasised —
 * except the selected one, which stays drawn and marked, because a filter narrows what the operator is
 * looking at and does not overrule what they have asked to watch (PRODUCT-SPEC F4).
 */
function draw(
  map: MapLibreMap,
  vehicles: Vehicle[],
  routes: Map<string, Route>,
  view: { selected: string | null; filter: Filter },
): void {
  const excluded = (vehicle: Vehicle) => !matches(view.filter, vehicle);
  const drawn = vehicles.filter(
    (vehicle) => !excluded(vehicle) || vehicle.vehicleId === view.selected,
  );

  fleet.paintFleet(map, drawn, routes, excluded);

  // Emphasis is drawn per route while selection is per vehicle, so the selected vehicle's route has to be
  // resolved here. A selected vehicle with no route emphasises nothing, which is right: a FREE or
  // WITH_CUSTOMER vehicle has no route to show (PRODUCT-SPEC F2).
  const chosen = vehicles.find((candidate) => candidate.vehicleId === view.selected);
  fleet.focus(map, view.selected ?? "", chosen?.routeId ?? "");
}

/** ResetControl is the way back from having panned away, which the generous bounds make possible
 * (ADR-0002 §2.4). */
class ResetControl implements IControl {
  private readonly reset: () => void;

  constructor(reset: () => void) {
    this.reset = reset;
  }

  onAdd(): HTMLElement {
    const group = document.createElement("div");
    group.className = "maplibregl-ctrl maplibregl-ctrl-group";

    const button = document.createElement("button");
    button.type = "button";
    button.title = "Frame the service area";
    button.textContent = "⤢";
    button.addEventListener("click", this.reset);

    group.appendChild(button);
    return group;
  }

  onRemove(): void {}
}
