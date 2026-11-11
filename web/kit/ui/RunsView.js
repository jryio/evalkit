// The Runs view: every run of a suite on the table page's grid. Tiles describe the selected
// run, charts show every run, and a table of numbers lists the runs newest first. Click a
// run to select it; double-click or Enter opens it in the table.
import { html, useEffect, useRef } from "../html.js";
import { cx, tone, fmtDate, runWhen } from "./bits.js";
import { SearchField, Segments } from "./ItemsTable.js";
import { RunTrends, RunPairs } from "./RunCharts.js";
import { markedBaseline } from "../core/runs.js";
import { autoDecimals, deltaTone, fmtNumber, signed, withUnit } from "../core/format.js";

const NONE = html`<span class="rv-none">—</span>`;
const isDate = v => /^\d{4}-\d\d-\d\d/.test(String(v));

// moves are the paired counts: better first. A lower-is-better metric improves by going down.
function moves(metric) {
  const lower = metric.better === "lower";
  return [
    { key: "up", glyph: lower ? "↓" : "↑", word: lower ? "lower" : "up", tone: "positive" },
    { key: "same", glyph: "=", word: "same", tone: "neutral" },
    { key: "down", glyph: lower ? "↑" : "↓", word: lower ? "higher" : "down", tone: "negative" },
  ];
}

// formats prints scores and spend as the items table does; a paired change whose interval
// crosses zero is no movement.
function formats(metric, spend, rows) {
  const d = metric?.decimals ?? 2;
  const sd = spend?.decimals ?? autoDecimals(rows.map(r => r.spend?.value));
  return {
    score: v => withUnit(fmtNumber(v, d), metric.unit),
    ci: ({ lo, hi }) => withUnit(`${fmtNumber(lo, d)}–${fmtNumber(hi, d)}`, metric.unit),
    scoreDelta: v => withUnit(signed(v, d), metric.unit),
    prevTone: v => deltaTone(v, metric),
    vsTone: ci => (ci.lo <= 0 && ci.hi >= 0 ? "neutral" : deltaTone(ci.value, metric)),
    spend: v => withUnit(fmtNumber(v, sd), spend.unit),
    spendDelta: v => withUnit(signed(v, sd), spend.unit),
    spendTone: v => deltaTone(v, spend),
  };
}

// Delta is a signed change: ▲ or ▼ say which way the number moved, the tone whether that is better.
function Delta({ value, tone: t, fmt }) {
  if (value === null || value === undefined) return NONE;
  return html`<span class=${cx("delta", tone(t))}>${value > 0 ? "▲ " : value < 0 ? "▼ " : ""}${fmt(value)}</span>`;
}

// varying is the dimensions whose value differs between runs: the declared ones, else every key of dims.
function varying(suite, runs) {
  const dims = suite.dimensions ?? [...new Set(runs.flatMap(r => Object.keys(r.dims)))].map(id => ({ id }));
  return dims.filter(d => new Set(runs.map(r => String(r.dims[d.id] ?? ""))).size > 1);
}

function best(rows, metric) {
  const scored = rows.filter(r => r.score);
  const sign = metric?.better === "lower" ? -1 : 1;
  return scored.length > 1 ? scored.reduce((a, b) => (sign * (b.score.value - a.score.value) > 0 ? b : a)) : null;
}

