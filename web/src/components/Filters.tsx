import type { AttentionReason, VehicleStatus } from "../contract.generated";
import type { Filter } from "../state";
import { isNarrowed, toggle, wholeFleet } from "../state";
import { attentionWording, statusWording } from "../wording";

const statuses: VehicleStatus[] = ["FREE", "EN_ROUTE", "WITH_CUSTOMER"];
const reasons: AttentionReason[] = ["LOW_BATTERY", "STALE"];

interface Props {
  filter: Filter;
  onFilter: (filter: Filter) => void;
  shown: number;
  total: number;
}

/**
 * Vehicles that do not match are hidden rather than de-emphasised, so the indicator below is
 * load-bearing rather than decorative: it is what prevents a filtered map from being mistaken for the
 * whole fleet. It says how much is hidden and clears in one action (PRODUCT-SPEC F4, §7.5).
 */
export default function Filters({ filter, onFilter, shown, total }: Props) {
  const narrowed = isNarrowed(filter);
  const hidden = total - shown;

  return (
    <section className="filters">
      <div className="filter-group">
        {statuses.map((status) => (
          <button
            type="button"
            key={status}
            className={filter.statuses.includes(status) ? "chip on" : "chip"}
            onClick={() => onFilter({ ...filter, statuses: toggle(filter.statuses, status) })}
          >
            {statusWording[status]}
          </button>
        ))}
      </div>

      <div className="filter-group">
        {reasons.map((reason) => (
          <button
            type="button"
            key={reason}
            className={filter.attention.includes(reason) ? "chip on" : "chip"}
            onClick={() => onFilter({ ...filter, attention: toggle(filter.attention, reason) })}
          >
            {attentionWording[reason]}
          </button>
        ))}
      </div>

      {narrowed && (
        <p className="narrowed">
          {/* "of N on the map" rather than "of N": the summary counts the whole fleet including any
              vehicle awaiting a first report, which cannot be drawn, and the two figures would otherwise
              read as a discrepancy (PRODUCT-SPEC F8). */}
          <strong>
            Showing {shown} of {total} on the map
          </strong>
          {hidden > 0 ? ` — ${hidden} hidden` : ""}{" "}
          <button type="button" className="link" onClick={() => onFilter(wholeFleet)}>
            show the whole fleet
          </button>
        </p>
      )}
    </section>
  );
}
