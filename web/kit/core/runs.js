// Multi-run suites as data: the runs, the metric that ranks them, a summary per run with
// intervals and paired comparisons, and a one-run suite the items table can show. No DOM.
import { answersOf, optionOf, primaryOf, valueOf } from "./derive.js";
import { deltaTone, stepDecimals } from "./format.js";
import { bootstrapMean, mcnemar, wilson } from "./stats.js";

const isNum = v => typeof v === "number" && Number.isFinite(v);
const unbox = v => (v !== null && typeof v === "object" ? v.value : v);
const sum = xs => xs.reduce((a, b) => a + b, 0);
const pairOf = item => item.pair ?? item.id;
const TONE_SCORE = { positive: 1, warning: 0.5, negative: 0 };

// caseOf is the pair without its rep suffix: every rep of one case shares it.
function caseOf(item) {
  const pair = pairOf(item);
  const suffix = `.r${item.rep}`;
  return item.rep && pair.endsWith(suffix) ? pair.slice(0, -suffix.length) : pair;
}

// runList is the runs in run order: declared runs, then runs only items name, first seen first.
export function runList(suite) {
  const all = [...(suite.runs || [])];
  const known = new Set(all.map(r => r.id));
  for (const { run } of suite.items) {
    if (run === undefined || known.has(run)) continue;
    known.add(run);
    all.push({ id: run });
  }
  return all.map((r, index) => ({
    id: r.id, label: r.label ?? r.id, note: r.note, baseline: !!r.baseline, dims: r.dims || {}, values: r.values || {}, index,
  }));
}

export const latestRun = suite => runList(suite).at(-1)?.id ?? null;

// markedBaseline is the run the suite marks baseline: true, or null.
export const markedBaseline = (suite, runs) => runs.find(r => r.id === suite.runs?.find(d => d.baseline)?.id) ?? null;

// vsOf is the run another run is compared with: the marked baseline, else the run before.
export function vsOf(suite, runs, run) {
  const vs = markedBaseline(suite, runs) ?? runs[run.index - 1] ?? null;
  return vs?.id === run.id ? null : vs;
}

// findRuns is the rows whose run id, label or note has every search term, newest first.
export function findRuns(rows, search = "") {
  const terms = search.toLowerCase().split(/\s+/).filter(Boolean);
  const text = ({ run }) => [run.id, run.label, run.note].filter(Boolean).join(" ").toLowerCase();
  return rows.filter(r => terms.every(t => text(r).includes(t))).reverse();
}

// metricOf is what ranks runs: the primary question when it scores, else a bounded or directed column.
export function metricOf(suite) {
  const cols = suite.columns || [];
  const q = primaryOf(suite);
  if (q && !cols.some(c => c.primary) && ["choice", "toggle", "number", "scale"].includes(q.kind)) {
    const kind = q.kind === "choice" || q.kind === "toggle" ? "rate" : "mean";
    return { id: q.id, label: q.label ?? q.id, source: "question", kind, better: "higher", unit: q.unit, decimals: q.kind === "scale" ? 0 : stepDecimals(q.step), q };
  }
  const c = cols.find(c => c.primary) ?? cols.find(c => isNum(c.min) && isNum(c.max)) ?? cols.find(c => c.better);
  if (!c) return null;
  return { id: c.id, label: c.label ?? c.id, source: "column", kind: "mean", better: c.better ?? "higher", unit: c.unit, decimals: c.decimals, noise: c.noise };
}

export function spendOf(suite, metric = metricOf(suite)) {
  const cols = (suite.columns || []).filter(c => !(metric?.source === "column" && c.id === metric.id));
  const c = cols.find(c => c.unit === "$") ?? cols.find(c => /token/i.test(`${c.id} ${c.label ?? ""}`)) ?? cols.find(c => c.unit === "s");
  return c ? { id: c.id, label: c.label ?? c.id, unit: c.unit, better: "lower", decimals: c.decimals, noise: c.noise } : null;
}

// metricColumn is the items table's column for the metric; its Δ column is "d:" + that.
export const metricColumn = metric => `${metric.source === "question" ? "q" : "v"}:${metric.id}`;

// columnValue is the item's own value for a column, else its run's.
function columnValue(item, id, runsById) {
  const v = unbox(item.values?.[id]) ?? unbox(runsById.get(item.run)?.values?.[id]);
  return isNum(v) ? v : undefined;
}

function rateScore(q, v) {
  if (q.kind === "toggle") return v === true ? 1 : v === false ? 0 : undefined;
  const o = optionOf(q, v);
  return TONE_SCORE[o?.tone];
}

// itemValue is the item's metric value; undefined means the item does not count.
export function itemValue(state, item, metric, runsById, { judgeOnly = false } = {}) {
  if (metric.source === "column") return columnValue(item, metric.id, runsById);
  const v = judgeOnly ? item.proposed?.[metric.id]?.value : valueOf(state, item, metric.q);
  if (metric.kind === "rate") return rateScore(metric.q, v);
  return isNum(v) ? v : undefined;
}

