// A number field with a unit. It keeps its own text while typing so "1." survives.
// With both min and max (a bounded score) a slider sits beside it.
import { html, useState, useEffect } from "../html.js";
import { cx } from "../ui/bits.js";

export default function NumberField({ q, value, answer, proposed, set, row }) {
  const [text, setText] = useState(value ?? "");
  useEffect(() => {
    if (text === "" ? value !== undefined : Number(text) !== value) setText(value ?? "");
  }, [value]);
  const onInput = e => {
    const t = e.currentTarget.value;
    setText(t);
    if (t === "") set(proposed ? null : undefined); // null keeps a cleared proposal cleared
    else if (Number.isFinite(Number(t))) set(Number(t));
  };
  return html`<span class="number-field">
    <input type="text" inputmode="decimal" value=${text} placeholder=${q.placeholder ?? ""}
      class=${answer === undefined && value !== undefined ? "tentative" : ""} data-focus=${row === undefined ? `q:${q.id}` : undefined}
      aria-label=${q.label} onInput=${onInput} />
    ${q.unit ? html`<span class="unit">${q.unit}</span>` : null}
    ${q.min !== undefined && q.max !== undefined && row === undefined ? html`<input type="range" min=${q.min} max=${q.max} step=${q.step ?? "any"}
        value=${value ?? q.min} class=${cx("number-slider", answer === undefined && "tentative", value === undefined && "empty")} aria-label=${q.label}
        onInput=${e => set(Number(e.currentTarget.value))} />` : null}
  </span>`;
}

NumberField.note = ({ q, value, proposed }) => {
  if (value !== undefined && ((q.min !== undefined && value < q.min) || (q.max !== undefined && value > q.max))) {
    return `Must be between ${q.min ?? "−∞"} and ${q.max ?? "∞"}`;
  }
  if (!proposed) return null;
  return value === proposed.value ? proposed.why ?? null : `Changed from ${proposed.value}`;
};

NumberField.labelKey = true;
