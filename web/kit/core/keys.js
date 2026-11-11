// Keyboard bindings are data: built from the suite and item, matched against key
// events, checked for conflicts and rendered as the help sheet.

const lower = s => (s || "").toLowerCase();

// rows is how many rows the keyboard cursor can visit on this item (see rowTargets);
// runs is how many runs the suite has.
export function bindings(suite, item, questions, rows = 0, runs = 0) {
  const out = [];
  const add = (keys, label, action, row) => out.push({ keys, label, action, row });

  add(["j", "ArrowRight"], "Next item", { type: "move", by: 1 }, { id: "nav", label: "Next / previous item" });
  add(["k", "ArrowLeft"], "Previous item", { type: "move", by: -1 }, { id: "nav" });
  if (rows) {
    add(["ArrowDown"], "Next row", { type: "row", by: 1 }, { id: "rows", label: "Next / previous row" });
    add(["ArrowUp"], "Previous row", { type: "row", by: -1 }, { id: "rows" });
  }
  runKeys(add, runs);

  primaryKeys(add, questions.find(q => q.primary), "");

  for (const q of questions) {
    const perRow = !!q.each;
    const label = text => (perRow ? `${text} (current row)` : text);
    const k = q.key;
    if (k && q.kind === "choice") add([k], label(`Change ${q.label}, or ${keyName(k)} then 1–${q.options.length}`), { type: "cycle", qid: q.id, perRow });
    if (k && q.kind === "scale") add([k], label(`${q.label}: ${keyName(k)} then ${q.min ?? 1}–${q.max ?? 5}`), { type: "arm", qid: q.id, perRow });
    if (k && q.kind === "multi") add([k], label(`${q.label}: ${keyName(k)} then 1–${Math.min(9, q.options.length)} to toggle`), { type: "arm", qid: q.id, perRow });
    if (k && q.kind === "toggle") add([k], label(q.label), { type: "toggle", qid: q.id, perRow });
    if (k && q.kind === "action" && !item.done?.[q.id]) add([k], q.states.off.button, { type: "toggle", qid: q.id });
    if (k && (q.kind === "text" || q.kind === "number")) add([k], q.label, { type: "focus", target: `q:${q.id}` });
    if (k && q.kind === "pick") add([k], `${q.label}: the current row`, { type: "pick", qid: q.id });
    for (const o of q.options || []) {
      if (o.key) add([o.key], label(o.label), { type: "option", qid: q.id, value: o.value, perRow });
    }
  }
  for (const a of [...(item.show || []), ...(item.context || [])]) {
    if (a.editable) add([a.key || "e"], `Edit ${a.label || "text"}`, { type: "focus", target: `a:${a.id ?? "edit"}` });
  }
  add(["?"], "Keyboard shortcuts", { type: "help" });
  return out;
}

// primaryKeys binds 1–9 to the primary question's options, when it is on screen.
function primaryKeys(add, primary, suffix) {
  const row = primary && { id: primary.id, label: `${primary.label}${suffix}: ${primary.options.map(o => lower(o.label)).join(", ")}` };
  primary?.options.forEach((o, i) =>
    add([String(i + 1)], `${primary.label}: ${o.label}`, { type: "set", qid: primary.id, value: o.value }, row));
}

// runKeys step through the runs, when there is more than one.
function runKeys(add, runs) {
  if (runs < 2) return;
  add(["["], "Previous run", { type: "run", by: -1 }, { id: "runs", label: "Previous / next run" });
  add(["]"], "Next run", { type: "run", by: 1 }, { id: "runs" });
}

