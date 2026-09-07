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
 *
 * Every colour in this file clears 3:1 against the basemap background (#f2f3f0), which is WCAG 2.1
 * SC 1.4.11's threshold for a graphical object that carries meaning. That is a floor rather than a
 * preference: below it the marker is present but its status is not readable, which fails the operator in
 * the same way as not drawing it. EN_ROUTE was #d97b1a and reached only 2.77, so it is darkened here —
 * at constant hue, so the CVD reasoning above is unaffected, and the added lightness separation from
 * blue and purple only strengthens it.
 *
 * ⚠️ WITH_CUSTOMER is the case that shows why 3:1 is a floor and not a measure of legibility. At #7a5aa8
 * it held 4.89 — the *best* ratio of the three — and was still the hardest marker to find on the map. The
 * reason is chroma rather than lightness, which contrast ratio does not describe: its saturation was 0.46
 * against blue's 0.74 and orange's 0.88, so a greyish violet sat on a deliberately grey basemap and read
 * as part of it. Raised to #6b3fa0 it carries saturation 0.61 and 6.63 against the map, and the white core
 * of its fill treatment gains contrast too. Worth recording because the metric said this marker was the
 * healthiest one on the display.
 */
export const statusIcons: Record<
  VehicleStatus,
  { id: string; colour: string; fill: FillTreatment }
> = {
  FREE: { id: "vehicle-free", colour: "#2b7fa8", fill: "hollow" },
  EN_ROUTE: { id: "vehicle-en-route", colour: "#bf6c17", fill: "solid" },
  WITH_CUSTOMER: { id: "vehicle-with-customer", colour: "#6b3fa0", fill: "cored" },
};

/**
 * A vehicle warranting attention gains a halo and a badge, added to its marker rather than replacing
 * any part of it. Recolouring the marker would be the loudest signal available and would destroy
 * status on exactly the vehicles where status matters most — a stale FREE vehicle and a stale
 * WITH_CUSTOMER vehicle are very different situations (PRODUCT-SPEC §7.5).
 *
 * Colour and badge both differ, so neither condition is distinguished by colour alone.
 *
 * These two carry a second constraint the status colours do not: each is also the badge's text halo
 * (layers.ts, `haloFor`), so it has to hold 3:1 against the map *and* stay legible under the badge's
 * #111827 glyph. LOW_BATTERY was #e8b21e, which managed only 1.74 against the map — the weakest ink on
 * the whole display, on the layer whose entire job is to be noticed. Darkened, it reaches 3.50 while the
 * badge still reads at 4.55.
 */
export const attentionMarks: Record<
  Exclude<AttentionReason, "NONE">,
  { halo: string; badge: string }
> = {
  LOW_BATTERY: { halo: "#a17c15", badge: "!" },
  STALE: { halo: "#7c818a", badge: "?" },
};

/** BELOW_MINIMUM shares LOW_BATTERY's amber deliberately rather than by copy-paste: both mean a reading
 * that has fallen under a stated threshold, and the operator reads them as the same kind of problem. The
 * red is unchanged, already at 4.88. */
export const coveragePatterns = {
  BELOW_MINIMUM: { id: "coverage-below", colour: "#a17c15", spacing: 8 },
  NONE_AVAILABLE: { id: "coverage-none", colour: "#c0392b", spacing: 4 },
} as const;

type FillTreatment = "hollow" | "solid" | "cored";

/** borderShade is how much darker a marker's border is than its body. Every marker is bordered in its own
 * colour rather than in white.
 *
 * ⚠️ That is what makes the three read at one size, which is the part worth recording. The border straddles
 * the outline, half in and half out. A white border's outer half disappears into a light basemap while its
 * inner half eats into the body, so a solid marker's *visible colour* stopped a whole line width inside
 * where the hollow marker's blue outline reached — and the hollow one consequently looked bigger than the
 * other two at identical geometry. Bordering in colour puts all three boundaries in the same place. */
const borderShade = 0.68;

/** markerSize is the drawn size in logical pixels: large enough to read a heading from, small enough
 * that a hundred of them stay individually distinguishable. */
export const markerSize = 18;

const scale = 2;

/** vehicleImage draws the arrow pointing north, because the symbol layer rotates it by the vehicle's
 * heading, which is degrees clockwise from north. */
export function vehicleImage(colour: string, fill: FillTreatment): ImageData {
  const { context, size } = canvas(markerSize);
  const point = (x: number, y: number): [number, number] => [x * size, y * size];

  context.beginPath();
  context.moveTo(...point(0.5, 0.06));
  context.lineTo(...point(0.88, 0.94));
  context.lineTo(...point(0.5, 0.82));
  context.lineTo(...point(0.12, 0.94));
  context.closePath();

  context.lineJoin = "round";
  context.lineWidth = 2 * scale;

  if (fill === "hollow") {
    // Still filled, faintly: an outline alone disappears against a busy basemap, and the shape has to
    // stay readable at this size.
    context.fillStyle = "rgba(255, 255, 255, 0.85)";
    context.fill();
    context.strokeStyle = colour;
    context.stroke();
  } else {
    context.fillStyle = colour;
    context.fill();
    context.strokeStyle = shade(colour, borderShade);
    context.stroke();
  }

  // The centre is what tells this marker from the plain solid one, so it does not rest on colour
  // (PRODUCT-SPEC §7.4). It can be generous now that the border is coloured: with a white border it met
  // that border from the inside and squeezed the body into a thin band, which is what made this the
  // faintest marker of the three.
  if (fill === "cored") {
    context.beginPath();
    context.arc(...point(0.5, 0.64), 0.12 * size, 0, 2 * Math.PI);
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

/**
 * dataUrl renders one of the images above as an image the legend can show. The legend is a defect if it
 * does not account for every distinction the map makes, so it draws its swatches with this same code
 * rather than with a hand-made copy that could drift from it (PRODUCT-SPEC F1).
 */
export function dataUrl(image: ImageData): string {
  const element = document.createElement("canvas");
  element.width = image.width;
  element.height = image.height;

  const context = element.getContext("2d");
  if (context === null) {
    throw new Error("no 2d canvas context, so the legend cannot be drawn");
  }
  context.putImageData(image, 0, 0);
  return element.toDataURL();
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

/** shade scales a hex colour's channels, which lowers its brightness while leaving hue and saturation
 * where they were. Derived rather than listed as three more constants, so a marker's border cannot be
 * left behind when its body colour changes. */
function shade(colour: string, factor: number): string {
  const value = Number.parseInt(colour.slice(1), 16);
  const channel = (offset: number) => Math.round(((value >> offset) & 0xff) * factor);
  return `rgb(${channel(16)}, ${channel(8)}, ${channel(0)})`;
}

/** pixelRatio is what the images were drawn at, so MapLibre places them at their logical size. */
export const pixelRatio = scale;
