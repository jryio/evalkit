// Code side by side across the whole width of the page: a deleted test beside the test
// that keeps its proof, a before beside an after. Each pane scrolls sideways on its own
// until Wrap folds long lines.
import { html, useState } from "../html.js";
import { cx, tone, Dot } from "../ui/bits.js";
import { CodeLines } from "./code.js";

export default function Compare({ a }) {
  const [wrap, setWrap] = useState(Boolean(a.wrap));
  return html`<div class="compare-panes" style=${`--panes: ${a.panes.length}`}>
    ${a.panes.map((p, i) => html`<section class=${cx("compare-pane", tone(p.tone))} key=${i}>
      <header class="compare-head">
        ${p.tone ? html`<${Dot} tone=${p.tone} />` : null}
        <span class="compare-title">${p.label}</span>
        ${p.note ? html`<span class="muted">${p.note}</span>` : null}
        ${i === a.panes.length - 1 ? html`<button class=${cx("compare-wrap", wrap && "on")} aria-pressed=${wrap}
            onClick=${() => setWrap(!wrap)}>Wrap</button>` : null}
      </header>
      <${CodeLines} code=${p.code} language=${p.language ?? a.language} start=${p.start} highlight=${p.highlight} wrap=${wrap} />
    </section>`)}
  </div>`;
}

// The kit lets a wide renderer leave the reading column and span the page.
Compare.wide = true;
