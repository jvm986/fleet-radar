import { useState } from "react";
import type { FleetState } from "../store";
import { useSlice } from "../store";

// The snapshot is one stable reference per tick, so it is what is subscribed to; the labels are derived
// during the render. A selector that built a new array on every read could never be memoised.
const selectSnapshot = (state: FleetState) => state.snapshot;

const shown = 6;

interface Props {
  onSelect: (vehicleId: string) => void;
}

/**
 * Handoff runs both ways. The panel covers the outbound half — the operator naming a vehicle to someone
 * else — and this is the inbound half: somebody radios "check LV-0042", which would otherwise mean
 * scanning a hundred markers by eye, because filters narrow by category and never by identity.
 *
 * This is a capability the brief does not ask for, and it is here on that specific justification rather
 * than because search is generally useful (PRODUCT-SPEC §7.6).
 */
export default function Search({ onSelect }: Props) {
  const snapshot = useSlice(selectSnapshot);
  const [typed, setTyped] = useState("");

  const labels =
    snapshot === null
      ? []
      : [
          ...snapshot.vehicles.map((vehicle) => ({ id: vehicle.vehicleId, label: vehicle.label })),
          // A vehicle awaiting its first report is findable too. It cannot be drawn, but somebody
          // radioing about it is exactly the case this exists for (PRODUCT-SPEC F6).
          ...snapshot.awaitingFirstReport.map((vehicle) => ({
            id: vehicle.vehicleId,
            label: vehicle.label,
          })),
        ];

  const trimmed = typed.trim().toLowerCase();
  const found =
    trimmed === ""
      ? []
      : labels.filter((vehicle) => vehicle.label.toLowerCase().includes(trimmed)).slice(0, shown);

  return (
    <section className="search">
      <input
        type="search"
        value={typed}
        placeholder="Find a vehicle by label"
        onChange={(event) => setTyped(event.target.value)}
      />
      {trimmed !== "" && (
        <ul>
          {found.length === 0 ? (
            <li className="note">No vehicle is labelled like that.</li>
          ) : (
            found.map((vehicle) => (
              <li key={vehicle.id}>
                <button
                  type="button"
                  className="link"
                  onClick={() => {
                    onSelect(vehicle.id);
                    setTyped("");
                  }}
                >
                  {vehicle.label}
                </button>
              </li>
            ))
          )}
        </ul>
      )}
    </section>
  );
}
