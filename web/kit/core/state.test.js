import { test } from "node:test";
import assert from "node:assert/strict";
import { emptyState, entry, reconcile, setAnswer, setEdit } from "./state.js";

const item = { id: "a", revision: "r2" };

test("setAnswer stores an answer for the item's revision", () => {
  const before = emptyState();
  const s = setAnswer(before, item, "verdict", "good");
  assert.deepEqual(s.items.a, { revision: "r2", answers: { verdict: "good" }, edits: {} });
  assert.deepEqual(before, emptyState());
});

for (const [name, empty] of [["undefined", undefined], ["empty string", ""], ["empty array", []]]) {
  test(`setAnswer clears with ${name}`, () => {
    const s = setAnswer(setAnswer(emptyState(), item, "notes", "x"), item, "notes", empty);
    assert.deepEqual(s.items.a.answers, {});
  });
}

test("setAnswer per row sets and clears, removing the empty map", () => {
  let s = setAnswer(emptyState(), item, "keep", true, "o1");
  s = setAnswer(s, item, "keep", false, "o2");
  assert.deepEqual(s.items.a.answers, { keep: { o1: true, o2: false } });
  s = setAnswer(s, item, "keep", undefined, "o1");
  assert.deepEqual(s.items.a.answers, { keep: { o2: false } });
  s = setAnswer(s, item, "keep", undefined, "o2");
  assert.deepEqual(s.items.a.answers, {});
});

test("setEdit stores text and clears it when equal to the original", () => {
  const s = setEdit(emptyState(), item, "draft", "new", "old");
  assert.deepEqual(s.items.a.edits, { draft: "new" });
  assert.deepEqual(setEdit(s, item, "draft", "old", "old").items.a.edits, {});
});

test("entry ignores state for an older revision", () => {
  const s = { items: { a: { revision: "r1", answers: { verdict: "good" }, edits: { draft: "x" } } }, history: {} };
  assert.deepEqual(entry(s, item), { revision: "r2", answers: {}, edits: {} });
  assert.deepEqual(entry(s, { id: "a", revision: "r1" }).answers, { verdict: "good" });
  assert.deepEqual(setAnswer(s, item, "notes", "y").items.a, { revision: "r2", answers: { notes: "y" }, edits: {} });
});

test("reconcile moves old revisions with input to history and drops empty ones", () => {
  const now = "2026-11-09T12:00:00.000Z";
  const state = {
    items: {
      a: { revision: "r1", answers: { verdict: "good" }, edits: {} },
      b: { revision: "r1", answers: {}, edits: {} },
      c: { revision: "r1", answers: { verdict: "bad" }, edits: {} },
    },
    history: { a: [{ revision: "r0", answers: { verdict: "bad" }, edits: {}, replaced_at: "earlier" }] },
  };
  const items = [{ id: "a", revision: "r2" }, { id: "b", revision: "r2" }, { id: "c", revision: "r1" }];
  const { state: next, changed } = reconcile(state, items, now);
  assert.equal(changed, true);
  assert.deepEqual(next.items, { c: state.items.c });
  assert.deepEqual(next.history, {
    a: [
      { revision: "r0", answers: { verdict: "bad" }, edits: {}, replaced_at: "earlier" },
      { revision: "r1", answers: { verdict: "good" }, edits: {}, replaced_at: now },
    ],
  });
  assert.ok(state.items.a, "reconcile must not mutate its input");
  assert.equal(reconcile(next, items, now).changed, false);
});
