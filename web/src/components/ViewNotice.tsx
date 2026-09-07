import type { Lifecycle } from "../contract.generated";

interface Props {
  current: boolean;
  silentForMs: number;
  lifecycle: Lifecycle | null;
  narrowed: boolean;
  shown: number;
  total: number;
}

/**
 * The four ways of knowing nothing. Each demands a different response from the operator, so each has to
 * be separately recognisable, and none of them may render as simply an empty map — treating unknown as
 * empty is the failure that is easiest to ship by accident (PRODUCT-SPEC F6, §7.6).
 *
 * The order is the order of authority. Disconnected comes first because when the view cannot be trusted,
 * nothing else it says is worth saying: a count of zero means nothing if the count itself is stale.
 */
export default function ViewNotice({
  current,
  silentForMs,
  lifecycle,
  narrowed,
  shown,
  total,
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
        Nothing matches this filter. All {total} vehicles are hidden by it — the fleet is not empty.
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

  return null;
}
