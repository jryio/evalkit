import { test } from "node:test";
import assert from "node:assert/strict";
import {
  isReviewed, limitTone, measure, orderedItems, pendingActions, progress,
  proposalNote, questionsFor, snapshot, statusTone, trailing, valueOf,
} from "./derive.js";
import { emptyState, setAnswer } from "./state.js";

const tones = [
  { value: "onTrack", label: "On track", tone: "positive" },
  { value: "atRisk", label: "At risk", tone: "warning" },
  { value: "offTrack", label: "Off track", tone: "negative" },
];
const verdict = {
  id: "verdict", kind: "choice", label: "Verdict", primary: true,
  options: [{ value: "good", label: "Good" }, { value: "bad", label: "Bad" }],
};
const health = { id: "health", kind: "choice", label: "Health", status: true, options: tones };
const notes = { id: "notes", kind: "text", label: "Notes" };
const post = {
  id: "post", kind: "action", label: "Post", groups: ["live"],
  states: { off: { button: "Approve" }, on: { button: "Approved", badge: "Post" }, done: { button: "Posted", badge: "Posted" } },
};

const live = { id: "l1", group: "live", title: "Live one", revision: "r1", proposed: { health: { value: "atRisk" } } };
const live2 = { id: "l2", group: "live", title: "Live two", revision: "r1" };
const back = { id: "b1", group: "back", title: "Back one", revision: "r1", ask: [{ id: "q1", kind: "text", label: "Why?" }] };

const suite = {
  title: "Updates",
  groups: [{ id: "live" }, { id: "back" }],
  questions: [health, verdict, notes, post],
  items: [live, live2, back],
};

test("questionsFor filters by groups and appends item questions", () => {
  const ids = q => q.map(x => `${x.id}:${x.scope}`);
  assert.deepEqual(ids(questionsFor(suite, live)), ["health:suite", "verdict:suite", "notes:suite", "post:suite"]);
  assert.deepEqual(ids(questionsFor(suite, back)), ["health:suite", "verdict:suite", "notes:suite", "q1:item"]);
});

test("valueOf prefers the answer and falls back to the proposal", () => {
  assert.equal(valueOf(emptyState(), live, health), "atRisk");
  assert.equal(valueOf(setAnswer(emptyState(), live, "health", "onTrack"), live, health), "onTrack");
  assert.equal(valueOf(emptyState(), live2, health), undefined);
  const s = setAnswer(emptyState(), live, "keep", true, "o1");
  assert.equal(valueOf(s, live, { id: "keep" }, "o1"), true);
  assert.equal(valueOf(s, live, { id: "keep" }, "o2"), undefined);
});

test("isReviewed", async t => {
  await t.test("uses suite.reviewed when set", () => {
    const s = { ...suite, reviewed: ["notes"] };
    assert.equal(isReviewed(s, setAnswer(emptyState(), live2, "verdict", "good"), live2), false);
    assert.equal(isReviewed(s, setAnswer(emptyState(), live2, "notes", "x"), live2), true);
  });
  await t.test("defaults to the primary question", () => {
    assert.equal(isReviewed(suite, setAnswer(emptyState(), live2, "health", "onTrack"), live2), false);
    assert.equal(isReviewed(suite, setAnswer(emptyState(), live2, "verdict", "bad"), live2), true);
  });
  await t.test("counts any answer when there is no primary", () => {
    const s = { ...suite, questions: [health, notes] };
    assert.equal(isReviewed(s, emptyState(), live2), false);
    assert.equal(isReviewed(s, setAnswer(emptyState(), live2, "health", "onTrack"), live2), true);
  });
  await t.test("counts a done action", () => {
    assert.equal(isReviewed(suite, emptyState(), { ...live2, done: { post: { at: "now" } } }), true);
  });
});

test("progress counts reviewed items", () => {
  const s = setAnswer(setAnswer(emptyState(), live, "verdict", "good"), back, "verdict", "bad");
  assert.deepEqual(progress(suite, s), { reviewed: 2, total: 3 });
  assert.deepEqual(progress(suite, emptyState()), { reviewed: 0, total: 3 });
});

test("statusTone is the tone of the status question's value", () => {
  assert.equal(statusTone(suite, emptyState(), live), "warning");
  assert.equal(statusTone(suite, setAnswer(emptyState(), live, "health", "offTrack"), live), "negative");
  assert.equal(statusTone(suite, emptyState(), live2), null);
  assert.equal(statusTone({ ...suite, questions: [verdict] }, emptyState(), live), null);
});

test("trailing: done beats approved beats reviewed", () => {
  const reviewed = setAnswer(emptyState(), live, "verdict", "good");
  const approved = setAnswer(reviewed, live, "post", true);
  assert.deepEqual(trailing(suite, approved, { ...live, done: { post: {} } }), { kind: "done", label: "Posted" });
  assert.deepEqual(trailing(suite, approved, live), { kind: "approved", label: "Post" });
  assert.deepEqual(trailing(suite, reviewed, live), { kind: "reviewed" });
  assert.equal(trailing(suite, emptyState(), live), null);
});

