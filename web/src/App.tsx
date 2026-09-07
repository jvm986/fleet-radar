import { useCallback, useEffect, useState } from "react";
import DetailPanel from "./components/DetailPanel";
import Filters from "./components/Filters";
import Legend from "./components/Legend";
import Search from "./components/Search";
import Summary from "./components/Summary";
import ViewNotice from "./components/ViewNotice";
import { useConnection } from "./connection";
import type { Vehicle } from "./contract.generated";
import FleetMap from "./map/FleetMap";
import type { Filter, Layers } from "./state";
import {
  loadLayers,
  matches,
  saveLayers,
  selectionFromLocation,
  wholeFleet,
  writeSelection,
} from "./state";
import type { FleetState } from "./store";
import { useSlice } from "./store";

/** panelWidth is what the camera is padded by while the panel is open. */
const panelWidth = 320;

const noVehicles: Vehicle[] = [];

const selectLifecycle = (state: FleetState) => state.snapshot?.lifecycle ?? null;
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

  const shown = vehicles.filter((vehicle) => matches(filter, vehicle)).length;

  return (
    <div className="app">
      <div className="stage" style={selected === null ? undefined : { right: panelWidth }}>
        <FleetMap
          selected={selected}
          filter={filter}
          showCoverage={layers.coverage}
          panelWidth={selected === null ? 0 : panelWidth}
          onSelect={select}
          onBasemapUnavailable={onBasemapUnavailable}
        />

        <Summary filter={filter} onFilter={setFilter} />

        <div className="controls">
          <Search onSelect={select} />
          <Filters filter={filter} onFilter={setFilter} shown={shown} total={vehicles.length} />
          <Legend layers={layers} onLayers={setLayers} />
        </div>

        <div className="notices">
          <ViewNotice
            current={connection.current}
            silentForMs={connection.silentForMs}
            lifecycle={lifecycle}
            narrowed={filter.statuses.length > 0 || filter.attention.length > 0}
            shown={shown}
            total={vehicles.length}
          />
          {basemapUnavailable && (
            <p className="notice">
              The street map could not be loaded. The fleet drawn here is current; only the imagery
              is missing.
            </p>
          )}
        </div>
      </div>

      {selected !== null && <DetailPanel selected={selected} onClear={() => setSelected(null)} />}
    </div>
  );
}
