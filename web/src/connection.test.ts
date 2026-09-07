import { describe, expect, test } from "vitest";
import { assess, missedTicks } from "./connection";

const tick = 200;

/**
 * The disconnected state is one of the four ways of knowing nothing, and the only one that depends on the
 * passage of time — so it takes a clock and gets a test rather than a demonstration
 * (ADR-0009 §9.9, §9.12).
 */
describe("the view's own freshness", () => {
  test("nothing having arrived yet is not the same as having lost contact", () => {
    expect(assess(null, 10_000, tick)).toEqual({ current: true, silentForMs: 0 });
  });

  test("a snapshot within the last few ticks is current", () => {
    const at = 10_000;

    expect(assess(at, at, tick).current).toBe(true);
    expect(assess(at, at + tick * missedTicks, tick).current).toBe(true);
  });

  // The backend publishes unconditionally, whether or not anything changed, which is what makes silence
  // unambiguous: there is no "nothing is happening" to confuse with "the connection is dead"
  // (ADR-0005 §5.2, §5.7).
  test("silence beyond that means the view cannot be trusted", () => {
    const at = 10_000;
    const assessment = assess(at, at + tick * missedTicks + 1, tick);

    expect(assessment.current).toBe(false);
    expect(assessment.silentForMs).toBe(tick * missedTicks + 1);
  });

  test("a clock that steps backwards does not report negative silence", () => {
    expect(assess(10_000, 9_000, tick).silentForMs).toBe(0);
  });
});
