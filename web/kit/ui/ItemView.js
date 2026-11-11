// One item: header, the artifacts under review, the controls card, text questions,
// then collapsed context. The layout is fixed; the suite only supplies content.
import { html } from "../html.js";
import { controlFor, rendererFor } from "../registry.js";
import { cx, tone, Key, StatusTag, Boundary, Icon } from "./bits.js";
import { cell } from "../core/table.js";
import { questionsFor, answersOf, editsOf, valueOf, asked } from "../core/derive.js";

const isSection = q => controlFor(q.kind)?.layout === "section";
const rowProposal = (p, row) => (p?.value?.[row] !== undefined ? { value: p.value[row], why: p.why } : undefined);

export function ItemView({ suite, cols, item, state, act, cursor }) {
  const questions = questionsFor(suite, item);
  const answers = answersOf(state, item);
  const edits = editsOf(state, item);

  // A per-row answer is a { rowId: value } map, except pick, whose answer is one row id.
  const props = (q, row) => {
    const perRow = row !== undefined && q.kind !== "pick";
    return {
      q, item, row,
      value: valueOf(state, item, q, perRow ? row : undefined),
      answer: perRow ? answers[q.id]?.[row] : answers[q.id],
      proposed: perRow ? rowProposal(item.proposed?.[q.id], row) : item.proposed?.[q.id],
      set: v => act.answer(item, q.id, v, perRow ? row : undefined),
    };
  };
  const control = (q, extra = {}) => {
    const C = controlFor(q.kind);
    return C ? html`<${C} ...${props(q, extra.row)} ...${extra} />` : html`<div class="artifact-error">No control for kind "${q.kind}".</div>`;
  };
  // rowControls(row) -> { lead, below, cells }: per-row controls for a row renderer. below is
  // labeled for stacked layouts; cells are unlabeled, compact controls for table columns.
  const rowControls = a => {
    const qs = questions.filter(q => q.each === a.id || q.from === a.id);
    return row => {
      if (!qs.length || row.ask === false) return { lead: null, below: null, cells: null };
      const live = q => asked(state, item, questions, q, row.id);
      const at = place => qs.filter(q => (controlFor(q.kind)?.rowPlacement ?? "below") === place);
      const lead = at("lead").filter(live).map(q => control(q, { row: row.id, key: q.id }));
      const below = at("below").filter(live).map(q => html`<span class="row-q" key=${q.id}><span class="row-q-label">${q.label}</span>${control(q, { row: row.id })}</span>`);
      // One cell per question on every row, so table columns line up when `when` hides one.
      const cells = at("below").map(q => ({ label: q.label, node: live(q) ? control(q, { row: row.id, compact: true }) : null }));
      return { lead: lead.length ? lead : null, below: below.length ? below : null, cells: cells.length ? cells : null };
    };
  };
  const artifact = (a, i, inContext) => html`<${Artifact} key=${a.id ?? i} a=${a} item=${item} inContext=${inContext}
    edit=${edits[a.id ?? "edit"]} setEdit=${t => act.edit(item, a, t)} rowControls=${rowControls(a)}
    answers=${answers} questions=${questions}
    cursor=${cursor && a.id !== undefined && cursor.artifact === a.id ? cursor.row : undefined} />`;

  const own = questions.filter(q => !q.each && asked(state, item, questions, q));
  const rows = own.filter(q => !isSection(q) && !q.with);
  const notes = own.filter(q => isSection(q) && q.scope === "suite");
  const itemTexts = own.filter(q => isSection(q) && q.scope === "item");

  return html`
    <${ItemHeader} item=${item} suite=${suite} cols=${cols} />
    ${item.show?.length ? html`<div class="show">${item.show.map((a, i) => artifact(a, i, false))}</div>` : null}
    ${rows.length ? html`<div class="card">${rows.map(q => html`<${CardRow} q=${q} control=${control}
        attached=${own.filter(o => o.with === q.id)} note=${controlFor(q.kind)?.note?.(props(q))} />`)}</div>` : null}
    ${notes.map(q => html`<div class="section" key=${q.id}>
      <div class="section-title">${q.label}${q.key ? html`<${Key} k=${q.key} />` : null}</div>
      ${control(q)}
    </div>`)}
    ${itemTexts.length ? html`<div class="section">
      <div class="section-title">Questions</div>
      ${itemTexts.map(q => html`<div class="question" key=${q.id}><p>${q.label}</p>${control(q, { compact: true })}</div>`)}
    </div>` : null}
    ${item.context?.length ? html`<div class="disclosures">${item.context.map((a, i) => html`<details key=${a.id ?? i}>
      <summary>${a.label ?? a.kind}${a.note ? html`<span class="muted">${a.note}</span>` : null}</summary>
      <div class="details-body">${artifact(a, i, true)}</div>
    </details>`)}</div>` : null}`;
}

