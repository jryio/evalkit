import { test } from "node:test";
import assert from "node:assert/strict";
import { acrossRuns, frontier, itemValue, latestRun, metricOf, pairsOf, runList, spendOf, summarize, withBaselines } from "./runs.js";
import { bootstrapMean, mcnemar, wilson } from "./stats.js";
import { cell, columns } from "./table.js";
import { emptyState, setAnswer } from "./state.js";

const verdict = {
  id: "verdict", kind: "choice", label: "Verdict", primary: true,
  options: [{ value: "pass", label: "Pass", tone: "positive" }, { value: "fail", label: "Fail", tone: "negative" }, { value: "skip", label: "Skip" }],
};
const it = (run, id, v, extra = {}) => ({ id: `${run}/${id}`, pair: id, run, title: id, revision: "r", proposed: { verdict: { value: v } }, ...extra });
const hard = { group: "hard" };

// base: a b c d g; v2 drops d and g and adds e; v3 drops c and adds f. g and f are skips (not counted).
const items = [
  it("base", "a", "pass", { ...hard, values: { cost: 6 } }),
  it("base", "b", "fail", hard),
  it("base", "c", "pass"),
  it("base", "d", "fail"),
  it("base", "g", "skip"),
  it("v2", "a", "pass", { ...hard, values: { cost: { value: 4, baseline: 99 } } }),
  it("v2", "b", "pass", { ...hard, proposed: { verdict: { value: "pass", baseline: "fail" } } }),
  it("v2", "c", "fail"),
  it("v2", "e", "pass"),
  it("v3", "a", "pass", hard),
  it("v3", "b", "pass", hard),
  it("v3", "d", "pass"),
  it("v3", "f", "skip"),
  it("v3", "g", "pass"),
];
const suite = {
  title: "Runs",
  questions: [verdict],
  columns: [{ id: "cost", label: "Cost", unit: "$" }],
  runs: [{ id: "base", values: { cost: 1 } }, { id: "v2", values: { cost: 2 } }, { id: "v3", values: { cost: 0.5 } }],
  items,
};
const byId = id => items.find(i => i.id === id);
const counts = vs => vs && { paired: vs.paired, n: vs.n, up: vs.up, same: vs.same, down: vs.down };

test("runList: declared runs, then runs only items name; the first run is the default baseline", () => {
  const s = {
    runs: [{ id: "a" }, { id: "b", label: "Bee", note: "no skill", dims: { model: "x" }, values: { cost: 1 } }],
    items: [{ id: "c/1", run: "c" }, { id: "a/1", run: "a" }, { id: "d/1", run: "d" }, { id: "c/2", run: "c" }],
  };
  const runs = runList(s);
  assert.deepEqual(runs.map(r => [r.id, r.label, r.baseline, r.index]),
    [["a", "a", true, 0], ["b", "Bee", false, 1], ["c", "c", false, 2], ["d", "d", false, 3]]);
  assert.deepEqual([runs[1].note, runs[1].dims, runs[1].values], ["no skill", { model: "x" }, { cost: 1 }]);
  assert.deepEqual([runs[2].dims, runs[2].values], [{}, {}]);
  assert.equal(latestRun(s), "d");
  const declared = { ...s, runs: [{ id: "a" }, { id: "b", baseline: true }] };
  assert.deepEqual(runList(declared).filter(r => r.baseline).map(r => r.id), ["b"]);
  assert.deepEqual(runList({ items: [{ id: "y/1", run: "y" }, { id: "x/1", run: "x" }] }).map(r => [r.id, r.baseline]), [["y", true], ["x", false]]);
  assert.deepEqual(runList({ items: [{ id: "x" }] }), []);
  assert.equal(latestRun({ items: [] }), null);
});

test("metricOf: a choice question is a rate; options score by score, else tone", () => {
  const m = metricOf(suite);
  assert.deepEqual([m.id, m.source, m.kind, m.better], ["verdict", "question", "rate", "higher"]);
  const v = value => itemValue(emptyState(), it("x", "q", value), m, new Map());
  assert.deepEqual(["pass", "fail", "skip", "other"].map(v), [1, 0, undefined, undefined]);
  const graded = { ...verdict, options: [...verdict.options, { value: "partial", label: "Partial", tone: "positive", score: 0.5 }] };
  assert.equal(itemValue(emptyState(), it("x", "q", "partial"), metricOf({ questions: [graded] }), new Map()), 0.5);
  const toggle = metricOf({ questions: [{ id: "ok", kind: "toggle", primary: true }] });
  assert.equal(toggle.kind, "rate");
  const t = value => itemValue(emptyState(), { id: "t", revision: "r", proposed: { ok: { value } } }, toggle, new Map());
  assert.deepEqual([t(true), t(false), t(undefined)], [1, 0, undefined]);
});

