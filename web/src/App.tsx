import { useCallback, useEffect, useState } from "react";
import DetailPanel from "./components/DetailPanel";
import MapControls from "./components/MapControls";
import Search from "./components/Search";
import Summary from "./components/Summary";
import ViewNotice from "./components/ViewNotice";
import { useConnection } from "./connection";
import type { Vehicle } from "./contract.generated";
import FleetMap from "./map/FleetMap";
import type { Filter, Layers } from "./state";
import {
  isNarrowed,
  loadLayers,
  matches,
  saveLayers,
  selectionFromLocation,
  wholeFleet,
  writeSelection,
} from "./state";
import type { FleetState } from "./store";
import { useSlice } from "./store";

/** panelWidth is how much of the map's right edge the detail panel covers. The camera uses it to decide
 * whether the selected vehicle is hidden behind the panel, not to compensate for it as a matter of
 * course. */
const panelWidth = 320;

const noVehicles: Vehicle[] = [];

const selectLifecycle = (state: FleetState) => state.snapshot?.lifecycle ?? null;
const selectTransportFailed = (state: FleetState) => state.transportFailed;
const selectVehicles = (state: FleetState) => state.snapshot?.vehicles ?? noVehicles;

/**
 * The map has the whole window. The summary is a strip along the top, the operator's controls sit on the
 * left, and the detail panel displaces the map from the right rather than covering it — a fixed panel
 * costs known screen space, whereas a floating overlay costs unknown information, because which vehicles
 * it hides changes as the fleet moves (PRODUCT-SPEC §7.5).
 */
export default function App() {
  const lifecycle = useSlice(selectLifecycle);
  const vehicles = useSlice(selectVehicles);
  const connection = useConnection();
  // The transport reporting an error is a supplementary signal: it arrives sooner than the watchdog can
  // notice silence, but a stalled connection can stay open and quiet, so the watchdog stays
  // authoritative (ADR-0005 §5.9).
  const transportFailed = useSlice(selectTransportFailed);

  // The selection is read from the URL on load, which is how a shared link arrives. Filters are not, and
  // never will be: a link that selects a vehicle adds information, whereas a link that filters removes
  // it (ADR-0006 §6.3).
  const [selected, setSelected] = useState<string | null>(() =>
    selectionFromLocation(window.location.search),
  );

  // Every load begins with the whole fleet visible, so the operator can never inherit a hidden fleet
  // from a previous session (PRODUCT-SPEC F4).
  const [filter, setFilter] = useState<Filter>(wholeFleet);

  // Layer visibility is a display preference and does persist, which is safe because it cannot make the
  // fleet look smaller than it is (PRODUCT-SPEC §7.5).
  const [layers, setLayers] = useState<Layers>(loadLayers);
  const [basemapUnavailable, setBasemapUnavailable] = useState(false);

  useEffect(() => writeSelection(selected), [selected]);
  useEffect(() => saveLayers(layers), [layers]);

  const onBasemapUnavailable = useCallback(() => setBasemapUnavailable(true), []);
  const select = useCallback((vehicleId: string | null) => setSelected(vehicleId), []);
  const clearFilter = useCallback(() => setFilter(wholeFleet), []);

  const shown = vehicles.filter((vehicle) => matches(filter, vehicle)).length;

  /**
   * While the view is not current, the whole view is frozen: the controls go inert, and so does the map,
   * so it cannot be panned or zoomed either. Narrowing a fleet we are no longer hearing about, or panning
   * to look somewhere the fleet may have left, produces an answer about the past dressed as an answer
   * about now — and the operator would have no way to tell. The fleet is drawn in grey for the same
   * reason: every marker on it is a last known position, not a position (PRODUCT-SPEC F6, §7.6).
   *
   * The detail panel is deliberately left alive. It labels every value as last known, so reading it while
   * disconnected is honest rather than misleading, and being unable to dismiss a panel reads as a hung
   * application rather than a careful one.
   */
  const current = connection.current && !transportFailed;

  return (
    <div className={current ? "app" : "app stale"}>
      <div className="stage">
        <FleetMap
          selected={selected}
          filter={filter}
          showCoverage={layers.coverage}
          panelWidth={selected === null ? 0 : panelWidth}
          interactive={current}
          onSelect={select}
          onBasemapUnavailable={onBasemapUnavailable}
        />

        {/* The overlay is a column rather than independently positioned boxes, so everything below the
            header follows the header's real height rather than a guessed offset. */}
        <div className="overlay">
          {/* One strip along the top carries everything addressing the whole fleet: the figures, the
              filters they apply, and finding a vehicle by name. The figures and the filters are one
              control because a figure answers "how many" and then applies the filter that answers "which
              ones", so a separate filter bar would be the same five categories a second time
              (PRODUCT-SPEC F8, §7.6). Search is here because it is fleet-wide too, and because it is the
              inbound half of handoff — the operator is told a label and has to find it. */}
          <div className="header" inert={!current}>
            <Summary filter={filter} onFilter={setFilter} />
            <Search onSelect={select} />
          </div>

          {/* Everything below the header shares one row, so both sides clear the header without either
              having to guess an offset. */}
          <div className="below">
            {/* What sits over the map is what describes the map: what is drawn, where coverage is thin,
                and how to read it. All behind icons, so they cost a corner rather than a column. */}
            <MapControls layers={layers} onLayers={setLayers} disabled={!current} />

            {selected !== null && (
              <DetailPanel selected={selected} onClear={() => setSelected(null)} />
            )}
          </div>
        </div>

        {/* At the top, under the header, because the first of these says nothing on screen can be trusted
            — and a warning that the whole view is wrong belongs where the view is read, not in a corner
            below it. Floating rather than in the overlay column, so a notice appearing never reflows the
            header or the controls. */}
        <div className="notices">
          <ViewNotice
            current={current}
            silentForMs={connection.silentForMs}
            lifecycle={lifecycle}
            narrowed={isNarrowed(filter)}
            shown={shown}
            total={vehicles.length}
            onClearFilter={clearFilter}
          />
          {basemapUnavailable && (
            <p className="notice">
              The street map could not be loaded. The fleet drawn here is current; only the imagery
              is missing.
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
