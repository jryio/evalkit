// Free text. Renders as its own section rather than a row in the controls card.
import { html } from "../html.js";
import { AutoText } from "../ui/bits.js";

export default function Text({ q, value, proposed, set: store, compact, row }) {
  const set = v => store(v === "" && proposed ? null : v); // null keeps a cleared proposal cleared
  if (row !== undefined) {
    return html`<input class="row-input" type="text" value=${value ?? ""} placeholder=${q.placeholder ?? q.label} aria-label=${q.label}
      onInput=${e => set(e.currentTarget.value)} />`;
  }
  return html`<${AutoText} value=${value} onInput=${v => set(v)} focus=${`q:${q.id}`}
    rows=${compact ? 1 : 2} placeholder=${q.placeholder ?? (compact ? "Answer" : "")} />`;
}

Text.layout = "section";