export function RunsView({ suite, state, runs, rows, shown, metric, spend, filters, setFilters, selected, vs, select, open, banners }) {
  const row = rows.find(r => r.run.id === selected);
  const f = formats(metric, spend, rows);
  const marked = markedBaseline(suite, runs);
  const facts = [`${runs.length} ${runs.length === 1 ? "run" : "runs"}`, metric && `${metric.label}, ${metric.better} is better`, `vs ${marked ? marked.label : "the run before"}`];
  const charts = { suite, state, judgeOnly: filters.judgeOnly, group: filters.group, rows, metric, spend, runs, selected, vs, select, open };

  // Keys move the selection through the table; keep it in view, but leave the page at the top on arrival.
  const arrived = useRef(false);
  useEffect(() => {
    if (arrived.current) document.querySelector(`tr[data-run="${CSS.escape(selected)}"]`)?.scrollIntoView({ block: "nearest" });
    arrived.current = true;
  }, [selected]);

  return html`<div class="tv">
    <div class="tv-head">
      ${banners}
      <h1>${suite.title}</h1>
      ${suite.description ? html`<p class="tv-desc">${suite.description}</p>` : null}
      <p class="tv-run">${facts.filter(Boolean).join(" · ")}</p>
      <div class="tv-bar">
        <${SearchField} placeholder="Filter runs" focus="runs-search" value=${filters.search} set=${search => setFilters({ ...filters, search })} />
        ${suite.groups?.length ? html`<${Segments} label="Group" value=${filters.group} set=${group => setFilters({ ...filters, group })}
          options=${[{ value: null, label: "All groups" }, ...suite.groups.map(g => ({ value: g.id, label: g.label ?? g.id }))]} />` : null}
        ${metric?.source === "question" ? html`<${Segments} label="Answers" value=${filters.judgeOnly} set=${judgeOnly => setFilters({ ...filters, judgeOnly })}
          options=${[{ value: false, label: "Reviewed", tip: "Your answers where you gave one, else the judge's" }, { value: true, label: "Judge only", tip: "The judge's answers only" }]} />` : null}
        <div class="spacer"></div>
        ${filters.search ? html`<span class="tv-count">${shown.length} of ${rows.length}</span>` : null}
      </div>
    </div>
    <div class="tv-scroll rv-body">
      ${row ? html`<${RunTiles} row=${row} prev=${rows[row.run.index - 1]} count=${runs.length} metric=${metric} spend=${spend} f=${f} open=${open} />` : null}
      <div class="rv-band"><${RunTrends} ...${charts} /></div>
      <${RunsTable} suite=${suite} runs=${runs} rows=${rows} shown=${shown} metric=${metric} spend=${spend} f=${f} marked=${marked}
        selected=${selected} select=${select} open=${open} clear=${() => setFilters({ ...filters, search: "" })} />
      <div class="rv-band"><${RunPairs} ...${charts} /></div>
    </div>
  </div>`;
}

// RunTiles are the selected run's cards: the run, its score, its paired moves and its spend.
function RunTiles({ row, prev, count, metric, spend, f, open }) {
  const { run, vs, vsBase } = row;
  const when = runWhen(run);
  // When the compared run is the one before, its paired change already says how far the score moved.
  const showPrev = row.vsPrev && !(vsBase && vs?.id === prev?.run.id);
  return html`<div class="rv-tiles">
    <div class="rv-tile">
      <div class="rv-label">${run.index === count - 1 ? "Latest run" : `Run ${run.index + 1} of ${count}`}</div>
      <div class="rv-big">${run.label}</div>
      ${when ? html`<div class="rv-sub">${when}</div>` : null}
      ${run.note ? html`<div class="rv-sub">${run.note}</div>` : null}
    </div>
    ${metric ? html`<div class="rv-tile">
      <div class="rv-label">${metric.label}</div>
      <div class="rv-line">
        <span class="rv-big">${row.score ? f.score(row.score.value) : "—"}</span>
        ${row.score ? html`<span class="rv-sub">95% CI ${f.ci(row.score)}</span>` : null}
      </div>
      <div class="rv-line rv-sub">
        ${showPrev ? html`<span><${Delta} value=${row.vsPrev.delta} tone=${f.prevTone(row.vsPrev.delta)} fmt=${f.scoreDelta} /> vs ${prev.run.label}</span>` : null}
        ${vsBase ? html`<span><${Delta} value=${vsBase.delta.value} tone=${f.vsTone(vsBase.delta)} fmt=${f.scoreDelta} /> vs ${vs.label}</span>` : null}
      </div>
    </div>
    <div class="rv-tile">
      <div class="rv-label">${vs ? `${run.label} vs ${vs.label}${vsBase ? ` · ${vsBase.paired} of ${row.items} paired` : ""}` : run.baseline ? `${run.label} is the baseline` : "Comparison"}</div>
      ${vsBase ? html`<div class="rv-moves">
          ${moves(metric).map(m => html`<button class=${cx("rv-move", tone(m.tone))} disabled=${!vsBase[m.key]}
              data-tip="Open the table on these items" onClick=${() => open(run.id, { direction: m.key })}>
            ${m.glyph} ${vsBase[m.key]} ${m.word}</button>`)}
        </div>`
        : html`<div class="rv-sub">${vs ? `No items in common with ${vs.label}` : "No comparison"}</div>`}
    </div>` : null}
    ${spend ? html`<div class="rv-tile">
      <div class="rv-label">${spend.label}</div>
      <div class="rv-big">${row.spend ? f.spend(row.spend.value) : "—"}</div>
      ${row.spendPrev !== null ? html`<div class="rv-sub"><${Delta} value=${row.spendPrev} tone=${f.spendTone(row.spendPrev)} fmt=${f.spendDelta} /> vs ${prev.run.label}</div>` : null}
    </div>` : null}
  </div>`;
}

