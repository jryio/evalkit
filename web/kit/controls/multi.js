// Multiple choice as pill toggles; each option may carry its own key.
import { html } from "../html.js";
import { cx, Key } from "../ui/bits.js";

export default function Multi({ q, value, proposed, set }) {
  const picked = value || [];
  const flip = v => {
    const next = picked.includes(v) ? picked.filter(x => x !== v) : [...picked, v];
    set(next.length ? next : proposed ? null : undefined); // null keeps a cleared proposal cleared
  };
  return html`<div class="chips" role="group" aria-label=${q.label}>
    ${q.options.map(o => html`<button class=${cx("chip", picked.includes(o.value) && "on")} aria-pressed=${picked.includes(o.value)} onClick=${() => flip(o.value)}>
      ${o.label}${o.key ? html`<${Key} k=${o.key} />` : null}
    </button>`)}
  </div>`;
}
