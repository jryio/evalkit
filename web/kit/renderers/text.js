// Plain text. Editable text gets a reading-size textarea and an optional length meter;
// read-only text sits on a panel fill. In context (collapsed) it uses body size.
import { html } from "../html.js";
import { AutoText, cx, tone } from "../ui/bits.js";
import { measure, limitTone } from "../core/derive.js";

export default function TextArtifact({ a, edit, setEdit, inContext }) {
  if (!a.editable) return html`<div class=${inContext ? "text-block" : "reading-block"}>${a.text}</div>`;
  const text = edit ?? a.text;
  return html`
    <${AutoText} cls="reading" value=${text} onInput=${setEdit} focus=${`a:${a.id ?? "edit"}`} />
    ${a.limit?.max ? html`<${Meter} n=${measure(text)} limit=${a.limit} />` : null}`;
}

function Meter({ n, limit }) {
  const pct = v => `${Math.min(100, (100 * v) / limit.max)}%`;
  const [lo, hi] = limit.target ?? [];
  return html`<div class="meter">
    ${lo !== undefined ? html`<span class="zone" style=${{ left: pct(lo), width: `calc(${pct(hi)} - ${pct(lo)})` }}></span>` : null}
    <span class=${cx("fill", tone(limitTone(n, limit)))} style=${{ width: pct(n) }}></span>
  </div>`;
}

// head adds the length to the panel header: colored against the limit, else muted.
TextArtifact.head = ({ a, edit }) => {
  const n = measure(edit ?? a.text);
  const count = html`<span class=${cx("count", a.limit && tone(limitTone(n, a.limit)), !a.limit && "muted")}>${n} characters</span>`;
  if (!a.limit) return count;
  const parts = [a.limit.target && `target ${a.limit.target[0]}–${a.limit.target[1]}`, a.limit.max && `max ${a.limit.max}`].filter(Boolean);
  return html`${count}<span class="muted">${parts.join(" · ")}</span>`;
};
