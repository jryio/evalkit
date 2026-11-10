// Micro charts for dense places (table headers and cells): a histogram, a share bar
// and a meter. Marks are thin and gray unless a tone carries meaning; every mark is a
// button with a tooltip, and clicking one filters by it.
import { html } from "../html.js";
import { cx, tone } from "./bits.js";

const plural = (n, word) => `${n} ${word}${n === 1 ? "" : "s"}`;

// Histogram: one bar per bin, scaled to the fullest bin. on(bin) toggles a filter.
export function Histogram({ bins, active, on, label }) {
  const max = Math.max(1, ...bins.map(b => b.count));
  return html`<div class="mc-hist" role="group" aria-label=${`${label} distribution`}>
    ${bins.map(b => html`<button class=${cx("mc-bin", active && sameBin(active, b) && "on")} data-tip=${`${b.label}: ${plural(b.count, "item")}`}
        aria-label=${`${b.label}: ${plural(b.count, "item")}`} disabled=${!b.count} onClick=${e => { e.stopPropagation(); on(b); }}>
      <span style=${{ height: b.count ? `${Math.max(12, (100 * b.count) / max)}%` : "0" }}></span>
    </button>`)}
  </div>`;
}

export const sameBin = (a, b) => a.lo === b.lo && a.hi === b.hi;

// ShareBar: one whole split by option, in option order; a part at zero draws nothing.
export function ShareBar({ counts, active, on, label }) {
  const total = counts.reduce((n, c) => n + c.count, 0);
  if (!total) return html`<div class="mc-share empty"></div>`;
  return html`<div class="mc-share" role="group" aria-label=${`${label} shares`}>
    ${counts.filter(c => c.count).map(c => {
      const pct = Math.round((100 * c.count) / total);
      return html`<button class=${cx("mc-part", tone(c.tone), active === c.value && "on")} style=${{ flexGrow: c.count }}
          data-tip=${`${c.label}: ${c.count} (${pct}%)`} aria-label=${`${c.label}: ${c.count} (${pct}%)`}
          onClick=${e => { e.stopPropagation(); on(c.value); }}><span></span></button>`;
    })}
  </div>`;
}

// Meter: a value's position in its range, as a short bar beside the number.
export function Meter({ value, range: [lo, hi] }) {
  const f = hi > lo ? Math.min(1, Math.max(0, (value - lo) / (hi - lo))) : 0;
  return html`<span class="mc-meter" aria-hidden="true"><span style=${{ width: `${100 * f}%` }}></span></span>`;
}
