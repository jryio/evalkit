import { test } from "node:test";
import assert from "node:assert/strict";
import { cell, columns, histogram, profile, scopeCounts, sortBy, visible } from "./table.js";
import { emptyState, setAnswer } from "./state.js";
import { progress } from "./derive.js";

const verdict = {
  id: "verdict", kind: "choice", label: "Verdict", primary: true,
  options: [{ value: "pass", label: "Pass", tone: "positive", icon: "check" }, { value: "fail", label: "Fail", tone: "negative", icon: "cross" }],
};
const helpful = { id: "helpful", kind: "number", label: "Helpful", min: 0, max: 10, step: 1 };
const notes = { id: "notes", kind: "text", label: "Note" };
const perRow = { id: "triage", kind: "choice", label: "Triage", each: "findings", options: verdict.options };

const items = [
  { id: "a", title: "Alpha", group: "train", revision: "r", flag: "Judge confidence 0.51",
    proposed: { verdict: { value: "pass", baseline: "fail" }, helpful: { value: 8, baseline: 6 } },
    values: { cost: { value: 0.22, baseline: 0.2 }, split: "train", lint: true } },
  { id: "b", title: "Bravo", group: "test", revision: "r",
    proposed: { verdict: { value: "fail", baseline: "pass" }, helpful: { value: 3, baseline: 7 } },
    values: { cost: 0.4, split: "test", lint: false } },
  { id: "c", title: "Charlie", group: "train", revision: "r",
    proposed: { verdict: { value: "pass" }, helpful: { value: 10 } },
    values: { cost: null, split: "train", lint: true } },
];
const suite = {
  title: "Round 3",
  groups: [{ id: "train", label: "Train" }, { id: "test", label: "Test" }],
  questions: [verdict, helpful, notes, perRow],
  columns: [{ id: "cost", label: "Cost", unit: "$", better: "lower", noise: 0.05 }, { id: "split", label: "Split" }, { id: "lint", label: "Lint", better: "higher" }],
  items,
};
const ids = rows => rows.map(r => r.id);

test("columns: group, questions with Δ, values with Δ, then notes; per-row questions are left out", () => {
  assert.deepEqual(columns(suite).map(c => c.id),
    ["group", "q:verdict", "d:q:verdict", "q:helpful", "d:q:helpful", "v:cost", "d:v:cost", "v:split", "v:lint", "note"]);
});

test("columns: value types come from the data", () => {
  const byId = Object.fromEntries(columns(suite).map(c => [c.id, c]));
  assert.equal(byId["v:cost"].type, "number");
  assert.equal(byId["v:cost"].decimals, 2);
  assert.equal(byId["v:split"].type, "text");
  assert.equal(byId["v:lint"].type, "options");
  assert.deepEqual(byId["q:helpful"].range, [0, 10]);
  assert.equal(byId["q:helpful"].decimals, 0);
});

test("cell: the judge's proposal shows until the human overrides it", () => {
  const col = columns(suite).find(c => c.id === "q:verdict");
  const before = cell(col, items[0], emptyState(), suite);
  assert.equal(before.text, "Pass");
  assert.equal(before.human, false);
  const state = setAnswer(emptyState(), items[0], "verdict", "fail");
  const after = cell(col, items[0], state, suite);
  assert.equal(after.text, "Fail");
  assert.equal(after.human, true);
  assert.equal(after.from, "Pass");
});

test("cell: numeric Δ is toned by the better direction beyond noise", () => {
  const cols = Object.fromEntries(columns(suite).map(c => [c.id, c]));
  const s = emptyState();
  assert.deepEqual(pick(cell(cols["d:q:helpful"], items[0], s, suite)), { text: "+2", tone: "positive" });
  assert.deepEqual(pick(cell(cols["d:q:helpful"], items[1], s, suite)), { text: "−4", tone: "negative" });
  assert.deepEqual(pick(cell(cols["d:v:cost"], items[0], s, suite)), { text: "+0.02", tone: "neutral" });
  assert.equal(cell(cols["v:cost"], items[0], s, suite).text, "$0.22");
  assert.equal(cell(cols["v:cost"], items[2], s, suite).text, "");
});

test("cell: categorical Δ names the baseline and tones by option tone", () => {
  const col = columns(suite).find(c => c.id === "d:q:verdict");
  assert.deepEqual(pick(cell(col, items[0], emptyState(), suite)), { text: "was Fail", tone: "positive" });
  assert.deepEqual(pick(cell(col, items[1], emptyState(), suite)), { text: "was Pass", tone: "negative" });
  assert.equal(cell(col, items[2], emptyState(), suite).text, "");
  const same = { ...items[2], proposed: { verdict: { value: "pass", baseline: "pass" } } };
  assert.equal(cell(col, same, emptyState(), suite).text, "");
});

