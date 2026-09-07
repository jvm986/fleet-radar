import type { AttentionReason, Vehicle, VehicleStatus } from "./contract.generated";

/**
 * The per-viewer view state: what the operator has narrowed to, what they have selected, and which
 * layers they want drawn. None of it is shared with other viewers and none of it reaches the backend —
 * filtering is a property of one person's screen, not of the fleet (ADR-0001 §1.7, ADR-0005 §5.13).
 */

/**
 * A filter is alternatives within a category and intersection across categories. That is the standard
 * faceted model, it needs no explaining, and it is what makes the query that actually matters
 * expressible: *available vehicles I cannot rely on* (PRODUCT-SPEC §7.6).
 *
 * An empty category is no restriction rather than "match nothing", so the natural starting state is the
 * whole fleet.
 */
export interface Filter {
  statuses: VehicleStatus[];
  attention: AttentionReason[];
}

export const wholeFleet: Filter = { statuses: [], attention: [] };

export function isNarrowed(filter: Filter): boolean {
  return filter.statuses.length > 0 || filter.attention.length > 0;
}

/**
 * matches selects on the *conditions* a vehicle has rather than on the one the map shows. A vehicle
 * that is both stale and low on energy matches either filter, which is what makes each summary figure
 * agree with the filter it applies (PRODUCT-SPEC F4, F8).
 */
export function matches(filter: Filter, vehicle: Vehicle): boolean {
  const byStatus = filter.statuses.length === 0 || filter.statuses.includes(vehicle.status);
  const byAttention =
    filter.attention.length === 0 ||
    filter.attention.some((reason) => vehicle.attentionReasons.includes(reason));
  return byStatus && byAttention;
}

export function toggle<T>(values: T[], value: T): T[] {
  return values.includes(value) ? values.filter((held) => held !== value) : [...values, value];
}

/**
 * Which layers are drawn is a display preference, so it persists. Nothing else does: anything capable
 * of making the fleet look smaller than it is must not survive a reload, because an operator who
 * inherits yesterday's filter and sees four vehicles may reasonably conclude the fleet is four
 * vehicles (PRODUCT-SPEC §7.5, ADR-0006 §6.11).
 */
export interface Layers {
  coverage: boolean;
}

export const layersKey = "fleet-radar.layers";

export function loadLayers(): Layers {
  try {
    const stored = window.localStorage.getItem(layersKey);
    if (stored === null) {
      return { coverage: true };
    }
    return { coverage: (JSON.parse(stored) as Layers).coverage !== false };
  } catch {
    // A corrupt preference is not worth failing over; the operator sees the default.
    return { coverage: true };
  }
}

export function saveLayers(layers: Layers): void {
  window.localStorage.setItem(layersKey, JSON.stringify(layers));
}

/**
 * The selected vehicle is carried in the URL, which makes it shareable — "look at LV-0042" becomes a
 * link rather than a spoken identifier, and that is the handoff need behind N4 and N6.
 *
 * Nothing else goes in it, on a principle rather than an exception: **a link that selects a vehicle
 * adds information, whereas a link that filters removes it.** A colleague must not be able to be sent
 * a view with most of the fleet silently hidden (ADR-0006 §6.3).
 */
export const selectionParameter = "vehicle";

export function selectionFromLocation(search: string): string | null {
  return new URLSearchParams(search).get(selectionParameter);
}

export function writeSelection(vehicleId: string | null): void {
  const url = new URL(window.location.href);
  if (vehicleId === null) {
    url.searchParams.delete(selectionParameter);
  } else {
    url.searchParams.set(selectionParameter, vehicleId);
  }
  window.history.replaceState(null, "", url);
}
