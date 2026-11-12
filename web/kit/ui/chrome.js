// App chrome: toolbar, sheets, toast and banners.
import { html } from "../html.js";
import { cx, tone } from "./bits.js";
import { helpRows } from "../core/keys.js";
import { progressLabel } from "../core/derive.js";

const VIEWS = [
  { value: "review", label: "Review", tip: "One item at a time, full screen" },
  { value: "table", label: "Table", tip: "Every item as a row (Esc from review)" },
];

// Where Enter opens a table row. Icons follow the shape each mode takes on screen.
const OPEN_MODES = [
  { value: "sidebar", label: "Sidebar", tip: "Open rows in a sidebar beside the table",
    icon: html`<svg viewBox="0 0 16 12" aria-hidden="true"><rect x=".75" y=".75" width="14.5" height="10.5" rx="2" fill="none" stroke="currentColor" stroke-width="1.5"/><rect x="9.5" y="2.5" width="4" height="7" rx="1" fill="currentColor"/></svg>` },
  { value: "floating", label: "Floating", tip: "Open rows in a panel floating over the table",
    icon: html`<svg viewBox="0 0 16 12" aria-hidden="true"><rect x=".75" y=".75" width="14.5" height="10.5" rx="2" fill="none" stroke="currentColor" stroke-width="1.5"/><rect x="6.5" y="4" width="7" height="5.5" rx="1" fill="currentColor"/></svg>` },
  { value: "full", label: "Full screen", tip: "Open rows full screen in Review",
    icon: html`<svg viewBox="0 0 16 12" aria-hidden="true"><rect x=".75" y=".75" width="14.5" height="10.5" rx="2" fill="none" stroke="currentColor" stroke-width="1.5"/><rect x="2.5" y="2.5" width="11" height="7" rx="1" fill="currentColor"/></svg>` },
];

export function OpenIn({ value, set }) {
  return html`<div class="segmented small icons open-in" role="radiogroup" aria-label="Open rows in">
    ${OPEN_MODES.map(m => html`<button class=${cx("seg", value === m.value && "on")} role="radio" aria-checked=${value === m.value}
        aria-label=${m.label} data-tip=${m.tip} onClick=${() => set(m.value)}>${m.icon}</button>`)}
  </div>`;
}

export function Toolbar({ view, setView, index, count, go, saveLabel, sendLabel, openSend }) {
  return html`<div class="toolbar"><div class="toolbar-inner">
    <div class="segmented small view-switch" role="radiogroup" aria-label="View">
      ${VIEWS.map(v => html`<button class=${cx("seg", view === v.value && "on")} role="radio" aria-checked=${view === v.value}
          title=${v.tip} onClick=${() => setView(v.value)}>${v.label}${v.value === "table" && view === "review" ? html`<kbd>esc</kbd>` : null}</button>`)}
    </div>
    ${view === "review" ? html`
      <button class="icon-btn" title="Previous (K or ←)" aria-label="Previous item" disabled=${index <= 0} onClick=${() => go(index - 1)}>‹</button>
      <button class="icon-btn" title="Next (J or →)" aria-label="Next item" disabled=${index >= count - 1} onClick=${() => go(index + 1)}>›</button>
      <span class="position">${count ? `${index + 1} of ${count}` : ""}</span>` : null}
    <div class="spacer"></div>
    <span class="save-state" aria-live="polite">${saveLabel}</span>
    <button class="btn-primary" onClick=${openSend}>${sendLabel} <kbd>⌘↵</kbd></button>
  </div></div>`;
}

export function Sheet({ title, children, actions, close }) {
  return html`<div class="backdrop" onClick=${e => e.target === e.currentTarget && close()}>
    <div class="sheet" role="dialog" aria-modal="true" aria-label=${title}>
      <h2>${title}</h2>
      ${children}
      <div class="actions">${actions}</div>
    </div>
  </div>`;
}

// SendSheet says exactly what the send triggers: progress and every approved action.
export function SendSheet({ sendLabel, progress, actions, pending, close, confirm }) {
  return html`<${Sheet} title=${`${sendLabel}?`} close=${close} actions=${html`
      <button class="btn-secondary" data-role="cancel" onClick=${close}>Cancel</button>
      <button class="btn-primary" onClick=${confirm} ref=${el => el?.focus()}>Send <kbd>↵</kbd></button>`}>
    <p>${progressLabel(progress)}.</p>
    ${actions.map(q => {
      const approved = pending.filter(p => p.question.id === q.id).map(p => p.item.short ?? p.item.title);
      return html`<p key=${q.id}>${approved.length
        ? `${q.label}: ${approved.length} approved (${approved.join(", ")}). ${q.summary ?? ""}`
        : `${q.label}: nothing approved.`}</p>`;
    })}
    <p>You can keep reviewing and send again anytime.</p>
  </${Sheet}>`;
}

export function HelpSheet({ binds, sendLabel, description, view, close }) {
  return html`<${Sheet} title="Keyboard shortcuts" close=${close} actions=${html`<button class="btn-primary" onClick=${close}>Done</button>`}>
    ${description ? html`<p>${description}</p>` : null}
    <div class="keys">
      ${helpRows(binds, sendLabel, view).map(r => html`
        <span class="caps">${r.keys.map(k => html`<kbd>${k}</kbd>`)}</span>
        <span>${r.label}${r.also.length ? html` (also ${r.also.map(k => html`<kbd>${k}</kbd> `)})` : null}</span>`)}
    </div>
  </${Sheet}>`;
}

export const Toast = ({ message, show }) => html`<div class=${cx("toast", show && "show")} role="status">${message}</div>`;

export function Banner({ tone: t, title, lines = [], action }) {
  return html`<div class=${cx("banner", tone(t))}>
    <div><strong>${title}</strong>${lines.length ? html`<ul>${lines.map(l => html`<li>${l}</li>`)}</ul>` : null}</div>
    ${action ?? null}
  </div>`;
}
