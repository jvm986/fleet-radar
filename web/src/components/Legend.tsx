import {
  attentionMarks,
  coveragePatterns,
  dataUrl,
  hatchImage,
  statusIcons,
  vehicleImage,
} from "../map/icons";
import type { Layers } from "../state";
import type { FleetState } from "../store";
import { useSlice } from "../store";
import { attentionWording, coverageWording, statusWording } from "../wording";

const selectThresholds = (state: FleetState) => state.config?.thresholds ?? null;

// Drawn once, with the same code the map uses, so the legend cannot show something the map does not.
const swatches = {
  FREE: dataUrl(vehicleImage(statusIcons.FREE.colour, statusIcons.FREE.fill)),
  EN_ROUTE: dataUrl(vehicleImage(statusIcons.EN_ROUTE.colour, statusIcons.EN_ROUTE.fill)),
  WITH_CUSTOMER: dataUrl(
    vehicleImage(statusIcons.WITH_CUSTOMER.colour, statusIcons.WITH_CUSTOMER.fill),
  ),
  BELOW_MINIMUM: dataUrl(
    hatchImage(coveragePatterns.BELOW_MINIMUM.colour, coveragePatterns.BELOW_MINIMUM.spacing),
  ),
  NONE_AVAILABLE: dataUrl(
    hatchImage(coveragePatterns.NONE_AVAILABLE.colour, coveragePatterns.NONE_AVAILABLE.spacing),
  ),
};

interface Props {
  layers: Layers;
  onLayers: (layers: Layers) => void;
}

/**
 * The legend accounts for every visual distinction the map makes; anything the map encodes that this
 * does not explain is a defect (PRODUCT-SPEC F1).
 *
 * The thresholds are stated here, beside the thing they define, and they are the ones the backend sent.
 * The client holds no copy of its own, so the legend cannot state a line the system is not applying —
 * which is a correctness requirement rather than tidiness (ADR-0001 §1.8, PRODUCT-SPEC §7.6).
 */
export default function Legend({ layers, onLayers }: Props) {
  const thresholds = useSlice(selectThresholds);

  return (
    <section className="legend">
      <h2>Legend</h2>

      <ul>
        <li>
          <img src={swatches.FREE} alt="" width={18} height={18} />
          {statusWording.FREE} — hollow
        </li>
        <li>
          <img src={swatches.EN_ROUTE} alt="" width={18} height={18} />
          {statusWording.EN_ROUTE} — solid
        </li>
        <li>
          <img src={swatches.WITH_CUSTOMER} alt="" width={18} height={18} />
          {statusWording.WITH_CUSTOMER} — solid with a centre
        </li>
        <li className="note">Each marker points the way the vehicle is facing.</li>
      </ul>

      <ul>
        <li>
          <span className="ring" style={{ borderColor: attentionMarks.LOW_BATTERY.halo }}>
            {attentionMarks.LOW_BATTERY.badge}
          </span>
          {attentionWording.LOW_BATTERY}
          {thresholds === null ? "" : ` — below ${thresholds.lowBatteryPercent}%`}
        </li>
        <li>
          <span className="ring" style={{ borderColor: attentionMarks.STALE.halo }}>
            {attentionMarks.STALE.badge}
          </span>
          {attentionWording.STALE}
          {thresholds === null
            ? ""
            : ` — silent for ${thresholds.staleAfterMs / 1000}s, being ${
                thresholds.staleAfterMissedReports
              } missed reports`}
        </li>
        <li className="note">
          A vehicle that has gone quiet shows as not reporting even if its last energy reading was
          low: a reading we have stopped hearing about is not a fact.
        </li>
      </ul>

      <ul>
        <li>
          <span className="line faint" /> a planned route
        </li>
        <li>
          <span className="line emphasised" /> the selected vehicle's route, ending at its
          destination
        </li>
      </ul>

      <ul>
        <li>
          <span className="patch" style={{ backgroundImage: `url(${swatches.BELOW_MINIMUM})` }} />
          zone {coverageWording.BELOW_MINIMUM}
        </li>
        <li>
          <span className="patch" style={{ backgroundImage: `url(${swatches.NONE_AVAILABLE})` }} />
          zone with {coverageWording.NONE_AVAILABLE}
        </li>
        <li className="note">
          Only vehicles that could serve a customer now count towards a zone. A zone meeting its
          minimum is not shaded at all.
        </li>
      </ul>

      <label className="toggle">
        <input
          type="checkbox"
          checked={layers.coverage}
          onChange={(event) => onLayers({ ...layers, coverage: event.target.checked })}
        />
        Shade zones that are short
      </label>
    </section>
  );
}
