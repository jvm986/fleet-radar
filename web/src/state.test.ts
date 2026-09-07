import { beforeEach, describe, expect, test } from "vitest";
import type { Vehicle } from "./contract.generated";
import { layersKey, loadLayers, matches, saveLayers, selectionFromLocation, toggle } from "./state";

function vehicle(overrides: Partial<Vehicle> = {}): Vehicle {
  return {
    vehicleId: "v1",
    label: "LV-0001",
    position: [-115.17, 36.11],
    heading: 90,
    status: "FREE",
    batteryPercent: 76,
    silentForMs: 400,
    attention: "NONE",
    attentionReasons: [],
    routeId: "",
    zoneId: "strip",
    ...overrides,
  };
}

describe("filters", () => {
  test("the whole fleet matches when nothing is chosen", () => {
    expect(matches({ statuses: [], attention: [] }, vehicle())).toBe(true);
  });

  test("choices within a category are alternatives", () => {
    const filter = { statuses: ["FREE", "EN_ROUTE"] as const, attention: [] };
    expect(
      matches({ ...filter, statuses: [...filter.statuses] }, vehicle({ status: "EN_ROUTE" })),
    ).toBe(true);
    expect(
      matches({ ...filter, statuses: [...filter.statuses] }, vehicle({ status: "WITH_CUSTOMER" })),
    ).toBe(false);
  });

  // The query that actually matters, and the reason for choosing a faceted model at all: *available
  // vehicles I cannot rely on* (PRODUCT-SPEC §7.6).
  test("categories intersect, so available-and-unreliable is expressible", () => {
    const filter = { statuses: ["FREE" as const], attention: ["STALE" as const] };

    expect(
      matches(filter, vehicle({ status: "FREE", attention: "STALE", attentionReasons: ["STALE"] })),
    ).toBe(true);
    expect(matches(filter, vehicle({ status: "FREE" }))).toBe(false);
    expect(
      matches(
        filter,
        vehicle({ status: "EN_ROUTE", attention: "STALE", attentionReasons: ["STALE"] }),
      ),
    ).toBe(false);
  });

  // A vehicle that is both is counted in both figures, so it has to match both filters, or the summary
  // and the map would disagree about what "12 stale" means (PRODUCT-SPEC F4, F8).
  test("a vehicle with two conditions matches either", () => {
    const both = vehicle({ attention: "STALE", attentionReasons: ["STALE", "LOW_BATTERY"] });

    expect(matches({ statuses: [], attention: ["STALE"] }, both)).toBe(true);
    expect(matches({ statuses: [], attention: ["LOW_BATTERY"] }, both)).toBe(true);
  });

  // The empty-by-filter case, which is one of the four ways of knowing nothing and must never render as
  // an empty map (PRODUCT-SPEC F6).
  test("a combination can exclude everything", () => {
    const fleet = [vehicle({ status: "FREE" }), vehicle({ status: "EN_ROUTE" })];
    const filter = { statuses: ["WITH_CUSTOMER" as const], attention: [] };

    expect(fleet.filter((each) => matches(filter, each))).toHaveLength(0);
  });

  test("toggling adds then removes", () => {
    expect(toggle(["FREE"], "STALE")).toEqual(["FREE", "STALE"]);
    expect(toggle(["FREE", "STALE"], "FREE")).toEqual(["STALE"]);
  });
});

describe("what persists", () => {
  beforeEach(() => window.localStorage.clear());

  test("layer visibility survives, and is on by default", () => {
    expect(loadLayers().coverage).toBe(true);

    saveLayers({ coverage: false });
    expect(loadLayers().coverage).toBe(false);
  });

  // The guarantee is currently upheld by nobody writing one, which is exactly the kind of invariant that
  // regresses the first time somebody adds a convenience. An operator who inherits yesterday's filter
  // and sees four vehicles may reasonably conclude the fleet is four vehicles (ADR-0006 §6.11).
  test("nothing but the layer key is ever stored", () => {
    saveLayers({ coverage: false });

    expect(Object.keys(window.localStorage)).toEqual([layersKey]);
  });
});

describe("shared links", () => {
  test("a link carries the selected vehicle", () => {
    expect(selectionFromLocation("?vehicle=abc")).toBe("abc");
    expect(selectionFromLocation("")).toBeNull();
  });

  // A link that selects a vehicle adds information; a link that filters removes it. A colleague must not
  // be sent a view with most of the fleet silently hidden (ADR-0006 §6.3).
  test("a link never carries a filter", () => {
    const parameters = new URLSearchParams("?vehicle=abc&statuses=FREE&attention=STALE");

    expect(selectionFromLocation(parameters.toString())).toBe("abc");
    // Nothing reads the others, which is the point: they are not part of the vocabulary.
    expect(selectionFromLocation("?statuses=FREE")).toBeNull();
  });
});