test("pendingActions lists approved actions that are not done", () => {
  const done = { ...live2, done: { post: { at: "now" } } };
  const s = setAnswer(setAnswer(emptyState(), live, "post", true), done, "post", true);
  const got = pendingActions({ ...suite, items: [live, done, back] }, s);
  assert.deepEqual(got.map(p => `${p.item.id}:${p.question.id}`), ["l1:post"]);
});

test("snapshot", () => {
  let s = setAnswer(emptyState(), live, "health", "offTrack");
  s = setAnswer(s, live, "post", true);
  s = setAnswer(s, back, "verdict", "good");
  const got = snapshot(suite, s);
  assert.equal(got.suite, "Updates");
  assert.deepEqual(got.progress, { reviewed: 1, total: 3 });
  assert.deepEqual(got.actions, [{ item: "l1", action: "post", title: "Live one" }]);
  assert.deepEqual(Object.keys(got.items).sort(), ["b1", "l1"]);
  assert.deepEqual(got.items.l1, {
    title: "Live one", revision: "r1", reviewed: false,
    answers: { health: "offTrack", post: true }, edits: {},
    changed: { health: { from: "atRisk", to: "offTrack" } },
  });
  assert.equal("changed" in got.items.b1, false);
  const same = snapshot(suite, setAnswer(emptyState(), live, "health", "atRisk"));
  assert.equal("changed" in same.items.l1, false);
});

test("measure", () => {
  assert.equal(measure("  abc  "), 3);
  assert.equal(measure("   "), 0);
  assert.equal(measure(undefined), 0);
});

test("limitTone boundaries", () => {
  const limit = { target: [200, 300], max: 500 };
  for (const [n, want] of [[0, "positive"], [300, "positive"], [301, "warning"], [500, "warning"], [501, "negative"]]) {
    assert.equal(limitTone(n, limit), want, `limitTone(${n})`);
  }
  assert.equal(limitTone(500, { max: 500 }), "positive");
  assert.equal(limitTone(501, { max: 500 }), "negative");
  assert.equal(limitTone(3, { target: [1, 2] }), "warning");
});

test("orderedItems sorts by group and keeps written order within a group", () => {
  const items = [
    { id: "x", group: "back" }, { id: "y", group: "live" }, { id: "z", group: "back" }, { id: "w", group: "live" },
  ];
  assert.deepEqual(orderedItems({ ...suite, items }).map(i => i.id), ["y", "w", "x", "z"]);
  assert.deepEqual(items.map(i => i.id), ["x", "y", "z", "w"]);
});

test("an action hidden by when is neither pending nor shown in the sidebar", () => {
  const gate = { id: "ship", kind: "action", label: "Ship", when: { verdict: ["good"] }, states: { off: { button: "Ship" } } };
  const suite = { questions: [verdict, gate], items: [live2] };
  let s = setAnswer(emptyState(), live2, "ship", true);
  s = setAnswer(s, live2, "verdict", "off");
  assert.deepEqual(pendingActions(suite, s), []);
  assert.equal(trailing(suite, s, live2)?.kind, "reviewed");
  s = setAnswer(s, live2, "verdict", "good");
  assert.equal(pendingActions(suite, s).length, 1);
});

test("null is an explicit empty answer that hides the proposal", () => {
  const item = { id: "p", revision: "r1", proposed: { notes: { value: "agent text" } } };
  const s = setAnswer(emptyState(), item, "notes", null);
  assert.equal(valueOf(s, item, notes), null);
});

test("snapshot compares proposed lists by value", () => {
  const tags = { id: "tags", kind: "multi", label: "Tags", options: [{ value: "a", label: "A" }, { value: "b", label: "B" }] };
  const item = { id: "m", title: "M", revision: "r1", proposed: { tags: { value: ["a", "b"] } } };
  const suite = { title: "S", questions: [tags], items: [item] };
  const same = snapshot(suite, setAnswer(emptyState(), item, "tags", ["a", "b"]));
  assert.equal(same.items.m.changed, undefined);
  const diff = snapshot(suite, setAnswer(emptyState(), item, "tags", ["a"]));
  assert.deepEqual(diff.items.m.changed, { tags: { from: ["a", "b"], to: ["a"] } });
});

test("a proposed entry without a value is a baseline, not a proposal the human overrode", () => {
  const onlyBase = { id: "o1", group: "back", title: "Baseline only", revision: "r1", proposed: { verdict: { baseline: "bad" } } };
  const s = setAnswer(emptyState(), onlyBase, "verdict", "good");
  assert.equal(valueOf(s, onlyBase, verdict), "good");
  assert.equal("changed" in snapshot({ ...suite, items: [onlyBase] }, s).items.o1, false);
});

test("proposalNote names the proposal and keeps its reason", () => {
  assert.equal(proposalNote("Bad", undefined), null);
  assert.deepEqual(proposalNote("Bad", { value: "bad", why: "Drops the only check." }), { lead: "Proposed Bad", text: "Drops the only check." });
  assert.deepEqual(proposalNote(7, { value: 7 }), { lead: "Proposed 7", text: "" });
});
