import { useCallback, useRef, useSyncExternalStore } from "react";
import type { Config, Route, Routes, Snapshot } from "./contract.generated";
import * as stream from "./stream";

/**
 * Fleet state lives here, in a module, outside React. That is not a preference: ADR-0001 chose React
 * on the explicit grounds that vehicle updates would bypass the render cycle, and this is where that
 * claim is discharged or not at all. The map will be handed the snapshot directly; React only ever
 * sees the slices it renders (ADR-0006 §6.1, §6.2).
 *
 * No previous state is retained and nothing is merged. Every snapshot is complete, so there is no
 * reconciliation to get wrong and no unexpected-vehicle case to handle (ADR-0006 §6.7, §6.8).
 */
export interface FleetState {
  config: Config | null;
  snapshot: Snapshot | null;

  /**
   * Route geometry, by id: the one piece of state the client caches, because geometry is too large and
   * too slow-changing to repeat in a snapshot. It is replaced wholesale rather than merged, since the
   * routes message is the complete set. A snapshot referencing geometry the client does not hold
   * simply draws no route — degraded, never wrong (ADR-0005 §5.5).
   */
  routes: Map<string, Route>;

  /**
   * transportFailed is what the transport itself reported, and it is a supplementary signal only. A
   * stalled connection can stay open and silent, so what will be authoritative is the watchdog
   * noticing that snapshots have stopped arriving (ADR-0005 §5.9).
   */
  transportFailed: boolean;
}

let state: FleetState = {
  config: null,
  snapshot: null,
  routes: new Map(),
  transportFailed: false,
};

const listeners = new Set<() => void>();

function publish(next: FleetState): void {
  state = next;
  for (const listener of listeners) {
    listener();
  }
}

export function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getState(): FleetState {
  return state;
}

/** connect opens the stream and feeds this store. It is called once, from outside React. */
export function connect(): () => void {
  return stream.open({
    onConfig: (config: Config) => publish({ ...state, config }),
    onRoutes: (routes: Routes) =>
      publish({ ...state, routes: new Map(routes.routes.map((route) => [route.routeId, route])) }),
    onSnapshot: (snapshot: Snapshot) => publish({ ...state, snapshot, transportFailed: false }),
    onTransportError: () => publish({ ...state, transportFailed: true }),
  });
}

/**
 * useSlice subscribes a component to one part of the state and re-renders it only when that part
 * changes by value. A snapshot arrives five times a second, so subscribing to the whole of it would
 * re-render the tree at tick rate for numbers that mostly have not moved (ADR-0006 §6.4).
 *
 * The selector has to be stable — declared at module scope, or memoised — because React subscribes
 * with it.
 */
export function useSlice<T>(
  select: (state: FleetState) => T,
  equal: (a: T, b: T) => boolean = Object.is,
): T {
  const previous = useRef<{ value: T } | null>(null);

  const read = useCallback(() => {
    const next = select(state);
    if (previous.current !== null && equal(previous.current.value, next)) {
      return previous.current.value;
    }
    previous.current = { value: next };
    return next;
  }, [select, equal]);

  return useSyncExternalStore(subscribe, read);
}

/**
 * sameValues compares two objects a field at a time. The summary is a fresh object on every snapshot
 * but its numbers rarely change, so comparing by identity would re-render it at tick rate
 * (ADR-0006 consequences).
 */
export function sameValues<T extends object>(a: T | null, b: T | null): boolean {
  if (a === null || b === null) {
    return a === b;
  }
  const keys = Object.keys(a) as (keyof T)[];
  return keys.length === Object.keys(b).length && keys.every((key) => a[key] === b[key]);
}