// RunsTable is one row per run, newest first, in the items table's style. Columns that
// would be empty for every run are left out.
function RunsTable({ suite, runs, rows, shown, metric, spend, f, marked, selected, select, open, clear }) {
  const top = best(rows, metric);
  const pairs = rows.some(r => r.vsBase);
  const cols = [
    ...varying(suite, runs).map(d => ({ id: `dim:${d.id}`, label: d.label ?? d.id, left: true, cell: r => {
      const v = r.run.dims[d.id];
      return v === undefined ? "" : isDate(v) ? fmtDate(v) : String(v);
    } })),
    { id: "note", label: "Note", left: true, cls: "c-note", cell: r => html`<span class="txt" title=${r.run.note}>${r.run.note ?? ""}</span>` },
    pairs && { id: "paired", label: "Paired", cell: r => (r.vsBase ? `${r.vsBase.paired}/${r.items}` : NONE) },
    metric && { id: "score", label: metric.label, cell: r => (r.score ? f.score(r.score.value) : NONE) },
    metric && { id: "ci", label: "95% CI", cell: r => (r.score ? f.ci(r.score) : NONE) },
    metric && { id: "prev", label: "Δ prev", tip: "Change from the run before", cell: r => html`<${Delta} value=${r.vsPrev?.delta} tone=${r.vsPrev && f.prevTone(r.vsPrev.delta)} fmt=${f.scoreDelta} />` },
    pairs && { id: "vs", label: `Δ ${marked ? marked.label : "paired"}`, tip: `Item by item, against ${marked ? marked.label : "the run before"}`,
      cell: r => html`<${Delta} value=${r.vsBase?.delta.value} tone=${r.vsBase && f.vsTone(r.vsBase.delta)} fmt=${f.scoreDelta} />` },
    ...(pairs ? moves(metric).map(m => ({ id: m.key, label: m.glyph, tip: `Items ${m.word === "same" ? "that stayed the same" : `that moved ${m.word}`}`, cell: r => r.vsBase?.[m.key] ?? NONE })) : []),
    spend && { id: "spend", label: spend.label, cell: r => (r.spend ? f.spend(r.spend.value) : NONE) },
    spend && { id: "spendPrev", label: "Δ prev", tip: `${spend.label}: change from the run before`, cell: r => html`<${Delta} value=${r.spendPrev} tone=${r.spendPrev !== null && f.spendTone(r.spendPrev)} fmt=${f.spendDelta} />` },
  ].filter(Boolean);

  return html`<div class="rv-table-wrap">
    <table class="items-table rv-table">
      <thead><tr>
        <th class="c-state"></th>
        <th class="c-item"><span class="th-label">Run</span></th>
        ${cols.map(col => html`<th key=${col.id} class=${cx(!col.left && "right")}><span class="th-label" title=${col.tip}>${col.label}</span></th>`)}
        <th class="c-open"></th>
      </tr></thead>
      <tbody>
        ${shown.map(r => html`<tr key=${r.run.id} class=${cx(r.run.id === selected && "on")} data-run=${r.run.id} aria-selected=${r.run.id === selected}
            onClick=${() => select(r.run.id)} onDblClick=${() => open(r.run.id)}>
          <td class="c-state"></td>
          <td class="c-item"><span class="ti-title">${r.run.label}</span>${r === top ? html`<span class="ti-sub">best</span>` : null}</td>
          ${cols.map(col => html`<td key=${col.id} class=${cx(col.cls, !col.left && "right")}>${col.cell(r)}</td>`)}
          <td class="c-open"><button class="open-btn" aria-label=${`Open ${r.run.label} in the table`} onClick=${e => { e.stopPropagation(); open(r.run.id); }}>›</button></td>
        </tr>`)}
        ${shown.length ? null : html`<tr class="tv-empty"><td colspan=${cols.length + 3}>No runs match. <button class="link" onClick=${clear}>Clear filter</button></td></tr>`}
      </tbody>
    </table>
  </div>`;
}
