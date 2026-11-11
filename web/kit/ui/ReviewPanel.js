// The selected row's review beside the table: docked as a sidebar, or floating over
// the table body below its column headers. The full-screen mode is the review view.
import { html, useEffect, useLayoutEffect, useRef, useState } from "../html.js";
import { ItemView } from "./ItemView.js";
import { OpenIn } from "./chrome.js";

export function ReviewPanel({ mode, setMode, item, position, go, close, ...view }) {
  const body = useRef();
  const top = useFloatTop(mode);
  useEffect(() => {
    if (body.current) body.current.scrollTop = 0;
  }, [item?.id]);
  return html`<aside class=${`rp ${mode}`} style=${mode === "floating" ? { top: `${top}px` } : null} aria-label="Review">
    <div class="rp-bar">
      <button class="icon-btn small" title="Previous row (K)" aria-label="Previous row" disabled=${position.index <= 0} onClick=${() => go(-1)}>‹</button>
      <button class="icon-btn small" title="Next row (J)" aria-label="Next row" disabled=${position.index < 0 || position.index >= position.count - 1} onClick=${() => go(1)}>›</button>
      <span class="position">${position.index >= 0 ? `${position.index + 1} of ${position.count}` : "Not in the filter"}</span>
      <div class="spacer"></div>
      <${OpenIn} value=${mode} set=${setMode} />
      <button class="rp-close" title="Close (Esc)" aria-label="Close the review panel" onClick=${close}>✕ <kbd>esc</kbd></button>
    </div>
    <div class="rp-body" ref=${body}>
      ${item ? html`<div class="content"><${ItemView} key=${item.id} item=${item} ...${view} /></div>`
        : html`<p class="rp-empty">Select a row to review it here.</p>`}
    </div>
  </aside>`;
}

// useFloatTop is where a floating panel starts: just below the table's column headers.
function useFloatTop(mode) {
  const [top, setTop] = useState(0);
  useLayoutEffect(() => {
    if (mode !== "floating") return;
    const measure = () => {
      const head = document.querySelector(".items-table thead");
      const layout = document.querySelector(".tv-layout");
      if (head && layout) setTop(head.getBoundingClientRect().bottom - layout.getBoundingClientRect().top + 8);
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(document.querySelector(".tv-layout"));
    return () => ro.disconnect();
  }, [mode]);
  return top;
}
