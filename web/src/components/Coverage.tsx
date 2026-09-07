import type { ZoneCoverage } from "../contract.generated";
import type { Layers } from "../state";
import type { FleetState } from "../store";
import { useSlice } from "../store";
import { coverageWording } from "../wording";

const noCoverage: ZoneCoverage[] = [];

const selectCoverage = (state: FleetState) => state.snapshot?.coverage ?? noCoverage;
const selectOutsideAnyZone = (state: FleetState) =>
  state.snapshot?.summary.availableOutsideAnyZone ?? 0;

interface Props {
  layers: Layers;
  onLayers: (layers: Layers) => void;
}

/**
 * Where coverage is thin, zone by zone, with the switch that draws it on the map directly beneath.
 *
 * The numbers and the switch belong together: the switch shades exactly the zones this list names as
 * short, so separating them would put a control in one place and the only explanation of what it does in
 * another. Shading is also the one preference that persists, and it is safe to persist precisely because
 * it cannot make the fleet look smaller than it is — turning it off hides an annotation, never a vehicle
 * (PRODUCT-SPEC F7, §7.5, ADR-0006 §6.11).
 *
 * Only vehicles that could serve a customer now count towards a zone, and the figure for those in no zone
 * completes the arithmetic: it plus every zone's available equals the number free. Without it, available
 * vehicles in no zone would be missing from every total the operator could cross-check
 * (PRODUCT-SPEC F7, §7.2).
 */
export default function Coverage({ layers, onLayers }: Props) {
  const coverage = useSlice(selectCoverage);
  const outsideAnyZone = useSlice(selectOutsideAnyZone);

  return (
    <div className="coverage">
      {coverage.length === 0 ? (
        <p className="note">No zones have been reported yet.</p>
      ) : (
        <ul>
          {coverage.map((zone) => (
            <li key={zone.zoneId} className={zone.state === "MEETING" ? "" : "short"}>
              <strong>{zone.name}</strong> — {zone.available} of {zone.minimum},{" "}
              {coverageWording[zone.state]}
            </li>
          ))}
          <li className="note">{outsideAnyZone} available in no zone</li>
        </ul>
      )}

      <label className="toggle">
        <input
          type="checkbox"
          checked={layers.coverage}
          onChange={(event) => onLayers({ ...layers, coverage: event.target.checked })}
        />
        Shade zones that are short
      </label>
    </div>
  );
}
