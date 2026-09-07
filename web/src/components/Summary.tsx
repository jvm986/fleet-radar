import { useState } from "react";
import type { AttentionReason, VehicleStatus, ZoneCoverage } from "../contract.generated";
import type { Filter } from "../state";
import type { FleetState } from "../store";
import { sameValues, useSlice } from "../store";
import { coverageWording } from "../wording";

const noCoverage: ZoneCoverage[] = [];

const selectSummary = (state: FleetState) => state.snapshot?.summary ?? null;
const selectCoverage = (state: FleetState) => state.snapshot?.coverage ?? noCoverage;

interface Props {
  filter: Filter;
  onFilter: (filter: Filter) => void;
}

/**
 * The summary is a way in, not just a readout. It answers "how many", and the operator's next question
 * is always "which ones" — which is exactly a filter, so selecting a figure applies one and no new
 * concept is needed (PRODUCT-SPEC §7.6).
 *
 * Every figure describes the whole fleet, and keeps describing it while a filter is applied. A figure
 * that tracked its own filter would be both a defect and an easy one to introduce: selecting "12 stale"
 * has to leave the figure reading 12 while the map narrows to those 12 (PRODUCT-SPEC F8).
 *
 * It is a strip along the top rather than a floating panel, because a fixed overlay costs a predictable
 * piece of the map whereas a floating one costs unknown information: which vehicles it hides changes as
 * the fleet moves (PRODUCT-SPEC §7.5).
 */
export default function Summary({ filter, onFilter }: Props) {
  const summary = useSlice(selectSummary, sameValues);
  const coverage = useSlice(selectCoverage);
  const [expanded, setExpanded] = useState(false);

  if (summary === null) {
    return null;
  }

  const byStatus = (status: VehicleStatus) => () => onFilter({ ...filter, statuses: [status] });
  const byAttention = (reason: AttentionReason) => () =>
    onFilter({ ...filter, attention: [reason] });

  return (
    <section className="summary">
      <div className="summary-row">
        <strong>{summary.total} vehicles</strong>

        <Figure count={summary.free} label="available" onSelect={byStatus("FREE")} />
        <Figure count={summary.enRoute} label="remotely driven" onSelect={byStatus("EN_ROUTE")} />
        <Figure
          count={summary.withCustomer}
          label="with a customer"
          onSelect={byStatus("WITH_CUSTOMER")}
        />
        <Figure
          count={summary.lowBattery}
          label="low on energy"
          onSelect={byAttention("LOW_BATTERY")}
          marked
        />
        <Figure
          count={summary.stale}
          label="not reporting"
          onSelect={byAttention("STALE")}
          marked
        />

        {/* The map shows every vehicle with a known position, and this accounts for the ones with none.
            A vehicle we know about is never invisible in both places at once (PRODUCT-SPEC F8). */}
        {summary.awaitingFirstReport > 0 && (
          <span className="figure-static">
            {summary.awaitingFirstReport} awaiting a first report
          </span>
        )}

        <button type="button" className="link" onClick={() => setExpanded(!expanded)}>
          {expanded ? "less" : "coverage"}
        </button>
      </div>

      {expanded && (
        <div className="summary-detail">
          {coverage.map((zone) => (
            <span key={zone.zoneId} className={zone.state === "MEETING" ? "" : "short"}>
              {zone.name}: {zone.available} of {zone.minimum} — {coverageWording[zone.state]}
            </span>
          ))}
          <span>{summary.availableOutsideAnyZone} available in no zone</span>
        </div>
      )}
    </section>
  );
}

function Figure({
  count,
  label,
  onSelect,
  marked = false,
}: {
  count: number;
  label: string;
  onSelect: () => void;
  marked?: boolean;
}) {
  return (
    <button
      type="button"
      className={marked && count > 0 ? "figure marked" : "figure"}
      onClick={onSelect}
      title={`Show only vehicles ${label}`}
    >
      {count} {label}
    </button>
  );
}
