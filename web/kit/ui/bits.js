// Small shared pieces: class helpers, key caps, tone dots, icons, an auto-growing
// textarea and an error boundary so one bad artifact never blanks the page.
import { html, useRef, useLayoutEffect } from "../html.js";
import { Component } from "preact";
import { keyName } from "../core/keys.js";

export const cx = (...xs) => xs.filter(Boolean).join(" ");
export const tone = t => (t ? `tone-${t}` : "");

export const Key = ({ k }) => html`<kbd>${keyName(k)}</kbd>`;
export const Dot = ({ tone: t }) => html`<span class=${cx("dot", tone(t))}></span>`;

const ICONS = { check: "✓", approx: "≈", cross: "✕", question: "?" };
export const Icon = ({ name }) => html`<span class="glyph" aria-hidden="true">${ICONS[name] ?? ""}</span>`;

export const StatusTag = ({ tag }) => html`<span class="status-tag"><${Dot} tone=${tag.tone} />${tag.label}</span>`;

export { withUnit } from "../core/format.js";

export const fmtDate = iso => new Date(iso).toLocaleDateString("en-US", { month: "short", day: "numeric", timeZone: "UTC" });
// runWhen is a run's date ("Nov 10"), else its round ("round 3"), when its dims have one.
export function runWhen(run) {
  const { date, round } = run.dims;
  if (date !== undefined) return isDate(date) ? fmtDate(date) : String(date);
  return round !== undefined ? `round ${round}` : null;
}
export const fmtTime = date => date.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" });

// AutoText grows with its content. focus is the data-focus id keyboard shortcuts target.
export function AutoText({ value, onInput, cls, placeholder, rows = 2, focus, spellcheck = true }) {
  const ref = useRef();
  useLayoutEffect(() => {
    const el = ref.current;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight + 2}px`;
  }, [value]);
  return html`<textarea ref=${ref} class=${cls} rows=${rows} placeholder=${placeholder} data-focus=${focus}
    spellcheck=${spellcheck} value=${value ?? ""} onInput=${e => onInput(e.currentTarget.value)}></textarea>`;
}

export class Boundary extends Component {
  componentDidCatch(error) {
    this.setState({ error });
  }
  render({ label, children }, { error }) {
    return error ? html`<div class="artifact-error">${label} failed to render: ${error.message}</div>` : children;
  }
}

export const isDate = v => /^\d{4}-\d\d-\d\d/.test(String(v));
