import type { AttentionReason, CoverageState, VehicleStatus } from "./contract.generated";

/**
 * The operator-facing words for the states the backend names. They live in one place because the same
 * words have to appear on the map, in the panel and in the legend: the legend is a defect if it does
 * not account for every distinction the map makes, and it cannot do that if each surface invents its
 * own vocabulary (PRODUCT-SPEC F1).
 */

export const statusWording: Record<VehicleStatus, string> = {
  FREE: "available",
  EN_ROUTE: "remotely driven",
  WITH_CUSTOMER: "with a customer",
};

export const attentionWording: Record<AttentionReason, string> = {
  NONE: "",
  LOW_BATTERY: "low energy",
  STALE: "not reporting",
};

export const coverageWording: Record<CoverageState, string> = {
  MEETING: "meeting its minimum",
  BELOW_MINIMUM: "below its minimum",
  NONE_AVAILABLE: "nothing available",
};