test("metricOf: a number question is a mean; else a bounded column, then a directed one", () => {
  const n = metricOf({ questions: [{ id: "score", kind: "number", label: "Score", unit: "pts", primary: true }] });
  assert.deepEqual([n.source, n.kind, n.unit, n.better], ["question", "mean", "pts", "higher"]);
  const text = [{ id: "why", kind: "text", primary: true }];
  const cols = [{ id: "x" }, { id: "y", better: "lower", noise: 0.1 }, { id: "z", label: "Zed", min: 0, max: 1 }];
  const z = metricOf({ questions: text, columns: cols });
  assert.deepEqual([z.id, z.label, z.source, z.kind, z.better], ["z", "Zed", "column", "mean", "higher"]);
  const y = metricOf({ columns: cols.slice(0, 2) });
  assert.deepEqual([y.id, y.better, y.noise], ["y", "lower", 0.1]);
  assert.equal(metricOf({ columns: [{ id: "x" }] }), null);
  const primary = metricOf({ questions: [{ id: "v", kind: "choice", primary: true, options: [] }], columns: [...cols, { id: "audit", label: "Audit", primary: true }] });
  assert.equal(primary.id, "audit", "a primary column wins over the primary question");
});

test("spendOf: dollars, else tokens, else seconds", () => {
  const secs = { id: "latency", unit: "s" }, toks = { id: "out", label: "Output tokens" }, usd = { id: "cost", unit: "$" };
  assert.deepEqual(spendOf({ columns: [secs, toks, usd] }), { id: "cost", label: "cost", unit: "$", better: "lower" });
  assert.equal(spendOf({ columns: [secs, toks] }).id, "out");
  assert.equal(spendOf({ columns: [secs] }).id, "latency");
  assert.equal(spendOf({ columns: [{ id: "x" }] }), null);
});

test("itemValue reads a column from the item, boxed or not, else from its run", () => {
  const m = metricOf({ columns: [{ id: "latency", better: "lower" }] });
  const runsById = new Map([["r", { values: { latency: 7 } }]]);
  const v = (values, run = "r") => itemValue(emptyState(), { run, values }, m, runsById);
  assert.deepEqual([v({ latency: 3 }), v({ latency: { value: 4, baseline: 1 } }), v({ latency: null }), v(undefined), v({}, "other")], [3, 4, 7, 7, undefined]);
});

test("summarize: scores, spend, paired counts against the baseline and the step from the previous run", () => {
  const rows = summarize(suite, emptyState());
  assert.deepEqual(rows.map(r => [r.run.id, r.items, r.n]), [["base", 5, 4], ["v2", 4, 4], ["v3", 5, 4]]);
  assert.deepEqual(rows.map(r => r.score), [wilson(2, 4), wilson(3, 4), wilson(4, 4)]);
  assert.deepEqual(rows.map(r => r.spend.value), [2, 2.5, 0.5]);
  assert.deepEqual(rows.map(r => r.spendPrev), [null, 0.5, -2]);
  assert.deepEqual(rows.map(r => r.vsPrev), [null, { delta: 0.25 }, { delta: 0.25 }]);
  assert.equal(rows[0].vsBase, null);
  assert.deepEqual(counts(rows[1].vsBase), { paired: 3, n: 3, up: 1, same: 1, down: 1 });
  assert.equal(rows[1].vsBase.p, 1);
  // g pairs with the baseline but the baseline skipped it: paired, not compared.
  assert.deepEqual(counts(rows[2].vsBase), { paired: 4, n: 3, up: 2, same: 1, down: 0 });
  assert.equal(rows[2].vsBase.p, mcnemar(2, 0));
  const d = rows[2].vsBase.delta;
  assert.ok(Math.abs(d.value - 2 / 3) < 1e-12 && d.lo <= d.value && d.value <= d.hi);
});

test("summarize: a human override counts unless judgeOnly", () => {
  const state = setAnswer(emptyState(), byId("v2/c"), "verdict", "pass");
  const human = summarize(suite, state)[1];
  assert.equal(human.score.value, 1);
  assert.deepEqual(counts(human.vsBase), { paired: 3, n: 3, up: 1, same: 2, down: 0 });
  const judge = summarize(suite, state, { judgeOnly: true })[1];
  assert.equal(judge.score.value, 0.75);
  assert.deepEqual(counts(judge.vsBase), { paired: 3, n: 3, up: 1, same: 1, down: 1 });
});

test("summarize: group filters items first", () => {
  const rows = summarize(suite, emptyState(), { group: "hard" });
  assert.deepEqual(rows.map(r => [r.items, r.score.value]), [[2, 0.5], [2, 1], [2, 1]]);
  assert.deepEqual(counts(rows[1].vsBase), { paired: 2, n: 2, up: 1, same: 1, down: 0 });
});

const lat = (run, id, v, rep) => {
  const pair = rep ? `${id}.r${rep}` : id;
  return { id: `${run}/${pair}`, pair, rep, run, title: id, revision: "r", values: { latency: v } };
};
const latency = better => ({
  title: "Latency",
  columns: [{ id: "latency", better, noise: 0.5 }],
  runs: [{ id: "base" }, { id: "fast" }],
  items: [lat("base", "a", 10), lat("base", "b", 10), lat("base", "c", 10), lat("fast", "a", 8), lat("fast", "b", 10.3), lat("fast", "c", 9)],
});

