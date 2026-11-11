// The table view: every item as a row, every judged or measured value as a column with
// a profile in its header. Filters and sort live in App, so review mode follows the
// same list. Click selects, double-click or Enter opens the item for review.
import { html, useEffect, useMemo } from "../html.js";
import { cx, tone, Icon, Dot, runWhen } from "./bits.js";
import { OpenIn } from "./chrome.js";
import { Histogram, ShareBar, Meter, sameBin } from "./minicharts.js";
import { cell, profile, visible, firstDir } from "../core/table.js";
import { isReviewed } from "../core/derive.js";
import { fmtNumber, signed, withUnit } from "../core/format.js";

export function ItemsTable({ suite, state, cols, rows, view, setView, selected, select, open, openIn, setOpenIn, counts, progress, banners, run }) {
  // A column's profile ignores the column's own filter, so its other bins stay visible.
  const profiles = useMemo(() => Object.fromEntries(cols.map(col => {
    const where = view.where.filter(f => f.column !== col.id);
    const items = view.where.length === where.length ? rows : visible(suite, state, cols, { ...view, where, sort: null });
    return [col.id, profile(col, items, state, suite)];
  })), [suite, state, cols, rows, view]);

  useEffect(() => {
    document.querySelector(`tr[data-id="${CSS.escape(selected ?? "")}"]`)?.scrollIntoView({ block: "nearest" });
  }, [selected]);

  // filterBy sets a column's filter; choosing the active one again clears it. A bin is
  // stored without its count, which changes as answers change.
  const filterBy = (col, f) => {
    const current = view.where.find(w => w.column === col.id);
    const same = current && (f.bin ? current.bin && sameBin(current.bin, f.bin) : f.tone ? current.tone === f.tone : current.value === f.value && !current.bin && !current.tone);
    const rest = view.where.filter(w => w.column !== col.id);
    const next = f.bin ? { bin: { lo: f.bin.lo, hi: f.bin.hi, first: f.bin.first, last: f.bin.last, label: f.bin.label } } : f;
    setView({ ...view, where: same ? rest : [...rest, { column: col.id, ...next }] });
  };
  const sortBy = col => {
    const s = view.sort?.column === col.id ? view.sort : null;
    const dir = !s ? firstDir(col) : s.dir === firstDir(col) ? -s.dir : null;
    setView({ ...view, sort: dir ? { column: col.id, dir } : null });
  };
  const filtered = view.scope !== "all" || view.group || view.search || view.where.length;

  return html`<div class="tv">
    <div class="tv-head">
      ${banners}
      <h1>${run ? `${suite.title} · ${run.run.label}` : suite.title}</h1>
      ${suite.description ? html`<p class="tv-desc">${suite.description}</p>` : null}
      ${run ? html`<p class="tv-run">${runFacts(run, suite.items.length)}</p>` : null}
      <div class="tv-progress">
        <div class="progress"><span style=${{ width: `${progress.total ? (100 * progress.reviewed) / progress.total : 0}%` }}></span></div>
        <span><strong>${progress.reviewed} of ${progress.total}</strong> ${progress.flagged ? "flagged " : ""}reviewed</span>
      </div>
      <${FilterBar} suite=${suite} cols=${cols} view=${view} setView=${setView} counts=${counts} shown=${rows.length} filtered=${filtered}
        openIn=${openIn} setOpenIn=${setOpenIn} />
    </div>
    <div class="tv-scroll">
      <table class="items-table">
        <thead><tr>
          <th class="c-state" aria-label="Review state"></th>
          <th class="c-item">
            <button class="th-label" onClick=${() => setView({ ...view, sort: null })}>Item</button>
            <div class="th-profile pf-text">${rows.length} ${rows.length === 1 ? "item" : "items"}</div>
          </th>
          ${cols.map(col => html`<${HeaderCell} key=${col.id} col=${col} p=${profiles[col.id]} sort=${view.sort?.column === col.id ? view.sort.dir : 0}
              active=${view.where.find(w => w.column === col.id)} sortBy=${() => sortBy(col)} filterBy=${f => filterBy(col, f)} total=${rows.length} />`)}
          <th class="c-open"></th>
        </tr></thead>
        <tbody>
          ${rows.map(item => html`<${Row} key=${item.id} item=${item} cols=${cols} suite=${suite} state=${state}
              on=${item.id === selected} select=${select} open=${open} />`)}
          ${rows.length ? null : html`<tr class="tv-empty"><td colspan=${cols.length + 3}>
            No items match.${filtered ? html` <button class="link" onClick=${() => setView({ ...view, scope: "all", group: null, search: "", where: [] })}>Clear filters</button>` : null}
          </td></tr>`}
        </tbody>
      </table>
    </div>
  </div>`;
}

