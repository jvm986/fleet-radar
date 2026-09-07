import type { Lifecycle } from "../contract.generated";

interface Props {
  current: boolean;
  silentForMs: number;
  lifecycle: Lifecycle | null;
  narrowed: boolean;
  shown: number;
  total: number;
  onClearFilter: () => void;
}

/**
 * The four ways of knowing nothing, and the one way of knowing only part. Each demands a different
 * response from the operator, so each has to be separately recognisable, and none of them may render as
 * simply an empty map — treating unknown as empty is the failure that is easiest to ship by accident
 * (PRODUCT-SPEC F6, §7.6).
 *
 * The order is the order of authority. Disconnected comes first because when the view cannot be trusted,
 * nothing else it says is worth saying: a count of zero means nothing if the count itself is stale.
 * Narrowing comes last because it is the only one the operator caused and the only one they can undo.
 */
export default function ViewNotice({
  current,
  silentForMs,
  lifecycle,
  narrowed,
  shown,
  total,
  onClearFilter,
}: Props) {
  // Disconnected: nothing on screen can be trusted, and we cannot know what is true. Per-vehicle
  // staleness deliberately does not advance while this holds — blaming a hundred healthy vehicles for
  // one connection failure would be a false claim, not a conservative one (PRODUCT-SPEC §7.6).
  if (!current) {
    return (
      <p className="notice bad" role="status">
        <strong>This view is not current.</strong> Nothing has arrived for{" "}
        {(silentForMs / 1000).toFixed(0)}s. What is drawn is the last thing we were told, and the
        fleet has moved since.
      </p>
    );
  }

  // Filling: connected, but the backend has not yet learned the whole fleet.
  if (lifecycle === "STARTING") {
    return (
      <p className="notice" role="status">
        The fleet is still arriving. What is drawn is not yet all of it.
      </p>
    );
  }

  // Excluded by filter: the operator did this, and clearing the filter undoes it.
  if (narrowed && shown === 0 && total > 0) {
    return (
      <p className="notice" role="status">
        Nothing matches this filter. All {total} vehicles are hidden by it — the fleet is not empty.{" "}
        <button type="button" className="link" onClick={onClearFilter}>
          show the whole fleet
        </button>
      </p>
    );
  }

  // Empty: connected, current, and there genuinely are no vehicles.
  if (total === 0) {
    return (
      <p className="notice" role="status">
        There are no vehicles in this fleet. The view is current — this is the fleet being empty,
        not information being missing.
      </p>
    );
  }

  /**
   * Narrowed, with something still on the map. This is load-bearing rather than decorative: it is what
   * prevents a filtered map from being mistaken for the whole fleet, and F4 requires all three of its
   * parts — that a filter is active, how much it hides, and a way to clear it in one action.
   *
   * It lives here rather than in the header because it is the same kind of statement as the four above:
   * what is drawn is not the whole truth. Putting it with them means the operator has one place to look
   * for that, and it keeps the header to a single row that never changes height. "of N on the map" rather
   * than "of N" because the header counts the whole fleet including any vehicle awaiting a first report,
   * which cannot be drawn, and the two would otherwise read as a discrepancy (PRODUCT-SPEC F4, F8).
   */
  if (narrowed) {
    return (
      <p className="notice narrowed" role="status">
        <strong>
          Showing {shown} of {total} on the map
        </strong>
        {total - shown > 0 ? ` — ${total - shown} hidden by this filter` : ""}{" "}
        <button type="button" className="link" onClick={onClearFilter}>
          show the whole fleet
        </button>
      </p>
    );
  }

  return null;
}
