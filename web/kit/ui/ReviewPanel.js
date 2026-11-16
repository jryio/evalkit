// The selected row's review beside the table: docked as a sidebar, or floating over
// the table body below its column headers. The full-screen mode is the review view. On a
// narrow window it is a bottom sheet: drag its tab down, tap outside or press ✕ to close.
import { html, useEffect, useLayoutEffect, useRef, useState } from "../html.js";
import { ItemView } from "./ItemView.js";
import { OpenIn } from "./chrome.js";

export function ReviewPanel({ mode, setMode, item, position, go, close: closeNow, ...view }) {
  const panel = useRef();
  const body = useRef();
  const top = useFloatTop(mode);
  const sheet = mode === "tray";
  const { close, drag, closing } = useSheet(panel, sheet, closeNow);
  // The panel takes focus when it opens and when its row changes, so a toolbar button
  // clicked before keeps nothing: every key goes to the review.
  useEffect(() => {
    if (body.current) body.current.scrollTop = 0;
    if (!panel.current?.contains(document.activeElement)) panel.current?.focus({ preventScroll: true });
  }, [item?.id]);
  return html`${sheet ? html`<div class=${closing ? "rp-scrim closing" : "rp-scrim"} onClick=${close}></div>` : null}
  <aside class=${`rp ${mode}${closing ? " closing" : ""}`} ref=${panel} tabIndex="-1" style=${mode === "floating" ? { top: `${top}px` } : null} aria-label="Review">
    ${sheet ? html`<div class="rp-grab" onPointerDown=${drag} aria-hidden="true"><span></span></div>` : null}
    <div class="rp-bar">
      <button class="icon-btn small" title="Previous row (K)" aria-label="Previous row" disabled=${position.index <= 0} onClick=${() => go(-1)}>‹</button>
      <button class="icon-btn small" title="Next row (J)" aria-label="Next row" disabled=${position.index < 0 || position.index >= position.count - 1} onClick=${() => go(1)}>›</button>
      <span class="position">${position.index >= 0 ? `${position.index + 1} of ${position.count}` : "Not in the filter"}</span>
      <div class="spacer"></div>
      ${sheet ? null : html`<${OpenIn} value=${mode} set=${setMode} />`}
      <button class="rp-close" title="Close (Esc)" aria-label="Close the review panel" onClick=${close}>✕ <kbd>esc</kbd></button>
    </div>
    <div class="rp-body" ref=${body}>
      ${item ? html`<div class="content"><${ItemView} key=${item.id} item=${item} ...${view} /></div>`
        : html`<p class="rp-empty">Select a row to review it here.</p>`}
    </div>
  </aside>`;
}

// useSheet drives the bottom sheet: close slides it down before it goes, and drag follows
// a pull on the tab, closing past a third of its height (or on a quick flick) and
// springing back otherwise.
function useSheet(panel, sheet, closeNow) {
  const [closing, setClosing] = useState(false);
  const close = () => {
    if (!sheet) return closeNow();
    setClosing(true);
    setTimeout(closeNow, 260);
  };
  const drag = e => {
    const el = panel.current, tab = e.currentTarget;
    const y0 = e.clientY, t0 = performance.now();
    el.classList.add("dragging");
    tab.setPointerCapture(e.pointerId);
    const move = ev => { el.style.transform = `translateY(${Math.max(0, ev.clientY - y0)}px)`; };
    const up = ev => {
      tab.removeEventListener("pointermove", move);
      tab.removeEventListener("pointerup", up);
      el.classList.remove("dragging");
      const dy = ev.clientY - y0, fast = dy / (performance.now() - t0) > 0.6;
      // Past the threshold it slides on down from where it was let go; short of it, it springs back.
      el.style.transform = dy > el.offsetHeight / 3 || (fast && dy > 30) ? "translateY(100%)" : "";
      if (el.style.transform) close();
    };
    tab.addEventListener("pointermove", move);
    tab.addEventListener("pointerup", up);
  };
  return { close, drag, closing };
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