// summarize is one row per run, in run order: its score with a 95% interval, its spend,
// a paired comparison with the run it is compared with (vsOf) and the change from the previous run.
export function summarize(suite, state, { judgeOnly = false, group = null, seed = 1 } = {}) {
  const runs = runList(suite);
  if (!runs.length) return [];
  const metric = metricOf(suite);
  const spend = spendOf(suite);
  const runsById = new Map(runs.map(r => [r.id, r]));
  const pool = suite.items.filter(i => !group || i.group === group);
  const itemsOf = run => pool.filter(i => i.run === run.id);
  const value = i => (metric ? itemValue(state, i, metric, runsById, { judgeOnly }) : undefined);
  const valuesOf = run => new Map(itemsOf(run).map(i => [pairOf(i), value(i)]));

  const rows = [];
  for (const run of runs) {
    const items = itemsOf(run);
    const scored = items.map(item => ({ item, v: value(item) })).filter(s => isNum(s.v));
    const prev = rows.at(-1);
    const vs = vsOf(suite, runs, run);
    const row = {
      run, vs, items: items.length, n: scored.length,
      score: scoreOf(metric, scored, seed),
      spend: spend ? runSpend(run, items, spend.id, runsById) : null,
    };
    row.vsBase = vs ? versusBase(items, scored, valuesOf(vs), metric, seed) : null;
    row.vsPrev = prev?.score && row.score ? { delta: row.score.value - prev.score.value } : null;
    row.spendPrev = prev?.spend && row.spend ? row.spend.value - prev.spend.value : null;
    rows.push(row);
  }
  return rows;
}

function scoreOf(metric, scored, seed) {
  const xs = scored.map(s => s.v);
  const reps = scored.some(s => s.item.rep > 0);
  if (metric?.kind === "rate" && !reps) return wilson(sum(xs), xs.length);
  return bootstrapMean(xs, { seed, clusters: scored.map(s => caseOf(s.item)) });
}

function runSpend(run, items, id, runsById) {
  const xs = items.map(i => columnValue(i, id, runsById)).filter(isNum);
  const v = xs.length ? sum(xs) / xs.length : unbox(run.values[id]);
  return isNum(v) ? { value: v } : null;
}

// versusBase pairs a run's items with the compared run's. paired counts the items with a
// counterpart there; n those where both sides have a value.
function versusBase(items, scored, baseValues, metric, seed) {
  const both = scored.filter(s => isNum(baseValues.get(pairOf(s.item))));
  if (!both.length) return null;
  const deltas = both.map(s => s.v - baseValues.get(pairOf(s.item)));
  const tones = deltas.map(d => deltaTone(d, metric));
  const up = tones.filter(t => t === "positive").length;
  const down = tones.filter(t => t === "negative").length;
  return {
    paired: items.filter(i => baseValues.has(pairOf(i))).length,
    n: both.length,
    delta: bootstrapMean(deltas, { seed, clusters: both.map(s => caseOf(s.item)) }),
    up, same: both.length - up - down, down,
    p: metric.kind === "rate" ? mcnemar(up, down) : null,
  };
}

// frontier is the runs no other run beats on both spend (lower) and score (in its better direction).
export function frontier(rows, metric) {
  const pts = rows.filter(r => r.spend && r.score);
  const sign = metric?.better === "lower" ? -1 : 1;
  const beats = (a, b) => {
    const cheaper = b.spend.value - a.spend.value, better = sign * (a.score.value - b.score.value);
    return cheaper >= 0 && better >= 0 && (cheaper > 0 || better > 0);
  };
  return new Set(pts.filter(r => !pts.some(o => beats(o, r))).map(r => r.run.id));
}

export function pairsOf(suite, runId, vsId) {
  const vs = new Map(suite.items.filter(i => i.run === vsId).map(i => [pairOf(i), i]));
  return new Map(suite.items
    .filter(i => i.run === runId && vs.has(pairOf(i)))
    .map(i => [pairOf(i), { item: i, vs: vs.get(pairOf(i)) }]));
}

// withBaselines is the suite as the items table shows one run: runId's items, with the
// paired vsId item's values and effective answers as their baselines. Inline baselines are
// dropped. An answered question the item has no proposal for gets an entry without a
// value, which carries only the baseline.
export function withBaselines(suite, state, runId, vsId) {
  const pairs = pairsOf(suite, runId, vsId);
  const items = suite.items.filter(i => i.run === runId).map(item => {
    const vs = pairs.get(pairOf(item))?.vs;
    const baseline = qid => (vs ? valueOf(state, vs, { id: qid }) : undefined);
    const values = Object.fromEntries(Object.entries(item.values || {})
      .map(([id, v]) => [id, { value: unbox(v), baseline: unbox(vs?.values?.[id]) }]));
    const answered = vs ? Object.keys(answersOf(state, item)).filter(qid => !item.proposed?.[qid]) : [];
    const proposed = Object.fromEntries([
      ...Object.entries(item.proposed || {}).map(([qid, p]) => [qid, { ...p, baseline: baseline(qid) }]),
      ...answered.map(qid => [qid, { baseline: baseline(qid) }]),
    ]);
    return { ...item, values, proposed };
  });
  return { ...suite, items };
}

// acrossRuns is one item's case in every run that has it, in run order.
export function acrossRuns(suite, state, item, metric, { judgeOnly = false } = {}) {
  const runs = runList(suite);
  const runsById = new Map(runs.map(r => [r.id, r]));
  const rows = [];
  for (const run of runs) {
    const it = suite.items.find(i => i.run === run.id && pairOf(i) === pairOf(item));
    if (!it) continue;
    const value = metric ? itemValue(state, it, metric, runsById, { judgeOnly }) : undefined;
    const prev = rows.at(-1)?.value;
    rows.push({ run, item: it, value, deltaPrev: isNum(value) && isNum(prev) ? value - prev : null });
  }
  return rows;
}

// bestRun is the row with the best score in the metric's direction, or null.
export function bestRun(rows, metric) {
  const sign = metric?.better === "lower" ? -1 : 1;
  return rows.filter(r => r.score).reduce((a, b) => (!a || sign * (b.score.value - a.score.value) > 0 ? b : a), null);
}
