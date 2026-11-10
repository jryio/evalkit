// Pick one row of another artifact (e.g. the first wrong step of a transcript).
// In the controls card it shows the picked row; on each row it is a small toggle.
import { html } from "../html.js";
import { cx } from "../ui/bits.js";
import { artifactsOf } from "../core/derive.js";

export function rowName(a, row) {
  const i = a.rows.indexOf(row);
  return row.title ?? `Step ${i + 1}${row.name ? ` · ${row.name}` : ""}`;
}

export default function Pick({ q, item, value, set, row }) {
  if (row !== undefined) {
    const on = value === row;
    return html`<button class=${cx("radio", on && "on")} role="radio" aria-checked=${on} aria-label=${q.label} title=${q.label}
      onClick=${() => set(on ? undefined : row)}></button>`;
  }
  const a = artifactsOf(item).find(x => x.id === q.from);
  const picked = a?.rows.find(r => r.id === value);
  return html`<span class=${cx("pick-value", !picked && "muted")}>${picked ? rowName(a, picked) : "None picked"}</span>`;
}

Pick.rowPlacement = "lead";
Pick.labelKey = true;
Pick.note = ({ q, value }) => (value === undefined && q.key ? `Move with ↑ ↓, then press ${q.key.toUpperCase()}` : null);
