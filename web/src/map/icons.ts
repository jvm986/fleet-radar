import type { AttentionReason, VehicleStatus } from "../contract.generated";

/**
 * The marker images, drawn at load rather than shipped as assets.
 *
 * One directional marker per vehicle, carrying heading by orientation and status by colour *together
 * with a fill treatment*. The second channel is not decoration: heading has already taken the
 * orientation channel, so without a difference in shape, status would be carried by colour alone —
 * which fails an operator on a bad monitor, under glare, or at 2am as surely as it fails a colourblind
 * one (PRODUCT-SPEC §7.4, §7.5).
 *
 * The colours are blue, orange and purple rather than any red/green pairing, so the three stay
 * distinguishable under the common colour vision deficiencies as well.
 */
export const statusIcons: Record<
  VehicleStatus,
  { id: string; colour: string; fill: FillTreatment }
> = {
  FREE: { id: "vehicle-free", colour: "#2b7fa8", fill: "hollow" },
  EN_ROUTE: { id: "vehicle-en-route", colour: "#d97b1a", fill: "solid" },
  WITH_CUSTOMER: { id: "vehicle-with-customer", colour: "#7a5aa8", fill: "cored" },
};

/**
 * A vehicle warranting attention gains a halo and a badge, added to its marker rather than replacing
 * any part of it. Recolouring the marker would be the loudest signal available and would destroy
 * status on exactly the vehicles where status matters most — a stale FREE vehicle and a stale
 * WITH_CUSTOMER vehicle are very different situations (PRODUCT-SPEC §7.5).
 *
 * Colour and badge both differ, so neither condition is distinguished by colour alone.
 */
export const attentionMarks: Record<
  Exclude<AttentionReason, "NONE">,
  { halo: string; badge: string }
> = {
  LOW_BATTERY: { halo: "#e8b21e", badge: "!" },
  STALE: { halo: "#8a9099", badge: "?" },
};

export const coveragePatterns = {
  BELOW_MINIMUM: { id: "coverage-below", colour: "#e8b21e", spacing: 8 },
  NONE_AVAILABLE: { id: "coverage-none", colour: "#c0392b", spacing: 4 },
} as const;

export type FillTreatment = "hollow" | "solid" | "cored";

/** markerSize is the drawn size in logical pixels: large enough to read a heading from, small enough
 * that a hundred of them stay individually distinguishable. */
export const markerSize = 20;

const scale = 2;

/** vehicleImage draws the arrow pointing north, because the symbol layer rotates it by the vehicle's
 * heading, which is degrees clockwise from north. */
export function vehicleImage(colour: string, fill: FillTreatment): ImageData {
  const { context, size } = canvas(markerSize);
  const point = (x: number, y: number): [number, number] => [x * size, y * size];

  context.beginPath();
  context.moveTo(...point(0.5, 0.06));
  context.lineTo(...point(0.88, 0.94));
  context.lineTo(...point(0.5, 0.72));
  context.lineTo(...point(0.12, 0.94));
  context.closePath();

  context.lineJoin = "round";
  context.lineWidth = 2 * scale;
  context.strokeStyle = colour;

  if (fill === "hollow") {
    // Still filled, faintly: an outline alone disappears against a busy basemap, and the shape has to
    // stay readable at this size.
    context.fillStyle = "rgba(255, 255, 255, 0.85)";
    context.fill();
    context.stroke();
  } else {
    context.fillStyle = colour;
    context.fill();
    context.strokeStyle = "rgba(255, 255, 255, 0.9)";
    context.stroke();
  }

  if (fill === "cored") {
    context.beginPath();
    context.arc(...point(0.5, 0.62), 0.13 * size, 0, 2 * Math.PI);
    context.fillStyle = "#ffffff";
    context.fill();
  }

  return context.getImageData(0, 0, size, size);
}

/** hatchImage is a tileable diagonal hatch. Coverage is paired with a pattern as well as a colour,
 * because the prohibition on colour-alone applies to zones as much as to vehicles (ADR-0002 §2.9). */
export function hatchImage(colour: string, spacing: number): ImageData {
  const { context, size } = canvas(spacing * 2);

  context.strokeStyle = colour;
  context.lineWidth = 1.5 * scale;
  for (let offset = -size; offset < size * 2; offset += spacing * scale) {
    context.beginPath();
    context.moveTo(offset, 0);
    context.lineTo(offset + size, size);
    context.stroke();
  }
  return context.getImageData(0, 0, size, size);
}

function canvas(logicalSize: number): { context: CanvasRenderingContext2D; size: number } {
  const size = logicalSize * scale;
  const element = document.createElement("canvas");
  element.width = size;
  element.height = size;

  const context = element.getContext("2d");
  if (context === null) {
    throw new Error("no 2d canvas context, so the markers cannot be drawn");
  }
  return { context, size };
}

/** pixelRatio is what the images were drawn at, so MapLibre places them at their logical size. */
export const pixelRatio = scale;
