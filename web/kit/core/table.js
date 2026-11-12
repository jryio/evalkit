// The items table as data: which columns a suite gets, what each cell shows, a profile
// of each column (mean and histogram, or option shares), filters and sorting. No DOM.
import { answersOf, changes, doneOf, optionOf, valueOf } from "./derive.js";
import { entry, hasInput } from "./state.js";
import { autoDecimals, deltaTone, fmtNumber, signed, stepDecimals, withUnit } from "./format.js";

const QUESTION_KINDS = new Set(["choice", "multi", "toggle", "scale", "number", "action"]);
const isNum = v => typeof v === "number" && Number.isFinite(v);
const bounded = (lo, hi) => (isNum(lo) && isNum(hi) && lo < hi ? [lo, hi] : null);
const boxed = v => (v !== null && typeof v === "object" ? v : { value: v });
const TONE_RANK = { negative: 0, warning: 1, neutral: 2, positive: 3 };

// columns lists the table's columns after the item column: group, then every suite-level
// question a cell can show, then the suite's value columns, each followed by a Δ column
// when some item has a baseline, then one note column for the suite's text questions.
export function columns(suite) {
  const items = suite.items;
  const out = [];
  if (suite.groups?.length) out.push({ id: "group", kind: "group", label: "Group", type: "text" });
  for (const q of suite.questions || []) {
    if (q.each || !QUESTION_KINDS.has(q.kind)) continue;
    const col = questionColumn(q, items);
    out.push(col);
    if (items.some(i => baselineOf(col, i) !== undefined)) out.push(deltaColumn(col));
  }
  for (const c of suite.columns || []) {
    const col = valueColumn(c, items);
    out.push(col);
    if (items.some(i => baselineOf(col, i) !== undefined)) out.push(deltaColumn(col));
  }
  const notes = (suite.questions || []).filter(q => q.kind === "text" && !q.each);
  if (notes.length) out.push({ id: "note", kind: "note", label: notes.length === 1 ? notes[0].label : "Notes", type: "text", questions: notes });
  return out;
}

function questionColumn(q, items) {
  const base = { id: `q:${q.id}`, kind: "question", q, label: q.label ?? q.id };
  if (q.kind === "scale") return { ...base, type: "number", align: "right", decimals: 0, range: bounded(q.min ?? 1, q.max ?? 5), integer: true };
  // Without a step, a human answer may be finer than the proposals: print up to three decimals.
  if (q.kind === "number") return { ...base, type: "number", align: "right", unit: q.unit, decimals: stepDecimals(q.step), range: bounded(q.min, q.max) };
  if (q.kind === "choice") return { ...base, type: "options", options: q.options, iconsOnly: q.options.every(o => o.icon) };
  if (q.kind === "toggle") return { ...base, type: "options", options: [{ value: true, label: "Yes", icon: "check" }] };
  return { ...base, type: "text" };
}

function valueColumn(c, items) {
  const values = items.map(i => boxed(i.values?.[c.id]).value).filter(v => v !== null && v !== undefined);
  const base = { id: `v:${c.id}`, kind: "value", def: c, label: c.label ?? c.id, unit: c.unit, better: c.better, noise: c.noise };
  if (values.length && values.every(isNum)) {
    return { ...base, type: "number", align: "right", decimals: c.decimals ?? autoDecimals(values), range: bounded(c.min, c.max) };
  }
  if (values.length && values.every(v => typeof v === "boolean")) {
    const t = good => (c.better ? ((c.better === "higher") === good ? "positive" : "negative") : undefined);
    return { ...base, type: "options", options: [{ value: true, label: "Yes", icon: "check", tone: t(true) }, { value: false, label: "No", icon: "cross", tone: t(false) }], iconsOnly: true };
  }
  return { ...base, type: "text" };
}

function deltaColumn(of) {
  return { id: `d:${of.id}`, kind: "delta", of, label: "Δ", type: of.type === "number" ? "number" : "text", align: "right", decimals: of.decimals, better: of.better ?? (of.q ? "higher" : undefined), noise: of.noise };
}

function baselineOf(col, item) {
  if (col.kind === "question") return item.proposed?.[col.q.id]?.baseline ?? undefined;
  if (col.kind === "value") return boxed(item.values?.[col.def.id]).baseline ?? undefined;
  return undefined;
}

