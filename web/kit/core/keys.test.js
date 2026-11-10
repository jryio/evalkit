import { test } from "node:test";
import assert from "node:assert/strict";
import { bindings, conflicts, helpRows, match } from "./keys.js";

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
    { keys: ["Esc"], also: [], label: "Stop typing or close a sheet" },
    { keys: ["⌘", "↵"], also: [], label: "Send to Claude" },
  ]);
});
