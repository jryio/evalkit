// A live tally of a per-row question's answers (with the agent's proposals as
// defaults): counts, or percentages when a stat has `of`. E.g. precision and recall.
import { html } from "../html.js";
import { artifactsOf } from "../core/derive.js";

export default function Tally({ a, item, answers, questions }) {
  const q = questions.find(x => x.id === a.question);
  const rows = artifactsOf(item).find(x => x.id === q?.each)?.rows;
  if (!rows) return html`<div class="artifact-error">Tally: "${a.question}" is not a per-row question on this item.</div>`;
  const proposed = item.proposed?.[q.id]?.value ?? {};
  const values = rows.map(r => answers[q.id]?.[r.id] ?? proposed[r.id]);
  const count = list => values.filter(v => list.includes(v)).length;
  return html`<div class="tiles">${a.stats.map(s => {
    const n = count(s.count);
    const of = s.of === "rows" ? rows.length : s.of ? count(s.of) : undefined;
    const value = of === undefined ? String(n) : of === 0 ? "—" : `${Math.round((100 * n) / of)}%`;
    return html`<div class="tile">
      <div class="tile-label">${s.label}</div>
      <div class="tile-value">${value}</div>
      <div class="tile-sub">${of === undefined ? `of ${rows.length} rows` : `${n} of ${of}`}</div>
    </div>`;
  })}</div>`;
}