function rawValue(col, item, state, suite) {
  switch (col.kind) {
    case "group": return suite.groups.find(g => g.id === item.group)?.label ?? item.group;
    case "question": return valueOf(state, item, col.q);
    case "value": return boxed(item.values?.[col.def.id]).value;
    case "note": {
      const answers = answersOf(state, item);
      return col.questions.map(q => answers[q.id] ?? item.proposed?.[q.id]?.value).find(v => typeof v === "string" && v.trim());
    }
  }
}

// cell is what one column shows for one item:
//   { value, text, tone, icon, sort, human, from }
// human: the human answered this question; from: the proposal they overrode.
export function cell(col, item, state, suite) {
  if (col.kind === "delta") return deltaCell(col, item, state, suite);
  const value = rawValue(col, item, state, suite);
  const out = { value, text: "", sort: undefined };
  if (col.kind === "question") {
    const answer = answersOf(state, item)[col.q.id];
    const proposed = item.proposed?.[col.q.id];
    out.human = answer !== undefined;
    if (out.human && proposed?.value !== undefined && JSON.stringify(proposed.value) !== JSON.stringify(answer)) out.from = display(col, proposed.value).text;
    if (col.q.kind === "action") return { ...out, ...actionCell(col.q, item, answer) };
    if (col.q.kind === "toggle" && value !== true) return out; // false is a turned-off proposal
  }
  if (value === undefined || value === null) return out;
  return { ...out, ...display(col, value) };
}

// display formats a value for a column: { text, tone, icon, sort }.
function display(col, value) {
  if (col.type === "number" && isNum(value)) return { text: withUnit(fmtNumber(value, col.decimals), col.unit), sort: value };
  if (col.type === "options") {
    const i = col.options.findIndex(o => o.value === value);
    const o = col.options[i];
    return o ? { text: o.label, tone: o.tone, icon: o.icon, sort: i } : { text: String(value), sort: col.options.length };
  }
  if (Array.isArray(value)) {
    const labels = value.map(v => optionOf(col.q, v)?.label ?? String(v));
    return { text: labels.join(", "), sort: labels.join(", ").toLowerCase() };
  }
  const text = String(value);
  return { text: col.kind === "note" ? text.trim().split("\n")[0] : text, sort: text.toLowerCase() };
}

function actionCell(q, item, answer) {
  if (doneOf(item, q.id)) return { text: q.states.done?.badge ?? "Done", sort: 2, tone: "neutral" };
  if (answer === true) return { text: q.states.on?.badge ?? "Approved", sort: 1, tone: "positive" };
  return { text: "", sort: 0 };
}

function deltaCell(col, item, state, suite) {
  const base = baselineOf(col.of, item);
  const value = rawValue(col.of, item, state, suite);
  if (base === undefined || base === null || value === undefined || value === null) return { text: "" };
  if (isNum(value) && isNum(base)) {
    const delta = value - base;
    const tone = deltaTone(delta, col);
    return { value: delta, text: signed(delta, col.decimals ?? 2), tone, sort: delta, base };
  }
  if (JSON.stringify(value) === JSON.stringify(base)) return { value: 0, text: "", sort: 0, base };
  const was = display(col.of, base);
  const now = display(col.of, value);
  const rank = t => TONE_RANK[t] ?? TONE_RANK.neutral;
  const tone = rank(now.tone) > rank(was.tone) ? "positive" : rank(now.tone) < rank(was.tone) ? "negative" : "neutral";
  return { value: 1, text: `was ${was.text}`, tone, sort: tone === "negative" ? -1 : 1, base };
}

// profile summarizes a column over items, for its header:
//   number: { type, n, mean, bins: [{ lo, hi, count, label }] }
//   options: { type, n, counts: [{ value, label, tone, icon, count }] }
//   delta: { type, n, mean?, better, worse }
//   text: { type, n, distinct }
export function profile(col, items, state, suite) {
  const cells = items.map(i => cell(col, i, state, suite)).filter(c => c.text !== "" && c.text !== undefined);
  const n = cells.length;
  if (col.kind === "delta") {
    const nums = cells.map(c => c.value).filter(isNum);
    return {
      type: "delta", n,
      mean: col.type === "number" && nums.length ? nums.reduce((a, b) => a + b, 0) / nums.length : undefined,
      better: cells.filter(c => c.tone === "positive").length,
      worse: cells.filter(c => c.tone === "negative").length,
    };
  }
  if (col.type === "number") {
    const values = cells.map(c => c.value).filter(isNum);
    if (!values.length) return { type: "number", n: 0, bins: [] };
    const range = col.range ?? [Math.min(...values), Math.max(...values)];
    if (!(range[0] <= range[1])) return { type: "number", n: values.length, mean: values.reduce((a, b) => a + b, 0) / values.length, bins: [] };
    return { type: "number", n: values.length, mean: values.reduce((a, b) => a + b, 0) / values.length, range, bins: histogram(values, range, col) };
  }
  if (col.type === "options") {
    const counts = col.options.map(o => ({ ...o, count: cells.filter(c => c.value === o.value).length }));
    return { type: "options", n, counts };
  }
  return { type: "text", n, distinct: new Set(cells.map(c => c.text)).size };
}