test("summarize: means, better lower flips up and down, and noise makes same", () => {
  const lower = summarize(latency("lower"), emptyState());
  assert.equal(lower[0].score.value, 10);
  assert.ok(Math.abs(lower[1].score.value - 9.1) < 1e-12);
  assert.deepEqual(counts(lower[1].vsBase), { paired: 3, n: 3, up: 2, same: 1, down: 0 });
  assert.equal(lower[1].vsBase.p, null);
  assert.ok(Math.abs(lower[1].vsBase.delta.value + 0.9) < 1e-12);
  assert.deepEqual(counts(summarize(latency("higher"), emptyState())[1].vsBase), { paired: 3, n: 3, up: 0, same: 1, down: 2 });
});

test("summarize: reps of one case resample together", () => {
  const s = {
    columns: [{ id: "latency", better: "lower" }],
    runs: [{ id: "base" }, { id: "fast" }],
    items: [
      lat("base", "a", 10), lat("base", "a", 10, 1), lat("base", "b", 10), lat("base", "b", 10, 1),
      lat("fast", "a", 8), lat("fast", "a", 7, 1), lat("fast", "b", 10), lat("fast", "b", 11, 1),
    ],
  };
  const [base, fast] = summarize(s, emptyState(), { seed: 5 });
  const clusters = ["a", "a", "b", "b"];
  assert.deepEqual(base.score, bootstrapMean([10, 10, 10, 10], { seed: 5, clusters }));
  assert.deepEqual(fast.vsBase.delta, bootstrapMean([-2, -3, 0, 1], { seed: 5, clusters }));
});

test("frontier keeps runs no other run beats on spend and score", () => {
  const row = (id, score, spend) => ({ run: { id }, score: score === null ? null : { value: score }, spend: spend === null ? null : { value: spend } });
  const rows = [row("a", 0.5, 1), row("b", 0.7, 2), row("c", 0.6, 3), row("d", 0.9, 5), row("e", 0.99, null), row("f", 0.7, 2)];
  assert.deepEqual([...frontier(rows, { better: "higher" })], ["a", "b", "d", "f"]);
  assert.deepEqual([...frontier(rows, { better: "lower" })], ["a"]);
});

test("pairsOf pairs a run's items with another run's by pair", () => {
  const pairs = pairsOf(suite, "v2", "base");
  assert.deepEqual([...pairs.keys()], ["a", "b", "c"]);
  assert.equal(pairs.get("c").vs.id, "base/c");
});

test("withBaselines feeds the items table one run with the other run as its baseline", () => {
  const state = setAnswer(emptyState(), byId("base/b"), "verdict", "pass");
  const v2 = withBaselines(suite, state, "v2", "base");
  assert.deepEqual(v2.items.map(i => i.id), ["v2/a", "v2/b", "v2/c", "v2/e"]);
  assert.deepEqual(v2.items[0].values.cost, { value: 4, baseline: 6 });
  assert.deepEqual(v2.items.map(i => i.proposed.verdict.baseline), ["pass", "pass", "pass", undefined]);
  const cols = columns(v2);
  assert.deepEqual(cols.map(c => c.id), ["q:verdict", "d:q:verdict", "v:cost", "d:v:cost"]);
  const delta = cols.find(c => c.id === "d:q:verdict");
  const c = cell(delta, v2.items[2], state, v2);
  assert.deepEqual([c.text, c.tone], ["was Pass", "negative"]);
  assert.equal(cell(cols.find(c => c.id === "d:v:cost"), v2.items[0], state, v2).text, "−2");
  assert.equal(byId("v2/a").values.cost.baseline, 99);
});

test("withBaselines drops inline baselines when there is nothing to pair with", () => {
  const alone = withBaselines(suite, emptyState(), "v2", "none");
  assert.deepEqual(columns(alone).map(c => c.id), ["q:verdict", "v:cost"]);
});

test("acrossRuns lists an item's case in run order with the step from the previous run", () => {
  const m = metricOf(suite);
  const steps = (item, state = emptyState(), opts) => acrossRuns(suite, state, item, m, opts).map(r => [r.run.id, r.item.id, r.value, r.deltaPrev]);
  assert.deepEqual(steps(byId("v3/b")), [["base", "base/b", 0, null], ["v2", "v2/b", 1, 1], ["v3", "v3/b", 1, 0]]);
  assert.deepEqual(steps(byId("base/c")), [["base", "base/c", 1, null], ["v2", "v2/c", 0, -1]]);
  const state = setAnswer(emptyState(), byId("v2/c"), "verdict", "pass");
  assert.deepEqual(steps(byId("base/c"), state).map(r => r[3]), [null, 0]);
  assert.deepEqual(steps(byId("base/c"), state, { judgeOnly: true }).map(r => r[3]), [null, -1]);
  assert.deepEqual(steps(byId("v3/f")).map(r => [r[0], r[2], r[3]]), [["v3", undefined, null]]);
});