// An entry without a value only carries a baseline.
const hasProposal = item => Object.values(item.proposed || {}).some(p => p.value !== undefined);

// runFacts is the run's line under the title: when, its note, its items and how many pair with the compared run.
function runFacts({ run, vs, paired }, n) {
  const count = `${n} ${n === 1 ? "item" : "items"}${vs ? `, ${paired} paired with ${vs.label}` : ""}`;
  return [runWhen(run), run.note, count].filter(Boolean).join(" · ");
}

function FilterBar({ suite, cols, view, setView, counts, shown, filtered, openIn, setOpenIn }) {
  const scopes = [
    { id: "all", label: "All" },
    counts.flagged ? { id: "flagged", label: "Flagged", tip: "Rows the agent flagged for you" } : null,
    { id: "input", label: "Your input" },
    suite.items.some(hasProposal) ? { id: "overrides", label: "Overrides" } : null,
  ].filter(Boolean);
  const chips = view.where.map(f => {
    const col = cols.find(c => c.id === f.column);
    return col && { f, text: `${col.kind === "delta" ? `${col.of.label} Δ` : col.label}: ${filterLabel(col, f)}` };
  }).filter(Boolean);
  return html`<div class="tv-bar">
    <${SearchField} placeholder="Filter items" focus="table-search" value=${view.search} set=${search => setView({ ...view, search })} />
    <${Segments} label="Show" value=${view.scope} set=${scope => setView({ ...view, scope })}
      options=${scopes.map(s => ({ value: s.id, label: s.label, count: counts[s.id], tip: s.tip }))} />
    ${suite.groups?.length ? html`<${Segments} label="Group" value=${view.group ?? null} set=${group => setView({ ...view, group })}
      options=${[{ value: null, label: "All groups" }, ...suite.groups.map(g => ({ value: g.id, label: g.label ?? g.id }))]} />` : null}
    ${chips.map(({ f, text }) => html`<button class="chip small on" key=${f.column} title="Remove this filter"
        onClick=${() => setView({ ...view, where: view.where.filter(w => w !== f) })}>${text} <span aria-hidden="true">✕</span></button>`)}
    <div class="spacer"></div>
    ${filtered ? html`<span class="tv-count">${shown} of ${suite.items.length}</span>` : null}
    <span class="toolbar-label">Open rows in</span>
    <${OpenIn} value=${openIn} set=${setOpenIn} />
  </div>`;
}

// SearchField is a filter field that / focuses (focus is its data-focus id).
export function SearchField({ placeholder, focus, value, set }) {
  return html`<label class="tv-search">
    <svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="7" cy="7" r="4.75" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M10.5 10.5 14 14" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
    <input type="search" placeholder=${placeholder} value=${value} data-focus=${focus} aria-label=${placeholder}
      onInput=${e => set(e.currentTarget.value)} />
    <kbd>/</kbd>
  </label>`;
}

export function Segments({ label, value, set, options }) {
  return html`<div class="segmented small" role="radiogroup" aria-label=${label}>
    ${options.map(o => html`<button class=${cx("seg", value === o.value && "on")} role="radio" aria-checked=${value === o.value}
        title=${o.tip} onClick=${() => set(o.value)}>${o.label}${o.count !== undefined ? html`<span class="seg-count">${o.count}</span>` : null}</button>`)}
  </div>`;
}

function filterLabel(col, f) {
  if (f.bin) return f.bin.label;
  if (f.tone) return { positive: "better", negative: "worse", neutral: "same" }[f.tone];
  return col.options?.find(o => o.value === f.value)?.label ?? String(f.value);
}

function HeaderCell({ col, p, sort, active, sortBy, filterBy, total }) {
  const arrow = sort ? html`<span class="sort-arrow" aria-hidden="true">${sort > 0 ? "↑" : "↓"}</span>` : null;
  return html`<th class=${cx(`c-${col.type}`, col.kind === "delta" && "c-delta", col.align === "right" && "right")}
      aria-sort=${sort > 0 ? "ascending" : sort < 0 ? "descending" : undefined}>
    <button class="th-label" title=${col.kind === "delta" ? `${col.of.label}: change from the baseline` : col.label} onClick=${sortBy}>
      ${col.align === "right" ? arrow : null}${col.label}${col.align === "right" ? null : arrow}
    </button>
    <div class="th-profile">${Profile({ col, p, active, filterBy, total })}</div>
  </th>`;
}

