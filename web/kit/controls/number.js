// A number field with a unit. It keeps its own text while typing so "1." survives.
// With both min and max (a bounded score) a slider sits beside it; it lets go of focus
// after a drag so the kit's keys work again.
import { html, useState, useEffect } from "../html.js";
import { cx } from "../ui/bits.js";
import { proposalNote } from "../core/derive.js";

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
  // While the field has focus, ↑ and → raise the value by one step, ↓ and ← lower it, and ↵ lets go.
  const onKeyDown = e => {
    if (e.key === "Enter") return e.currentTarget.blur();
    const by = ARROWS[e.key];
    if (!by) return;
    e.preventDefault();
    const start = text !== "" && Number.isFinite(Number(text)) ? Number(text) : value ?? proposed?.value ?? q.min ?? 0;
    const next = stepped(start, by, q);
    setText(next);
    set(next);
  };
  return html`<span class="number-field">
    <input type="text" inputmode="decimal" value=${text} placeholder=${q.placeholder ?? ""}
      class=${answer === undefined && value !== undefined ? "tentative" : ""} data-focus=${row === undefined ? `q:${q.id}` : undefined}
      aria-label=${q.label} onInput=${onInput} onKeyDown=${onKeyDown} />
    ${q.unit ? html`<span class="unit">${q.unit}</span>` : null}
    ${q.min !== undefined && q.max !== undefined && row === undefined ? html`<input type="range" min=${q.min} max=${q.max} step=${q.step ?? "any"}
        value=${value ?? q.min} class=${cx("number-slider", answer === undefined && "tentative", value === undefined && "empty")} aria-label=${q.label}
        onInput=${e => set(Number(e.currentTarget.value))} onPointerUp=${e => e.currentTarget.blur()} />` : null}
  </span>`;
}

const ARROWS = { ArrowUp: 1, ArrowRight: 1, ArrowDown: -1, ArrowLeft: -1 };

// stepped is start moved by one step, kept within min and max, and rounded to the step's decimals.
export function stepped(start, by, q) {
  const step = q.step ?? 1;
  const decimals = (String(step).split(".")[1] ?? "").length;
  const next = Number((start + by * step).toFixed(decimals));
  return Math.min(q.max ?? Infinity, Math.max(q.min ?? -Infinity, next));
}

NumberField.note =({ q, value, proposed }) => {
  if (value !== undefined && ((q.min !== undefined && value < q.min) || (q.max !== undefined && value > q.max))) {
    return `Must be between ${q.min ?? "−∞"} and ${q.max ?? "∞"}`;
  }
  return proposalNote(proposed?.value, proposed);
};

NumberField.labelKey = true;