test("cell: the note column shows the first line of the first text answer", () => {
  const col = columns(suite).find(c => c.id === "note");
  const state = setAnswer(emptyState(), items[1], "notes", "Judge missed the typo\nsecond line");
  assert.equal(cell(col, items[1], state, suite).text, "Judge missed the typo");
});

test("histogram: one bin per whole number on small integer ranges, else ten", () => {
  const whole = histogram([0, 3, 3, 10], [0, 10], { integer: true });
  assert.equal(whole.length, 11);
  assert.deepEqual(whole.map(b => b.count), [1, 0, 0, 2, 0, 0, 0, 0, 0, 0, 1]);
  const ten = histogram([0, 0.05, 0.5, 1, 1.2], [0, 1], { decimals: 2 });
  assert.equal(ten.length, 10);
  assert.deepEqual(ten.map(b => b.count), [2, 0, 0, 0, 0, 1, 0, 0, 0, 2]);
  assert.equal(ten[0].label, "0.00–0.10");
});

test("profile: mean and bins, option shares, Δ directions", () => {
  const cols = Object.fromEntries(columns(suite).map(c => [c.id, c]));
  const s = emptyState();
  const h = profile(cols["q:helpful"], items, s, suite);
  assert.equal(h.mean, 7);
  assert.equal(h.bins.length, 11);
  const v = profile(cols["q:verdict"], items, s, suite);
  assert.deepEqual(v.counts.map(c => [c.value, c.count]), [["pass", 2], ["fail", 1]]);
  const d = profile(cols["d:q:helpful"], items, s, suite);
  assert.deepEqual([d.better, d.worse, d.mean], [1, 1, -1]);
});

test("visible: scope, group, search, column filters and sort compose", () => {
  const cols = columns(suite);
  const s = setAnswer(emptyState(), items[1], "verdict", "pass");
  assert.deepEqual(ids(visible(suite, s, cols, { scope: "flagged" })), ["a"]);
  assert.deepEqual(ids(visible(suite, s, cols, { scope: "overrides" })), ["b"]);
  assert.deepEqual(ids(visible(suite, s, cols, { group: "train" })), ["a", "c"]);
  assert.deepEqual(ids(visible(suite, s, cols, { search: "char" })), ["c"]);
  assert.deepEqual(ids(visible(suite, s, cols, { where: [{ column: "d:q:helpful", tone: "negative" }] })), ["b"]);
  const bin = profile(cols.find(c => c.id === "q:helpful"), items, s, suite).bins[10];
  assert.deepEqual(ids(visible(suite, s, cols, { where: [{ column: "q:helpful", bin }] })), ["c"]);
  assert.deepEqual(ids(visible(suite, s, cols, { sort: { column: "q:helpful", dir: -1 } })), ["c", "a", "b"]);
  assert.deepEqual(ids(visible(suite, s, cols, { sort: { column: "v:cost", dir: 1 } })), ["a", "b", "c"]);
  assert.deepEqual(ids(visible(suite, s, cols, { sort: { column: "v:cost", dir: -1 } })), ["b", "a", "c"]);
});

test("scopeCounts counts each scope", () => {
  const s = setAnswer(emptyState(), items[2], "notes", "fine");
  assert.deepEqual(scopeCounts(items, s), { all: 3, flagged: 1, input: 1, overrides: 0 });
});

test("sortBy is stable and keeps missing keys last", () => {
  const rows = [{ k: 2 }, { k: undefined }, { k: 1 }, { k: 2 }];
  assert.deepEqual(sortBy(rows, r => r.k, 1).map(r => r.k), [1, 2, 2, undefined]);
  assert.deepEqual(sortBy(rows, r => r.k, -1).map(r => r.k), [2, 2, 1, undefined]);
});

test("progress counts only flagged items once any item is flagged", () => {
  const s = setAnswer(emptyState(), items[0], "verdict", "pass");
  assert.deepEqual(progress(suite, s), { reviewed: 1, total: 1, flagged: true });
  const unflagged = { ...suite, items: items.map(({ flag, ...rest }) => rest) };
  assert.deepEqual(progress(unflagged, s), { reviewed: 1, total: 3 });
});

function pick(c) {
  return { text: c.text, tone: c.tone };
}
