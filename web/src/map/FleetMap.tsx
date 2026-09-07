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
import type { Vehicle } from "../contract.generated";
import { getState, subscribe } from "../store";
import { attentionWording } from "../wording";
import * as fleet from "./layers";

/**
 * The map is imperative and lives outside React's render cycle. It subscribes to the store directly
 * and replaces its data source once per snapshot, so a hundred vehicles moving five times a second
 * never touch the component tree — which is what makes React a safe choice here rather than merely a
 * tolerable one (ADR-0001 §1.2, ADR-0006 §6.3).
 *
 * Only interaction travels the other way: the map is told which vehicle is selected, and never decides
 * it (ADR-0006 §6.1).
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
  onSelect: (vehicleId: string | null) => void;
  onBasemapUnavailable: () => void;
}

export default function FleetMap({ selected, onSelect, onBasemapUnavailable }: Props) {
  const container = useRef<HTMLDivElement | null>(null);
  const map = useRef<MapLibreMap | null>(null);
  const ready = useRef(false);
  const framed = useRef(false);
  const selectedRef = useRef(selected);
  selectedRef.current = selected;

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

        // On every load the operator sees the whole service area, framed once the geometry describing
        // it has arrived (PRODUCT-SPEC F1).
        if (!framed.current) {
          framed.current = true;
          instance.fitBounds(fleet.bounds(area), { padding: 48, animate: false });
        }
      }
      if (state.snapshot !== null) {
        fleet.paintFleet(instance, state.snapshot, state.routes);
        applySelection(instance, state.snapshot.vehicles, selectedRef.current);
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

  useEffect(() => {
    const instance = map.current;
    if (instance === null || !ready.current) {
      return;
    }
    applySelection(instance, getState().snapshot?.vehicles ?? [], selected);
  }, [selected]);

  return <div className="map" ref={container} />;
}

/** applySelection resolves the selected vehicle's route, because emphasis is drawn per route while
 * selection is per vehicle. A selected vehicle with no route emphasises nothing, which is right: a
 * FREE or WITH_CUSTOMER vehicle has no route to show (PRODUCT-SPEC F2). */
function applySelection(map: MapLibreMap, vehicles: Vehicle[], selected: string | null): void {
  const vehicle = vehicles.find((candidate) => candidate.vehicleId === selected);
  fleet.focus(map, selected ?? "", vehicle?.routeId ?? "");
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
