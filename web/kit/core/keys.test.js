import { test } from "node:test";
import assert from "node:assert/strict";
import { bindings, conflicts, helpRows, match, panelBindings, runsBindings, tableBindings } from "./keys.js";

const verdict = {
  id: "verdict", kind: "choice", label: "Verdict", primary: true,
  options: [{ value: "good", label: "Good" }, { value: "close", label: "Close" }, { value: "off", label: "Off" }],
};
const risk = { id: "risk", kind: "choice", label: "Risk", key: "r", options: [{ value: "low", label: "Low" }, { value: "high", label: "High" }] };
const tags = { id: "tags", kind: "multi", label: "Tags", options: [{ value: "perf", label: "Performance", key: "p" }, { value: "style", label: "Style" }] };
const notes = { id: "notes", kind: "text", label: "Notes", key: "n" };
const keep = { id: "keep", kind: "toggle", label: "Keep", key: "x", each: "evidence" };
const post = { id: "post", kind: "action", label: "Post", key: "a", states: { off: { button: "Approve for posting" } } };

const item = { id: "i1", show: [{ id: "draft", kind: "text", label: "Draft", editable: true }] };
const questions = [verdict, risk, tags, notes, keep, post];

const actionFor = (list, key) => list.find(b => b.keys.includes(key))?.action;

test("bindings", async t => {
  const list = bindings({}, item, questions);
  await t.test("navigation", () => {
    assert.deepEqual(list.find(b => b.keys.includes("j")).keys, ["j", "ArrowRight"]);
    assert.deepEqual(actionFor(list, "ArrowRight"), { type: "move", by: 1 });
    assert.deepEqual(list.find(b => b.keys.includes("k")).keys, ["k", "ArrowLeft"]);
    assert.deepEqual(actionFor(list, "ArrowLeft"), { type: "move", by: -1 });
  });
  await t.test("digits pick the primary options", () => {
    assert.deepEqual(actionFor(list, "1"), { type: "set", qid: "verdict", value: "good" });
    assert.deepEqual(actionFor(list, "3"), { type: "set", qid: "verdict", value: "off" });
    assert.equal(actionFor(list, "4"), undefined);
  });
  await t.test("a choice key cycles", () => {
    assert.deepEqual(actionFor(list, "r"), { type: "cycle", qid: "risk", perRow: false });
  });
  await t.test("multi option keys", () => {
    assert.deepEqual(actionFor(list, "p"), { type: "option", qid: "tags", value: "perf", perRow: false });
  });
  await t.test("a multi key arms it for digits", () => {
    const off = { ...tags, key: "f" };
    const armed = bindings({}, item, [verdict, off]).find(b => b.keys.includes("f"));
    assert.deepEqual(armed.action, { type: "arm", qid: "tags", perRow: false });
    assert.equal(armed.label, "Tags: F then 1–2 to toggle");
  });
  await t.test("a text key focuses the question", () => {
    assert.deepEqual(actionFor(list, "n"), { type: "focus", target: "q:notes" });
  });
  await t.test("an editable artifact gets e by default", () => {
    assert.deepEqual(actionFor(list, "e"), { type: "focus", target: "a:draft" });
    const custom = { show: [{ id: "draft", kind: "text", editable: true, key: "d" }] };
    assert.deepEqual(actionFor(bindings({}, custom, []), "d"), { type: "focus", target: "a:draft" });
  });
  await t.test("a per-row question key acts on the current row", () => {
    assert.deepEqual(actionFor(list, "x"), { type: "toggle", qid: "keep", perRow: true });
  });
  await t.test("an action toggles until it is done", () => {
    assert.deepEqual(actionFor(list, "a"), { type: "toggle", qid: "post" });
    assert.equal(actionFor(bindings({}, { ...item, done: { post: {} } }, questions), "a"), undefined);
  });
});

test("match lowercases letters and passes other keys through", () => {
  const list = bindings({}, item, questions);
  assert.deepEqual(match(list, { key: "J" }).action, { type: "move", by: 1 });
  assert.deepEqual(match(list, { key: "?" }).action, { type: "help" });
  assert.deepEqual(match(list, { key: "ArrowLeft" }).action, { type: "move", by: -1 });
  assert.equal(match(list, { key: "z" }), undefined);
});

test("conflicts finds keys bound twice", () => {
  const clash = { id: "edit", kind: "text", label: "Edit note", key: "e" };
  assert.deepEqual(conflicts(bindings({}, item, questions)), []);
  assert.deepEqual(conflicts(bindings({}, item, [clash])), [{ key: "e", first: "Edit note", second: "Edit Draft" }]);
});

test("helpRows merges the primary digits and navigation and appends fixed keys", () => {
  const rows = helpRows(bindings({}, { show: [] }, [verdict, notes]), "Send to Claude");
  assert.deepEqual(rows, [
    { keys: ["J", "K"], also: ["→", "←"], label: "Next / previous item" },
    { keys: ["1", "2", "3"], also: [], label: "Verdict: good, close, off" },
    { keys: ["N"], also: [], label: "Notes" },
    { keys: ["?"], also: [], label: "Keyboard shortcuts" },
    { keys: ["Esc"], also: [], label: "Stop typing or close a sheet; then back to the table" },
    { keys: ["⌘", "↵"], also: [], label: "Send to Claude" },
  ]);
});

