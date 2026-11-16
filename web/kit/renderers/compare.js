// Code side by side across the whole width of the page: a deleted test beside the test
// that keeps its proof, a before beside an after. Each pane scrolls sideways on its own
// until Wrap folds long lines. The pane headers pin under the toolbar while the panes
// scroll past, so the reviewer always knows which side is which.
import { html, useState, useRef, useEffect } from "../html.js";
import { cx, tone, Dot } from "../ui/bits.js";
import { CodeLines } from "./code.js";

export default function Compare({ a }) {
  const [wrap, setWrap] = useState(Boolean(a.wrap));
  return html`<div class="compare-panes" style=${`--panes: ${a.panes.length}`}>
    ${a.panes.map((p, i) => html`<section class=${cx("compare-pane", tone(p.tone))} key=${i}>
      <${StickyHead}>
        ${p.tone ? html`<${Dot} tone=${p.tone} />` : null}
        <span class="compare-title">${p.label}</span>
        ${p.note ? html`<span class="muted">${p.note}</span>` : null}
        ${i === a.panes.length - 1 ? html`<button class=${cx("compare-wrap", wrap && "on")} aria-pressed=${wrap}
            onClick=${() => setWrap(!wrap)}>Wrap</button>` : null}
      </${StickyHead}>
      <${CodeLines} code=${p.code} language=${p.language ?? a.language} start=${p.start} highlight=${p.highlight}
        wrap=${wrap} maxLines=${a.max_lines} />
    </section>`)}
  </div>`;
}

// StickyHead pins to the top of the scroll area and marks itself stuck while pinned.
// It sits 1px above the edge, so it is never fully visible while stuck.
function StickyHead({ children }) {
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
  return html`<header ref=${ref} class=${cx("compare-head", stuck && "stuck")}>${children}</header>`;
}

// The kit lets a wide renderer leave the reading column and span the page.
Compare.wide = true;
