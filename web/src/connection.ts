import { useEffect, useState } from "react";
import { getState } from "./store";

/**
 * Connection health and vehicle health are separate concerns, and conflating them produces a false
 * claim: if the client kept evaluating staleness while disconnected, every vehicle would go stale
 * within two reporting intervals and the map would blame a hundred healthy vehicles for one failed
 * socket (PRODUCT-SPEC §7.6).
 *
 * It needs no code to avoid that, which is the payoff from deriving staleness in the backend: a
 * disconnected client simply receives nothing further, so the flags it holds freeze exactly as they
 * were. All that is left is to say so, at the level of the whole view (ADR-0005 §5.10, ADR-0006 §6.10).
 */

/**
 * missedTicks is how many publications may be missed before the view is called untrustworthy. The
 * backend publishes unconditionally, so silence is unambiguous — three ticks is enough to absorb
 * ordinary jitter without leaving the operator looking at a frozen map that appears live
 * (ADR-0005 §5.2, §5.9).
 */
export const missedTicks = 3;

export interface Connection {
  /** current is false when the view can no longer be trusted at all. */
  current: boolean;
  /** silentForMs is how long since the last snapshot, which is what the operator is told. */
  silentForMs: number;
}

/**
 * assess is the whole rule, as a pure function of the clock, so the disconnected state has a test
 * rather than a demonstration (ADR-0009 §9.9, §9.12).
 */
export function assess(
  lastSnapshotAt: number | null,
  now: number,
  tickIntervalMs: number,
): Connection {
  if (lastSnapshotAt === null) {
    // Nothing has arrived yet. That is the connecting case, not the disconnected one: the operator is
    // told the view is still opening, and the four ways of knowing nothing keep it distinct.
    return { current: true, silentForMs: 0 };
  }

  const silentForMs = Math.max(now - lastSnapshotAt, 0);
  return { current: silentForMs <= tickIntervalMs * missedTicks, silentForMs };
}

/** watchInterval is how often the view re-checks. Well inside the tick, so the transition to
 * untrustworthy is prompt without polling being the client's main activity. */
const watchInterval = 250;

export function useConnection(): Connection {
  const [connection, setConnection] = useState<Connection>({ current: true, silentForMs: 0 });

  useEffect(() => {
    const check = () => {
      const state = getState();
      setConnection(
        assess(state.lastSnapshotAt, Date.now(), state.config?.tickIntervalMs ?? watchInterval),
      );
    };

    check();
    const timer = window.setInterval(check, watchInterval);
    return () => window.clearInterval(timer);
  }, []);

  return connection;
}
