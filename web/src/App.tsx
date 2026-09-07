import { useCallback, useState } from "react";
import type { Lifecycle, Summary } from "./contract.generated";
import FleetMap from "./map/FleetMap";
import type { FleetState } from "./store";
import { sameValues, useSlice } from "./store";

// Selectors are declared at module scope so they are stable across renders.
const selectSummary = (state: FleetState): Summary | null => state.snapshot?.summary ?? null;
const selectLifecycle = (state: FleetState): Lifecycle | null => state.snapshot?.lifecycle ?? null;

/**
 * The map has the whole window. Nothing else holds a permanent claim on screen space: the summary is a
 * collapsible strip along the top, and the detail panel displaces the map rather than covering it —
 * a fixed panel costs known screen space, whereas a floating overlay costs unknown information,
 * because which vehicles it hides changes as the fleet moves (PRODUCT-SPEC §7.5).
 *
 * The summary and the panel arrive with the rest of the operator's chrome. What is here is the map,
 * selection, and enough of a readout to see that the fleet is being received.
 */
export default function App() {
  const summary = useSlice(selectSummary, sameValues);
  const lifecycle = useSlice(selectLifecycle);
  const [selected, setSelected] = useState<string | null>(null);
  const [basemapUnavailable, setBasemapUnavailable] = useState(false);

  const onBasemapUnavailable = useCallback(() => setBasemapUnavailable(true), []);

  return (
    <div className="app">
      <FleetMap
        selected={selected}
        onSelect={setSelected}
        onBasemapUnavailable={onBasemapUnavailable}
      />

      <div className="readout">
        {summary === null ? (
          <p>Waiting for the first snapshot.</p>
        ) : (
          <p>
            {lifecycle === "STARTING" ? "The fleet is still arriving. " : ""}
            {summary.total} vehicles · {summary.free} available · {summary.enRoute} remotely driven
            · {summary.withCustomer} with a customer · {summary.lowBattery} low on energy ·{" "}
            {summary.stale} not reporting · {summary.awaitingFirstReport} awaiting a first report
          </p>
        )}
      </div>

      {basemapUnavailable && (
        <p className="notice">
          The street map could not be loaded. The fleet below is current; only the imagery is
          missing.
        </p>
      )}
    </div>
  );
}
