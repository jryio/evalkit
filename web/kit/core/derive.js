// Pure reads over (suite, state): which questions apply, effective values, progress,
// sidebar status and the snapshot sent to the agent.
import { entry, hasInput } from "./state.js";

// orderedItems is the sidebar and navigation order: by group, then as written.
export function orderedItems(suite) {
  const rank = new Map((suite.groups || []).map((g, i) => [g.id, i]));
  return [...suite.items].sort((a, b) => (rank.get(a.group) ?? 0) - (rank.get(b.group) ?? 0));
}

export const primaryOf = suite => (suite.questions || []).find(q => q.primary);
export const statusOf = suite => (suite.questions || []).find(q => q.status);

export function questionsFor(suite, item) {
  const shared = (suite.questions || []).filter(q => !q.groups || q.groups.includes(item.group));
  return [...shared.map(q => ({ ...q, scope: "suite" })), ...(item.ask || []).map(q => ({ ...q, scope: "item" }))];
}

export const answersOf = (state, item) => entry(state, item).answers;
export const editsOf = (state, item) => entry(state, item).edits;
export const optionOf = (q, value) => (q.options || []).find(o => o.value === value);
export const doneOf = (item, qid) => item.done?.[qid];
export const artifactsOf = item => [...(item.show || []), ...(item.context || [])];

// valueOf is the human's answer, else the agent's proposal. Per-row proposals are { rowId: value } maps.
export function valueOf(state, item, q, row) {
  const a = answersOf(state, item)[q.id];
  const p = item.proposed?.[q.id]?.value;
  if (row !== undefined) return a?.[row] !== undefined ? a[row] : p?.[row];
  return a !== undefined ? a : p;
}

// rowTargets are the rows the keyboard cursor visits: rows of shown artifacts that
// per-row (each) or pick (from) questions point at, in page order.
export function rowTargets(item, questions) {
  return (item.show || [])
    .filter(a => a.rows && questions.some(q => q.each === a.id || q.from === a.id))
    .flatMap(a => a.rows.filter(r => r.ask !== false).map(r => ({ artifact: a.id, row: r.id })));
}

// asked is false while a question's `when` condition fails (per row for per-row questions).
export function asked(state, item, questions, q, row) {
  return Object.entries(q.when || {}).every(([qid, values]) => {
    const dep = questions.find(x => x.id === qid);
    return dep !== undefined && values.includes(valueOf(state, item, dep, row));
  });
}

function reviewedIds(suite) {
  if (suite.reviewed?.length) return suite.reviewed;
  const p = primaryOf(suite);
  return p ? [p.id] : null;
}

// An item is reviewed once any "reviewed" question is answered (default: the primary
// question, else any answer) or the agent has completed one of its actions.
export function isReviewed(suite, state, item) {
  if (Object.keys(item.done || {}).length) return true;
  const answers = answersOf(state, item);
  const ids = reviewedIds(suite);
  return ids ? ids.some(id => answers[id] !== undefined) : Object.keys(answers).length > 0;
}

export function progress(suite, state) {
  const reviewed = suite.items.filter(i => isReviewed(suite, state, i)).length;
  return { reviewed, total: suite.items.length };
}

export function statusTone(suite, state, item) {
  const q = statusOf(suite);
  return q ? optionOf(q, valueOf(state, item, q))?.tone ?? null : null;
}

// actionsFor lists the item's action questions that are on screen (`when` can hide one).
function actionsFor(suite, state, item) {
  const qs = questionsFor(suite, item);
  return qs.filter(q => q.kind === "action" && asked(state, item, qs, q));
}

// trailing is the sidebar row's end mark: an action's done or approved badge, else a check.
export function trailing(suite, state, item) {
  const actions = actionsFor(suite, state, item);
  const done = actions.find(q => doneOf(item, q.id));
  if (done) return { kind: "done", label: done.states.done?.badge ?? "Done" };
  const approved = actions.find(q => answersOf(state, item)[q.id] === true);
  if (approved) return { kind: "approved", label: approved.states.on?.badge ?? approved.label ?? "Approved" };
  return isReviewed(suite, state, item) ? { kind: "reviewed" } : null;
}

// pendingActions are approved and not yet done: the agent performs these after a send.
export function pendingActions(suite, state) {
  return suite.items.flatMap(item =>
    actionsFor(suite, state, item)
      .filter(q => !doneOf(item, q.id) && answersOf(state, item)[q.id] === true)
      .map(question => ({ item, question })));
}

const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);
const isRowMap = v => v !== null && typeof v === "object" && !Array.isArray(v);

// liveAnswers drops answers whose `when` condition no longer holds.
function liveAnswers(state, item, questions, answers) {
  const out = {};
  for (const [qid, a] of Object.entries(answers)) {
    const q = questions.find(x => x.id === qid);
    if (!q?.when) out[qid] = a;
    else if (q.each) {
      const rows = Object.fromEntries(Object.entries(a).filter(([row]) => asked(state, item, questions, q, row)));
      if (Object.keys(rows).length) out[qid] = rows;
    } else if (asked(state, item, questions, q)) out[qid] = a;
  }
  return out;
}

export function snapshot(suite, state) {
  const items = {};
  for (const item of suite.items) {
    const e = entry(state, item);
    if (!hasInput(e)) continue;
    const changed = {};
    for (const [qid, p] of Object.entries(item.proposed || {})) {
      const a = e.answers[qid];
      if (a === undefined) continue;
      if (isRowMap(p.value)) {
        const rows = Object.fromEntries(Object.entries(a)
          .filter(([r, v]) => p.value[r] !== undefined && !same(p.value[r], v))
          .map(([r, v]) => [r, { from: p.value[r], to: v }]));
        if (Object.keys(rows).length) changed[qid] = rows;
      } else if (!same(a, p.value)) changed[qid] = { from: p.value, to: a };
    }
    items[item.id] = {
      title: item.title,
      revision: item.revision,
      reviewed: isReviewed(suite, state, item),
      answers: liveAnswers(state, item, questionsFor(suite, item), e.answers),
      edits: e.edits,
      ...(Object.keys(changed).length ? { changed } : {}),
    };
  }
  return {
    suite: suite.title,
    progress: progress(suite, state),
    actions: pendingActions(suite, state).map(({ item, question }) => ({ item: item.id, action: question.id, title: item.title })),
    items,
  };
}

export const measure = text => (text || "").trim().length;

// limitTone: within the target (or under it) is positive, up to max is a warning, past max is negative.
export function limitTone(n, limit) {
  const hi = limit.target?.[1] ?? limit.max;
  if (hi !== undefined && n <= hi) return "positive";
  if (limit.max === undefined || n <= limit.max) return "warning";
  return "negative";
}
