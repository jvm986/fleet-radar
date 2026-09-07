import type { Lifecycle, Summary } from "./contract.generated";
import type { FleetState } from "./store";
import { sameValues, useSlice } from "./store";

// Selectors are declared here, at module scope, so they are stable across renders.
const selectSummary = (state: FleetState): Summary | null => state.snapshot?.summary ?? null;
const selectLifecycle = (state: FleetState): Lifecycle | null => state.snapshot?.lifecycle ?? null;

/**
 * The shell. It exists to make the stream observable before there is a map to draw on: the summary
 * numbers, and which of the ways of knowing nothing applies. The map arrives next, and the operator's
 * chrome after it.
 */
export default function App() {
  const summary = useSlice(selectSummary, sameValues);
  const lifecycle = useSlice(selectLifecycle);

  if (summary === null) {
    return <p className="shell">Waiting for the first snapshot.</p>;
  }

  return (
    <div className="shell">
      <p>
        {lifecycle === "STARTING" ? "The fleet is still arriving." : "Connected."} {summary.total}{" "}
        vehicles.
      </p>
      <ul>
        <li>{summary.free} available</li>
        <li>{summary.enRoute} remotely driven</li>
        <li>{summary.withCustomer} with a customer</li>
        <li>{summary.lowBattery} low on energy</li>
        <li>{summary.stale} not reporting</li>
        <li>{summary.awaitingFirstReport} awaiting a first report</li>
        <li>{summary.availableOutsideAnyZone} available outside every zone</li>
      </ul>
    </div>
  );
}
