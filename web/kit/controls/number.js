// A number field with a unit. It keeps its own text while typing so "1." survives.
import { html, useState, useEffect } from "../html.js";

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
