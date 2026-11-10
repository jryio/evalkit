// Review state, saved on the server:
//   { items: { [itemId]: { revision, answers: { [qid]: value }, edits: { [artifactId]: text } } },
//     history: { [itemId]: [ { revision, answers, edits, replaced_at } ] } }
// Every function returns a new state; nothing mutates its input.

export const emptyState = () => ({ items: {}, history: {} });

// null is an answer: "explicitly empty", used when clearing a field the agent had proposed.
const isEmpty = v =>
  v === undefined || v === "" ||
  (Array.isArray(v) && v.length === 0) ||
  (v !== null && typeof v === "object" && !Array.isArray(v) && Object.keys(v).length === 0);

// entry is the item's state for its current revision; answers for an older revision do not apply.
export function entry(state, item) {
  const e = state.items[item.id];
  return e && e.revision === item.revision ? e : { revision: item.revision, answers: {}, edits: {} };
}

export const hasInput = e => !!e && (Object.keys(e.answers || {}).length > 0 || Object.keys(e.edits || {}).length > 0);

function update(state, item, fn) {
  return { ...state, items: { ...state.items, [item.id]: fn(entry(state, item)) } };
}

function put(obj, key, value) {
  if (isEmpty(value)) delete obj[key];
  else obj[key] = value;
  return obj;
}

// setAnswer stores a question's answer; undefined, "" and [] clear it. With row, the
// answer is one entry in a { rowId: value } map (per-row questions).
export function setAnswer(state, item, qid, value, row) {
  return update(state, item, e => {
    const answers = { ...e.answers };
    if (row !== undefined) value = put({ ...(answers[qid] || {}) }, row, value);
    put(answers, qid, value);
    return { ...e, answers };
  });
}

// setEdit stores an edited artifact; text equal to the original clears the edit.
export function setEdit(state, item, artifactId, text, original) {
  return update(state, item, e => {
    const edits = { ...e.edits };
    if (text === original) delete edits[artifactId];
    else edits[artifactId] = text;
    return { ...e, edits };
  });
}

// reconcile moves state for an older item revision into history, so a revised item
// starts a fresh review without losing what the human said about the old one.
export function reconcile(state, items, now = new Date().toISOString()) {
  const next = { items: { ...state.items }, history: { ...(state.history || {}) } };
  let changed = false;
  for (const item of items) {
    const e = next.items[item.id];
    if (!e || e.revision === item.revision) continue;
    if (hasInput(e)) next.history[item.id] = [...(next.history[item.id] || []), { ...e, replaced_at: now }];
    delete next.items[item.id];
    changed = true;
  }
  return { state: next, changed };
}
