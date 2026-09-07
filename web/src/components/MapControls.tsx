import { type ReactNode, useState } from "react";
import type { Layers } from "../state";
import Coverage from "./Coverage";
import Legend from "./Legend";

type Panel = "coverage" | "legend" | null;

interface Props {
  layers: Layers;
  onLayers: (layers: Layers) => void;
  /** disabled holds while the view is not current. Changing what is drawn over a fleet we are no longer
   * hearing about would dress up an answer about the past as an answer about now (PRODUCT-SPEC F6). */
  disabled: boolean;
}

/**
 * The two panels that describe the map rather than the fleet: where coverage is thin, and how to read what
 * is drawn. The switch that shades short zones lives inside the coverage panel, beside the numbers it
 * shades, rather than in a control of its own.
 *
 * Both live behind icons in one row, with the open panel beneath, so they cost a corner of the map rather
 * than a column of it. One at a time: there is a single place a panel appears, and two panels stacked
 * there would push the second below the fold on a short window.
 *
 * ⚠️ The cost of hiding the legend is real and worth naming rather than glossing. An operator who has not
 * learned the encoding cannot read the map without it, and a closed legend explains nothing until they
 * think to open it — the icon and its tooltip are all that stand between the two (PRODUCT-SPEC F1).
 */
export default function MapControls({ layers, onLayers, disabled }: Props) {
  const [open, setOpen] = useState<Panel>(null);
  const show = (panel: Panel) => setOpen(open === panel ? null : panel);

  return (
    <div className="controls" inert={disabled}>
      <div className="control-bar">
        <IconButton
          label="Zone coverage"
          icon={<CoverageIcon />}
          on={open === "coverage"}
          onPress={() => show("coverage")}
        />
        <IconButton
          label="Legend"
          icon={<LegendIcon />}
          on={open === "legend"}
          onPress={() => show("legend")}
        />
      </div>

      {open !== null && (
        <section className="control-body">
          {open === "coverage" ? <Coverage layers={layers} onLayers={onLayers} /> : <Legend />}
        </section>
      )}
    </div>
  );
}

/**
 * An icon with no visible label is a control with no accessible name unless `aria-label` and `title` are
 * both set, and a disclosure is not a disclosure unless it reports `aria-expanded`. Shared so that cannot
 * drift between the two buttons.
 */
function IconButton({
  label,
  icon,
  on,
  onPress,
}: {
  label: string;
  icon: ReactNode;
  on: boolean;
  onPress: () => void;
}) {
  return (
    <button
      type="button"
      className={on ? "icon-button on" : "icon-button"}
      title={label}
      aria-label={label}
      aria-expanded={on}
      onClick={onPress}
    >
      {icon}
    </button>
  );
}

/** A zone outline with the hatching that marks it short. Drawn here rather than taken from an icon set,
 * which would be a dependency of roughly a thousand glyphs for the two this application uses. */
function CoverageIcon() {
  return (
    <svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true" focusable="false">
      <g fill="none" stroke="currentColor" strokeWidth="1.3">
        <path d="M2.2 2.2h11.6v11.6H2.2z" strokeLinejoin="round" />
        <path d="m3 8.2 5.2-5.2M3 12.4 12.4 3M6.4 13 13 6.4M10.6 13 13 10.6" strokeWidth="1" />
      </g>
    </svg>
  );
}

/** A map key: three swatches against three rows. */
function LegendIcon() {
  return (
    <svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true" focusable="false">
      <g fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round">
        <path d="M1.8 3.6h2.4M1.8 8h2.4M1.8 12.4h2.4" />
        <path d="M7 3.6h7.2M7 8h7.2M7 12.4h7.2" />
      </g>
    </svg>
  );
}
