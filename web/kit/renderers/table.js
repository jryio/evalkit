// A table. A cell is a plain value or { value, tone }. Rows with an id can carry
// per-row controls: lead controls get a first column, the rest a column each.
import { html } from "../html.js";
import { cx, tone } from "../ui/bits.js";

const cell = v => (v !== null && typeof v === "object" ? v : { value: v });

export default function Table({ a, rowControls, cursor }) {
  const controls = a.rows.map(r => (r.id !== undefined ? rowControls(r) : {}));
  const lead = controls.some(c => c.lead);
  // A control column shows only while some row has a control in it (`when` can hide them all).
  const labels = controls.find(c => c.cells)?.cells.map(c => c.label) ?? [];
  const extra = labels.map((label, j) => ({ label, j })).filter(({ j }) => controls.some(c => c.cells?.[j]?.node));
  return html`<table class=${extra.length || lead ? "has-controls" : ""}>
    <thead><tr>
      ${lead ? html`<th></th>` : null}
      ${a.columns.map(c => html`<th class=${c.align === "right" ? "right" : ""}>${c.label}</th>`)}
      ${extra.map(({ label }) => html`<th>${label}</th>`)}
    </tr></thead>
    <tbody>${a.rows.map((r, i) => html`<tr class=${cx(cursor !== undefined && cursor === r.id && "row-cursor")} data-row=${r.id !== undefined ? `${a.id}:${r.id}` : undefined}>
      ${lead ? html`<td class="cell-controls">${controls[i].lead}</td>` : null}
      ${a.columns.map(c => {
        const v = cell(r[c.key]);
        return html`<td class=${cx(c.align === "right" && "right", c.muted && "muted", v.tone && "toned", tone(v.tone))}>${v.value ?? ""}</td>`;
      })}
      ${extra.map(({ j }) => html`<td class="cell-controls">${controls[i].cells?.[j]?.node ?? null}</td>`)}
    </tr>`)}</tbody>
  </table>`;
}
