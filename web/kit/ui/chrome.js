// App chrome: toolbar, suite head, sheets, toast and banners.
import { html } from "../html.js";
import { cx, tone, runWhen, Segmented } from "./bits.js";
import { helpRows } from "../core/keys.js";
import { progressLabel } from "../core/derive.js";

const REVIEW = { value: "review", label: "Review", tip: "One item at a time, full screen" };
const TABLE = { value: "table", label: "Table", tip: "Every item as a row (Esc from review)" };
const RUNS = { value: "runs", label: "Runs", tip: "Every run: its score, its change and its spend (Esc from the table)" };
// Esc climbs one view: review to the table, the table to runs.
const UP = { review: "table", table: "runs" };

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
  return html`<${Segmented} class="small icons open-in" label="Open rows in">
    ${OPEN_MODES.map(m => html`<button class=${cx("seg", value === m.value && "on")} role="radio" aria-checked=${value === m.value}
        aria-label=${m.label} data-tip=${m.tip} onClick=${() => set(m.value)}>${m.icon}</button>`)}
  </${Segmented}>`;
}

// What ‹ › step through in each view, with the keys that do the same.
const STEPS = {
  review: { noun: "item", prev: "K or ←", next: "J or →" },
  table: { noun: "row", prev: "K or ↑", next: "J or ↓" },
};

// One full-width toolbar for every view, so its controls never move when the view changes.
// A suite with runs gets the Runs view first, in the order Esc climbs.
export function Toolbar({ view, setView, nav, saveLabel, sendLabel, openSend, run, openNav }) {
  const views = run ? [RUNS, TABLE, REVIEW] : [REVIEW, TABLE];
  return html`<div class="toolbar">
    ${openNav ? html`<button class="icon-btn nav-toggle" aria-label="Show items" onClick=${openNav}>☰</button>` : null}
    <${Segmented} class="small view-switch" label="View">
      ${views.map(v => html`<button class=${cx("seg", view === v.value && "on")} role="radio" aria-checked=${view === v.value}
          title=${v.tip} onClick=${() => setView(v.value)}>${v.label}${v.value === UP[view] ? html`<kbd>esc</kbd>` : null}</button>`)}
    </${Segmented}>
    ${run ? html`<${RunStepper} ...${run} />` : null}
    ${STEPS[view] ? html`<${Stepper} ...${nav} ...${STEPS[view]} />` : null}
    <div class="spacer"></div>
    <span class="save-state" aria-live="polite">${saveLabel}</span>
    <button class="btn-primary" onClick=${openSend}><span class="send-long">${sendLabel}</span><span class="send-short">Send</span> <kbd>⌘↵</kbd></button>
  </div>`;
}

// Stepper is ‹ › and the position; index is -1 when no row is selected.
function Stepper({ index, count, step, noun, prev, next }) {
  return html`
    <button class="icon-btn" title=${`Previous ${noun} (${prev})`} aria-label=${`Previous ${noun}`} disabled=${index <= 0} onClick=${() => step(-1)}>‹</button>
    <button class="icon-btn" title=${`Next ${noun} (${next})`} aria-label=${`Next ${noun}`} disabled=${index >= count - 1} onClick=${() => step(1)}>›</button>
    <span class="position">${count ? `${index >= 0 ? index + 1 : "–"} of ${count}` : ""}</span>`;
}

// SuiteHead is the suite's title, description and progress under the toolbar in review,
// laid out as the table's head so the two views share one page grid.
export function SuiteHead({ title, description, progress }) {
  return html`<div class="tv-head suite-head">
    <h1>${title}</h1>
    ${description ? html`<p class="tv-desc">${description}</p>` : null}
    <${Progress} progress=${progress} />
  </div>`;
}

// Progress is the review progress bar and count, shared by the review and table heads.
export function Progress({ progress: p }) {
  return html`<div class="tv-progress">
    <div class="progress"><span style=${{ width: `${p.total ? (100 * p.reviewed) / p.total : 0}%` }}></span></div>
    <span><strong>${p.reviewed} of ${p.total}</strong> ${p.flagged ? "flagged " : ""}reviewed</span>
  </div>`;
}

// RunStepper names the run on screen and what it is compared with, between previous and next.
function RunStepper({ run, vs, count, step }) {
  const when = runWhen(run);
  return html`<div class="run-stepper">
    <button class="icon-btn small" title="Previous run ([)" aria-label="Previous run" disabled=${run.index <= 0} onClick=${() => step(-1)}>‹</button>
    <span class="run-name">Run <strong>${run.label}</strong>${when ? ` · ${when}` : ""} · ${vs ? `vs ${vs.label}` : "no comparison"}</span>
    <button class="icon-btn small" title="Next run (])" aria-label="Next run" disabled=${run.index >= count - 1} onClick=${() => step(1)}>›</button>
  </div>`;
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

export function HelpSheet({ binds, sendLabel, description, view, runs, close }) {
  return html`<${Sheet} title="Keyboard shortcuts" close=${close} actions=${html`<button class="btn-primary" onClick=${close}>Done</button>`}>
    ${description ? html`<p>${description}</p>` : null}
    <div class="keys">
      ${helpRows(binds, sendLabel, view, runs).map(r => html`
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