// histogram puts values in bins over range: one bin per whole number for small integer
// ranges (a 1–5 scale gets 5), else 10 equal bins. Values outside the range land in the end bins.
export function histogram(values, [lo, hi], col = {}) {
  const whole = (col.integer || values.every(Number.isInteger)) && Number.isInteger(lo) && Number.isInteger(hi) && hi - lo <= 12;
  const d = col.decimals ?? 2;
  const edges = [];
  if (whole) {
    for (let k = lo; k <= hi; k++) edges.push([k - 0.5, k + 0.5, String(k)]);
  } else {
    const n = hi > lo ? 10 : 1;
    const w = (hi - lo) / n;
    for (let k = 0; k < n; k++) edges.push([lo + k * w, lo + (k + 1) * w, `${fmtNumber(lo + k * w, d)}–${fmtNumber(lo + (k + 1) * w, d)}`]);
  }
  const bins = edges.map(([a, b, label], k) => ({ lo: a, hi: b, first: k === 0, last: k === edges.length - 1, label, count: 0 }));
  for (const v of values) bins.find(b => inBin(v, b)).count++;
  return bins;
}

export const inBin = (v, b) => isNum(v) && (b.first || v >= b.lo) && (b.last || v < b.hi);

// matches tests one column filter against an item. A filter is { column, bin } (a
// histogram bin), { column, value } (an option) or { column, tone } (a Δ direction).
export function matches(f, col, item, state, suite) {
  const c = cell(col, item, state, suite);
  if (f.bin) return inBin(c.value, f.bin);
  if (f.tone) return c.tone === f.tone;
  return c.value === f.value;
}

const SCOPES = {
  all: () => true,
  flagged: item => !!item.flag,
  input: (item, state) => hasInput(entry(state, item)),
  overrides: (item, state) => Object.keys(changes(state, item)).length > 0,
};

export const scopeCounts = (items, state) =>
  Object.fromEntries(Object.entries(SCOPES).map(([k, test]) => [k, items.filter(i => test(i, state)).length]));

const searchText = item => [item.id, item.title, item.short, item.subtitle, item.meta, item.flag].filter(Boolean).join(" ").toLowerCase();

// visible applies the scope, group, search and column filters, then sorts.
export function visible(suite, state, cols, view) {
  const { scope = "all", group, search = "", where = [], sort } = view;
  const terms = search.toLowerCase().split(/\s+/).filter(Boolean);
  const byId = new Map(cols.map(c => [c.id, c]));
  const rows = suite.items.filter(item =>
    SCOPES[scope](item, state) &&
    (!group || item.group === group) &&
    terms.every(t => searchText(item).includes(t)) &&
    // A filter on a column that a live reload removed no longer applies.
    where.every(f => !byId.has(f.column) || matches(f, byId.get(f.column), item, state, suite)));
  const col = sort && byId.get(sort.column);
  return col ? sortBy(rows, i => cell(col, i, state, suite).sort, sort.dir) : rows;
}

// sortBy is stable and keeps missing values last in either direction.
export function sortBy(rows, key, dir) {
  const keyed = rows.map((row, i) => ({ row, i, k: key(row) }));
  keyed.sort((a, b) => {
    const am = a.k === undefined || a.k === null, bm = b.k === undefined || b.k === null;
    if (am || bm) return am === bm ? a.i - b.i : am ? 1 : -1;
    const c = a.k < b.k ? -1 : a.k > b.k ? 1 : 0;
    return c * dir || a.i - b.i;
  });
  return keyed.map(k => k.row);
}

// firstDir: numbers sort high to low first, text A to Z (as in Finder).
export const firstDir = col => (col.type === "number" ? -1 : 1);