function Profile({ col, p, active, filterBy, total }) {
  if (!p.n) return html`<span class="pf-text">—</span>`;
  if (p.type === "number") {
    return html`<${Histogram} bins=${p.bins} label=${col.label} active=${active?.bin} on=${bin => filterBy({ bin })} />
      <span class="pf-text">avg ${withUnit(fmtNumber(p.mean, Math.max(1, col.decimals ?? 0)), col.unit)}</span>`;
  }
  if (p.type === "options") {
    const first = p.counts[0];
    return html`<${ShareBar} counts=${p.counts} label=${col.label} active=${active?.value} on=${value => filterBy({ value })} />
      <span class="pf-text">${Math.round((100 * first.count) / p.n)}% ${first.label.toLowerCase()}</span>`;
  }
  if (p.type === "delta") {
    return html`<span class="pf-delta">
      <button class=${cx("pf-dir tone-positive", active?.tone === "positive" && "on")} disabled=${!p.better}
        data-tip=${`${p.better} better than the baseline`} onClick=${() => filterBy({ tone: "positive" })}>▲ ${p.better}</button>
      <button class=${cx("pf-dir tone-negative", active?.tone === "negative" && "on")} disabled=${!p.worse}
        data-tip=${`${p.worse} worse than the baseline`} onClick=${() => filterBy({ tone: "negative" })}>▼ ${p.worse}</button>
    </span>
    ${p.mean !== undefined ? html`<span class="pf-text">avg ${signed(p.mean, Math.max(1, col.decimals ?? 0))}</span>` : null}`;
  }
  if (col.kind === "note") return html`<span class="pf-text">${p.n} with notes</span>`;
  if (col.kind === "question") return html`<span class="pf-text">${p.n} of ${total}</span>`;
  return html`<span class="pf-text">${p.distinct} ${p.distinct === 1 ? "value" : "values"}</span>`;
}

function Row({ item, cols, suite, state, on, select, open }) {
  const reviewed = isReviewed(suite, state, item);
  return html`<tr class=${cx(on && "on")} data-id=${item.id} aria-selected=${on}
      onClick=${() => select(item.id)} onDblClick=${() => open(item.id)}>
    <td class="c-state">
      ${reviewed ? html`<span class="trail-check" data-tip=${item.flag ? `Reviewed · the agent flagged it: ${item.flag}` : "Reviewed"}>✓</span>`
        : item.flag ? html`<span class="flag" data-tip=${`The agent flagged this: ${item.flag}`} aria-label=${`The agent flagged this: ${item.flag}`}>⚑</span>` : null}
    </td>
    <td class="c-item">
      <span class="ti-title">${item.short ?? item.title}</span>
      ${item.subtitle ? html`<span class="ti-sub">${item.subtitle}</span>` : null}
    </td>
    ${cols.map(col => html`<td key=${col.id} class=${cx(`c-${col.type}`, col.kind === "delta" && "c-delta", col.kind === "note" && "c-note", col.align === "right" && "right")}>
      <${Cell} col=${col} c=${cell(col, item, state, suite)} />
    </td>`)}
    <td class="c-open"><button class="open-btn" aria-label=${`Open ${item.title}`} onClick=${e => { e.stopPropagation(); open(item.id); }}>›</button></td>
  </tr>`;
}

function Cell({ col, c }) {
  if (!c.text) return null;
  const body = col.kind === "delta"
    ? html`<span class=${cx("delta", tone(c.tone))}>${c.text}</span>`
    : col.type === "options"
      ? html`<span class=${cx("opt", tone(c.tone))} data-tip=${col.iconsOnly ? c.text : undefined}>
          ${c.icon ? html`<${Icon} name=${c.icon} />` : c.tone ? html`<${Dot} tone=${c.tone} />` : null}
          ${col.iconsOnly && c.icon ? null : html`<span>${c.text}</span>`}
        </span>`
      : col.type === "number"
        ? html`<span class="num">${col.range ? html`<${Meter} value=${c.value} range=${col.range} />` : null}${c.text}</span>`
        : html`<span class="txt" title=${c.text}>${c.text}</span>`;
  return c.from !== undefined ? html`<span class="overridden" data-tip=${`You changed this from ${c.from}`}>${body}</span>` : body;
}