function ItemHeader({ item, suite, cols }) {
  return html`
    ${item.badges?.length || item.meta || item.flag ? html`<div class="eyebrow">
      ${item.flag ? html`<span class="badge tone-warning">⚑ Flagged by the agent</span>` : null}
      ${(item.badges || []).map(b => html`<span class=${cx("badge", tone(b.tone))}>${b.label}</span>`)}
      ${item.meta ? html`<span>${item.meta}</span>` : null}
    </div>` : null}
    <h1 class=${item.flag || item.values ? "tight" : ""}>${item.title}</h1>
    ${item.flag ? html`<p class="flag-reason">${item.flag}</p>` : null}
    <${Values} item=${item} suite=${suite} cols=${cols} />`;
}

// Values shows the item's read-only measurements, with the change from the baseline.
function Values({ item, suite, cols: all }) {
  if (!item.values) return null;
  const cols = all.filter(c => c.kind === "value" && item.values[c.def.id] !== undefined);
  return html`<dl class="values">${cols.map(col => {
    const c = cell(col, item, {}, suite);
    const d = all.find(x => x.kind === "delta" && x.of === col);
    const delta = d && cell(d, item, {}, suite);
    return html`<div class="value" key=${col.id}>
      <dt>${col.label}</dt>
      <dd>${c.icon ? html`<${Icon} name=${c.icon} /> ` : null}${c.text || "—"}${delta?.text ? html` <span class=${cx("delta", tone(delta.tone))}>${delta.text}</span>` : null}</dd>
    </div>`;
  })}</dl>`;
}

function CardRow({ q, control, attached, note }) {
  return html`<div class="card-row">
    <div class="row-label">${q.label}${controlFor(q.kind)?.labelKey && q.key ? html`<${Key} k=${q.key} />` : null}</div>
    ${control(q)}
    ${attached.length ? html`<div class="row-extra">${attached.map(o => control(o, { inline: true, key: o.id }))}</div>` : note ? html`<div class="row-note">${note}</div>` : null}
  </div>`;
}

// Artifact draws the panel header (label, renderer extras, note, status) and the
// registered renderer. In context the disclosure summary is the header.
function Artifact({ a, item, edit, setEdit, rowControls, cursor, inContext, answers, questions }) {
  const R = rendererFor(a.kind);
  if (!R) return html`<div class="artifact-error">No renderer for kind "${a.kind}".</div>`;
  const body = html`<${Boundary} label=${a.label ?? a.kind}>
    <${R} a=${a} item=${item} edit=${edit} setEdit=${setEdit} rowControls=${rowControls} cursor=${cursor} inContext=${inContext}
      answers=${answers} questions=${questions} />
  </${Boundary}>`;
  if (inContext) return html`<div data-artifact=${a.id}>${body}</div>`;
  return html`<div class=${a.width === "half" ? "half" : ""} data-artifact=${a.id}>
    ${a.label || a.note || a.status ? html`<div class="panel-head">
      ${a.label ? html`<span class="panel-title">${a.label}</span>` : null}
      ${R.head?.({ a, edit }) ?? null}
      ${a.note ? html`<span class="muted">${a.note}</span>` : null}
      ${a.status ? html`<${StatusTag} tag=${a.status} />` : null}
    </div>` : null}
    ${body}
  </div>`;
}
