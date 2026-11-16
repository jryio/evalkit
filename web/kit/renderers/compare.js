// Code side by side across the whole width of the page: a deleted test beside the test
// that keeps its proof, a before beside an after. The pane headers form one row that pins
// under the toolbar while the code scrolls by, so the reviewer always knows which side is
// which. With two panes, Diff aligns them as a split diff of left against right.
import { html, useState, useRef, useEffect } from "../html.js";
import { cx, tone, Dot } from "../ui/bits.js";
import { CodeLines } from "./code.js";
import { PierreDiff, ViewToggle, grammarOf } from "./pierre.js";

export default function Compare({ a }) {
  const [wrap, setWrap] = useState(Boolean(a.wrap));
  const two = a.panes.length === 2;
  const [mode, setMode] = useState(two && a.diff ? "diff" : "panes");
  const file = p => ({ name: p.label ?? "", contents: p.code ?? "", lang: grammarOf(p.language ?? a.language) });
  return html`<div class="compare" style=${`--panes: ${a.panes.length}`}>
    <${StickyRow}>
      ${a.panes.map((p, i) => html`<div class=${cx("compare-head", tone(p.tone))} key=${i}>
        ${p.tone ? html`<${Dot} tone=${p.tone} />` : null}
        <span class="compare-title">${p.label}</span>
        ${p.note ? html`<span class="muted">${p.note}</span>` : null}
        ${i === a.panes.length - 1 ? html`<span class="compare-tools">
          ${two ? html`<${ViewToggle} options=${[["panes", "Panes"], ["diff", "Diff"]]} value=${mode} set=${setMode} />` : null}
          <button class=${cx("compare-wrap", wrap && "on")} aria-pressed=${wrap} onClick=${() => setWrap(!wrap)}>Wrap</button>
        </span>` : null}
      </div>`)}
    </${StickyRow}>
    ${mode === "diff"
      ? html`<${PierreDiff} oldFile=${file(a.panes[0])} newFile=${file(a.panes[1])} wrap=${wrap} maxLines=${a.max_lines} />`
      : html`<div class="compare-panes">${a.panes.map((p, i) => html`<div class=${cx("compare-pane", tone(p.tone))} key=${i}>
          <${CodeLines} code=${p.code} language=${p.language ?? a.language} start=${p.start} highlight=${p.highlight} wrap=${wrap} maxLines=${a.max_lines} />
        </div>`)}</div>`}
  </div>`;
}

// StickyRow pins to the top of the scroll area and marks itself stuck while pinned.
// It sits 1px above the edge, so it is never fully visible while stuck.
function StickyRow({ children }) {
  const ref = useRef();
  const [stuck, setStuck] = useState(false);
  useEffect(() => {
    const el = ref.current;
    const root = el?.closest(".scroll");
    if (!el || !root) return;
    const io = new IntersectionObserver(([e]) => setStuck(e.intersectionRatio < 1 && e.boundingClientRect.top <= e.rootBounds.top), { root, threshold: [1] });
    io.observe(el);
    return () => io.disconnect();
  }, []);
  return html`<div ref=${ref} class=${cx("compare-heads", stuck && "stuck")}>${children}</div>`;
}

// The kit lets a wide renderer leave the reading column and span the page.
Compare.wide = true;
