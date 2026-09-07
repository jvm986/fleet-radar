import { useCallback } from "react";
import type { AwaitingReport, Vehicle } from "../contract.generated";
import type { FleetState } from "../store";
import { useSlice } from "../store";
import { attentionWording, statusWording } from "../wording";

interface Props {
  selected: string;
  onClear: () => void;
}

/**
 * Detail arrives in a panel that floats over the map, only as tall as what it has to say. See the note on
 * `.panel` for why it covers rather than displaces, which reverses PRODUCT-SPEC §7.5, and what the camera
 * does instead to keep the selected vehicle out from behind it.
 *
 * A selection is the operator's, and only the operator clears it: not a status change, not going stale,
 * and not a filter that would exclude it. The alternative is more internally consistent and was
 * declined, because the panel would vanish without the operator necessarily connecting it to the filter
 * they had just applied (PRODUCT-SPEC §7.6).
 *
 * The selected vehicle stays in the URL, so a vehicle is still shareable by copying the address — that is
 * ADR-0006 §6.3's whole reason for putting it there. There is no button for it here: the address bar
 * already is one (PRODUCT-SPEC N4, N6).
 */
export default function DetailPanel({ selected, onClear }: Props) {
  const vehicle = useSlice(useCallback((state: FleetState) => find(state, selected), [selected]));

  return (
    <aside className="panel">
      <header>
        <h2>{vehicle === null ? "Not in this fleet" : vehicle.label}</h2>
        <button type="button" className="link" onClick={onClear}>
          close
        </button>
      </header>

      {vehicle === null ? (
        <p>
          Nothing in the fleet has this identifier. The link may be for a different service area, or
          the vehicle may never have been registered.
        </p>
      ) : "position" in vehicle ? (
        <Reporting vehicle={vehicle} />
      ) : (
        <Awaiting />
      )}
    </aside>
  );
}

function Reporting({ vehicle }: { vehicle: Vehicle }) {
  return (
    <dl>
      <dt>Doing</dt>
      <dd>{statusWording[vehicle.status]}</dd>

      <dt>Energy</dt>
      <dd>{vehicle.batteryPercent.toFixed(0)}%</dd>

      {/* How current the information is is shown whether it is good or bad, because everything above is
          a last known value rather than a fact (PRODUCT-SPEC F3, §6.2.7). */}
      <dt>Last reported</dt>
      <dd>{describeSilence(vehicle.silentForMs)}</dd>

      <dt>Position</dt>
      <dd>
        {vehicle.position[1].toFixed(4)}, {vehicle.position[0].toFixed(4)} · facing{" "}
        {vehicle.heading.toFixed(0)}°
      </dd>

      <dt>Zone</dt>
      <dd>{vehicle.zoneId === "NO_ZONE" ? "outside every zone" : vehicle.zoneId}</dd>

      {/* No route row. It restated what the map and the status already say: an emphasised line ending at a
          destination marker, or a vehicle whose status is reason enough for there being no line
          (PRODUCT-SPEC F2). */}

      <dt>Attention</dt>
      <dd>
        {vehicle.attentionReasons.length === 0
          ? "none"
          : /* Both conditions are named here even when the map shows only the dominant one
               (PRODUCT-SPEC F3). */
            vehicle.attentionReasons.map((reason) => attentionWording[reason]).join(" and ")}
      </dd>
    </dl>
  );
}

function Awaiting() {
  return (
    <p>
      Registered, but it has never reported. This is not the same as having gone quiet: there is no
      last known position to show, so it cannot be drawn on the map.
    </p>
  );
}

function find(state: FleetState, selected: string): Vehicle | AwaitingReport | null {
  const snapshot = state.snapshot;
  if (snapshot === null) {
    return null;
  }
  return (
    snapshot.vehicles.find((vehicle) => vehicle.vehicleId === selected) ??
    snapshot.awaitingFirstReport.find((vehicle) => vehicle.vehicleId === selected) ??
    null
  );
}

function describeSilence(silentForMs: number): string {
  const seconds = silentForMs / 1000;
  return seconds < 1 ? "less than a second ago" : `${seconds.toFixed(1)}s ago`;
}
