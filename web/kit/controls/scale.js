// A 1–N rating as a segmented control with words for the two ends.
import { html } from "../html.js";
import { cx, Segmented } from "../ui/bits.js";
import { proposalNote } from "../core/derive.js";

export const scaleRange = q => Array.from({ length: (q.max ?? 5) - (q.min ?? 1) + 1 }, (_, i) => (q.min ?? 1) + i);

export default function Scale({ q, value, answer, set }) {
  return html`<div class="scale">
    ${q.labels ? html`<span class="scale-end">${q.labels[0]}</span>` : null}
    <${Segmented} label=${q.label}>
      ${scaleRange(q).map(n => html`<button class=${cx("seg", "seg-num", value === n && "on")} role="radio" aria-checked=${value === n}
        onClick=${() => set(answer === n ? undefined : n)}>${n}</button>`)}
    </${Segmented}>
    ${q.labels ? html`<span class="scale-end">${q.labels[1]}</span>` : null}
  </div>`;
}

Scale.note = ({ proposed }) => proposalNote(proposed?.value, proposed);

Scale.labelKey = true;