// tableBindings are the table view's keys: move the selection, open it, answer its
// primary question, filter. questions are those on screen for the selected row.
export function tableBindings(questions, runs = 0) {
  const out = [];
  const add = (keys, label, action, row) => out.push({ keys, label, action, row });
  add(["ArrowDown", "j"], "Next row", { type: "select", by: 1 }, { id: "rows", label: "Next / previous row" });
  add(["ArrowUp", "k"], "Previous row", { type: "select", by: -1 }, { id: "rows" });
  runKeys(add, runs);
  add(["Enter"], "Open the row for review", { type: "open" });
  primaryKeys(add, questions.find(q => q.primary), " of the selected row");
  add(["/"], "Filter items", { type: "focus", target: "table-search" });
  add(["?"], "Keyboard shortcuts", { type: "help" });
  return out;
}

// runsBindings are the Runs view's keys: move the run selection through the runs table,
// open the selected run in the table, filter.
export function runsBindings(runs = 0) {
  const out = [];
  const add = (keys, label, action, row) => out.push({ keys, label, action, row });
  add(["ArrowDown", "j"], "Select the run below", { type: "selectRun", by: 1 }, { id: "rows", label: "Select the run below / above" });
  add(["ArrowUp", "k"], "Select the run above", { type: "selectRun", by: -1 }, { id: "rows" });
  runKeys(add, runs);
  add(["Enter"], "Open the selected run in the table", { type: "openRun" });
  add(["/"], "Filter runs", { type: "focus", target: "runs-search" });
  add(["?"], "Keyboard shortcuts", { type: "help" });
  return out;
}

// panelBindings are the table's keys while a panel shows the selected row: the item's own
// keys, with J/K and ←/→ moving the selection (and ↑/↓ too, unless the item has rows to
// answer) and / filtering.
export function panelBindings(suite, item, questions, rows = 0, runs = 0) {
  const out = bindings(suite, item, questions, rows, runs).map(b => b.action.type !== "move" ? b : {
    ...b,
    label: b.action.by > 0 ? "Next row" : "Previous row",
    action: { type: "select", by: b.action.by },
    row: { id: "nav", label: "Next / previous row" },
  });
  if (!rows) {
    out.push({ keys: ["ArrowDown"], label: "Next row", action: { type: "select", by: 1 }, row: { id: "nav" } });
    out.push({ keys: ["ArrowUp"], label: "Previous row", action: { type: "select", by: -1 }, row: { id: "nav" } });
  }
  out.push({ keys: ["/"], label: "Filter items", action: { type: "focus", target: "table-search" } });
  return out;
}

export function match(list, event) {
  const key = event.key.length === 1 ? event.key.toLowerCase() : event.key;
  return list.find(b => b.keys.includes(key));
}

export function conflicts(list) {
  const seen = new Map();
  const out = [];
  for (const b of list) {
    for (const k of b.keys) {
      if (seen.has(k)) out.push({ key: k, first: seen.get(k).label, second: b.label });
      else seen.set(k, b);
    }
  }
  return out;
}

const NAMES = { ArrowRight: "→", ArrowLeft: "←", ArrowUp: "↑", ArrowDown: "↓", Escape: "Esc", Enter: "↵" };
export const keyName = k => NAMES[k] || (k.length === 1 ? k.toUpperCase() : k);

// helpRows merges bindings that share a row (number keys of one question, J/K) and
// adds the fixed keys the app handles itself. Esc climbs from review to the table, and
// from the table to runs when the suite has runs.
export function helpRows(list, sendLabel, view = "review", runs = 0) {
  const rows = [];
  const byId = new Map();
  for (const b of list) {
    const [first, ...also] = b.keys.map(keyName);
    const existing = b.row && byId.get(b.row.id);
    if (existing) {
      existing.keys.push(first);
      existing.also.push(...also);
      continue;
    }
    const row = { keys: [first], also, label: b.row?.label ?? b.label };
    if (b.row) byId.set(b.row.id, row);
    rows.push(row);
  }
  const up = view === "review" ? "the table" : view === "table" && runs ? "runs" : null;
  rows.push({ keys: ["Esc"], also: [], label: `Stop typing or close a sheet${up ? `; then back to ${up}` : ""}` });
  rows.push({ keys: ["⌘", "↵"], also: [], label: sendLabel });
  return rows;
}