test("tableBindings move the selection, open it, answer the primary question and filter", () => {
  const questions = [{ id: "v", kind: "choice", label: "Verdict", primary: true, options: [{ value: "pass", label: "Pass" }, { value: "fail", label: "Fail" }] }];
  assert.equal(match(tableBindings([]), { key: "1" }), undefined, "no digits without a primary on screen");
  const list = tableBindings(questions);
  const key = k => match(list, { key: k })?.action;
  assert.deepEqual(key("ArrowDown"), { type: "select", by: 1 });
  assert.deepEqual(key("k"), { type: "select", by: -1 });
  assert.deepEqual(key("Enter"), { type: "open" });
  assert.deepEqual(key("2"), { type: "set", qid: "v", value: "fail" });
  assert.deepEqual(key("/"), { type: "focus", target: "table-search" });
  assert.deepEqual(conflicts(list), []);
  assert.equal(helpRows(list, "Send", "table").find(r => r.keys[0] === "Esc").label, "Stop typing or close a sheet");
});

test("panelBindings keep the item's keys and move the table selection with J/K and arrows", () => {
  const verdict = { id: "v", kind: "choice", label: "Verdict", primary: true, options: [{ value: "pass", label: "Pass" }, { value: "fail", label: "Fail" }] };
  const notes = { id: "n", kind: "text", label: "Notes", key: "n" };
  const item = { id: "i" };
  const list = panelBindings({}, item, [verdict, notes]);
  const key = k => match(list, { key: k })?.action;
  assert.deepEqual(key("j"), { type: "select", by: 1 });
  assert.deepEqual(key("ArrowUp"), { type: "select", by: -1 });
  assert.deepEqual(key("ArrowRight"), { type: "select", by: 1 });
  assert.deepEqual(key("ArrowLeft"), { type: "select", by: -1 });
  assert.deepEqual(key("1"), { type: "set", qid: "v", value: "pass" });
  assert.deepEqual(key("n"), { type: "focus", target: "q:n" });
  assert.deepEqual(key("/"), { type: "focus", target: "table-search" });
  assert.deepEqual(conflicts(list), []);
  const withRows = panelBindings({}, item, [verdict], 3);
  assert.deepEqual(match(withRows, { key: "ArrowDown" }).action, { type: "row", by: 1 });
});

test("[ and ] step runs in review, table and panel, only with two runs or more", () => {
  const step = list => [match(list, { key: "[" })?.action, match(list, { key: "]" })?.action];
  const both = [{ type: "run", by: -1 }, { type: "run", by: 1 }];
  for (const runs of [0, 1]) {
    assert.deepEqual(step(bindings({}, item, questions, 0, runs)), [undefined, undefined]);
    assert.deepEqual(step(tableBindings([], runs)), [undefined, undefined]);
    assert.deepEqual(step(panelBindings({}, item, questions, 0, runs)), [undefined, undefined]);
  }
  const lists = [bindings({}, item, questions, 0, 3), tableBindings([verdict], 3), panelBindings({}, item, questions, 2, 3)];
  for (const list of lists) {
    assert.deepEqual(step(list), both);
    assert.deepEqual(conflicts(list), []);
    assert.deepEqual(helpRows(list, "Send").find(r => r.label === "Previous / next run"), { keys: ["[", "]"], also: [], label: "Previous / next run" });
  }
});

test("runsBindings move the run selection, open it in the table and filter runs", () => {
  const list = runsBindings(3);
  const key = k => match(list, { key: k })?.action;
  assert.deepEqual([key("ArrowDown"), key("j")], [{ type: "selectRun", by: 1 }, { type: "selectRun", by: 1 }]);
  assert.deepEqual([key("ArrowUp"), key("K")], [{ type: "selectRun", by: -1 }, { type: "selectRun", by: -1 }]);
  assert.deepEqual(key("Enter"), { type: "openRun" });
  assert.deepEqual(key("/"), { type: "focus", target: "runs-search" });
  assert.deepEqual(key("?"), { type: "help" });
  assert.deepEqual([key("["), key("]")], [{ type: "run", by: -1 }, { type: "run", by: 1 }]);
  assert.equal(key("1"), undefined);
  assert.deepEqual(conflicts(list), []);
  const rows = helpRows(list, "Send", "runs", 3);
  assert.deepEqual(rows[0], { keys: ["↓", "↑"], also: ["J", "K"], label: "Select the run below / above" });
  assert.equal(rows.find(r => r.keys[0] === "Esc").label, "Stop typing or close a sheet");
});

test("M edits the message, unless a question already uses M", () => {
  assert.deepEqual(actionFor(bindings({}, item, questions), "m"), { type: "focus", target: "a:draft" });
  const list = bindings({}, item, [{ id: "mood", kind: "text", label: "Mood", key: "m" }]);
  assert.equal(list.filter(b => b.keys.includes("m")).length, 1);
  assert.deepEqual(actionFor(list, "e"), { type: "focus", target: "a:draft" });
});

test("the table opens a row with Space and filters with F, showing only /", () => {
  const list = tableBindings([verdict]);
  assert.deepEqual(match(list, { key: " " }).action, { type: "open" });
  assert.deepEqual(match(list, { key: "f" }).action, { type: "focus", target: "table-search" });
  const shown = helpRows(list, "Send").flatMap(r => [...r.keys, ...r.also]);
  assert.ok(shown.includes("/") && !shown.includes("F"));
});

test("Esc help climbs review → table → runs", () => {
  const esc = (view, runs) => helpRows([], "Send", view, runs).find(r => r.keys[0] === "Esc").label;
  assert.equal(esc("review", 3), "Stop typing or close a sheet; then back to the table");
  assert.equal(esc("table", 3), "Stop typing or close a sheet; then back to runs");
  assert.equal(esc("table", 0), "Stop typing or close a sheet");
});
