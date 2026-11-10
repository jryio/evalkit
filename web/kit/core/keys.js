// Keyboard bindings are data: built from the suite and item, matched against key
// events, checked for conflicts and rendered as the help sheet.

const lower = s => (s || "").toLowerCase();

// rows is how many rows the keyboard cursor can visit on this item (see rowTargets).
export function bindings(suite, item, questions, rows = 0) {
  const out = [];
  const add = (keys, label, action, row) => out.push({ keys, label, action, row });

  add(["j", "ArrowRight"], "Next item", { type: "move", by: 1 }, { id: "nav", label: "Next / previous item" });
  add(["k", "ArrowLeft"], "Previous item", { type: "move", by: -1 }, { id: "nav" });
  if (rows) {
    add(["ArrowDown"], "Next row", { type: "row", by: 1 }, { id: "rows", label: "Next / previous row" });
    add(["ArrowUp"], "Previous row", { type: "row", by: -1 }, { id: "rows" });
  }

  const primary = questions.find(q => q.primary);
  const primaryRow = primary && { id: primary.id, label: `${primary.label}: ${primary.options.map(o => lower(o.label)).join(", ")}` };
  primary?.options.forEach((o, i) =>
    add([String(i + 1)], `${primary.label}: ${o.label}`, { type: "set", qid: primary.id, value: o.value }, primaryRow));

  for (const q of questions) {
    const perRow = !!q.each;
    const label = text => (perRow ? `${text} (current row)` : text);
    const k = q.key;
    if (k && q.kind === "choice") add([k], label(`Change ${q.label}, or ${keyName(k)} then 1–${q.options.length}`), { type: "cycle", qid: q.id, perRow });
    if (k && q.kind === "scale") add([k], label(`${q.label}: ${keyName(k)} then ${q.min ?? 1}–${q.max ?? 5}`), { type: "arm", qid: q.id, perRow });
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
// adds the fixed keys the app handles itself.
export function helpRows(list, sendLabel) {
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
  rows.push({ keys: ["Esc"], also: [], label: "Stop typing or close a sheet" });
  rows.push({ keys: ["⌘", "↵"], also: [], label: sendLabel });
  return rows;
}
