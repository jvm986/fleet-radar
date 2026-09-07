import type { AttentionReason, VehicleStatus } from "../contract.generated";
import type { Filter } from "../state";
import { toggle } from "../state";
import type { FleetState } from "../store";
import { sameValues, useSlice } from "../store";
import { attentionWording, statusWording } from "../wording";

const selectSummary = (state: FleetState) => state.snapshot?.summary ?? null;

interface Props {
  filter: Filter;
  onFilter: (filter: Filter) => void;
}

/**
 * The summary is a way in, not just a readout. It answers "how many", and the operator's next question
 * is always "which ones" — which is exactly a filter, so a figure *is* its filter and no second control
 * is needed (PRODUCT-SPEC F8, §7.6).
 *
 * That is why the figures and the filters are one bar rather than two surfaces. Held apart, the same five
 * categories appeared twice with different behaviour — a figure replaced its category while a chip
 * toggled it — so "available then remotely driven" gave one status or two depending on where the
 * operator had clicked. One control, one behaviour: toggling, which is what makes F4's "alternatives
 * within one category" reachable here at all.
 *
 * It is exactly one row and stays one row. Nothing here expands, and nothing appears beneath: a strip
 * that changes height moves the map under the operator, and it does so at the moment they have just
 * acted, which is the worst moment for the ground to shift. The two things that used to want a second
 * row now have their own places — how much a filter is hiding is a notice, and coverage detail is a
 * panel — and both are better served there (PRODUCT-SPEC F4, F7).
 *
 * Every figure describes the whole fleet, and keeps describing it while a filter is applied. A figure
 * that tracked its own filter would be both a defect and an easy one to introduce: selecting "12 stale"
 * has to leave the figure reading 12 while the map narrows to those 12 (PRODUCT-SPEC F8).
 */
export default function Summary({ filter, onFilter }: Props) {
  const summary = useSlice(selectSummary, sameValues);

  if (summary === null) {
    return null;
  }

  // The words come from the shared vocabulary rather than being written here. The same filter must not be
  // labelled two ways depending on which surface names it (wording.ts).
  const statuses: { status: VehicleStatus; count: number }[] = [
    { status: "FREE", count: summary.free },
    { status: "EN_ROUTE", count: summary.enRoute },
    { status: "WITH_CUSTOMER", count: summary.withCustomer },
  ];
  const reasons: { reason: Exclude<AttentionReason, "NONE">; count: number }[] = [
    { reason: "LOW_BATTERY", count: summary.lowBattery },
    { reason: "STALE", count: summary.stale },
  ];

  return (
    /* Grouped rather than run together, because the grouping is the filter model: within a group the
       choices are alternatives, and across groups they intersect — which is what makes "available
       vehicles I cannot rely on" expressible (PRODUCT-SPEC F4). */
    <div className="summary">
      <strong className="figure-total">{summary.total} vehicles</strong>

      <div className="group">
        {statuses.map(({ status, count }) => (
          <Figure
            key={status}
            count={count}
            label={statusWording[status]}
            on={filter.statuses.includes(status)}
            onToggle={() => onFilter({ ...filter, statuses: toggle(filter.statuses, status) })}
          />
        ))}
      </div>

      <div className="group">
        {reasons.map(({ reason, count }) => (
          <Figure
            key={reason}
            count={count}
            label={attentionWording[reason]}
            on={filter.attention.includes(reason)}
            onToggle={() => onFilter({ ...filter, attention: toggle(filter.attention, reason) })}
            marked
          />
        ))}
      </div>

      {/* The map shows every vehicle with a known position, and this accounts for the ones with none. A
          vehicle we know about is never invisible in both places at once (PRODUCT-SPEC F8). It is not a
          figure to select: there is no filter for it, because it cannot be drawn to be narrowed to. */}
      {summary.awaitingFirstReport > 0 && (
        <div className="group">
          <span className="figure-static">
            {summary.awaitingFirstReport} awaiting a first report
          </span>
        </div>
      )}
    </div>
  );
}

function Figure({
  count,
  label,
  on,
  onToggle,
  marked = false,
}: {
  count: number;
  label: string;
  on: boolean;
  onToggle: () => void;
  marked?: boolean;
}) {
  const classes = ["figure"];
  if (on) {
    classes.push("on");
  }
  if (marked && count > 0) {
    classes.push("marked");
  }

  return (
    <button
      type="button"
      className={classes.join(" ")}
      // The figure is a toggle, so it has to report its own state: which filters are active is exactly
      // the fact F4 requires to be visible, and a class name conveys it to nobody who cannot see it.
      aria-pressed={on}
      onClick={onToggle}
      title={on ? `Stop showing only vehicles ${label}` : `Show only vehicles ${label}`}
    >
      {count} {label}
    </button>
  );
}
