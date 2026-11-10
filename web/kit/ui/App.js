// App owns the loaded suite, the review state, navigation, autosave, sending,
// the keyboard and live reload of the suite.
import { html, useState, useEffect, useRef, useMemo } from "../html.js";
import { api } from "../core/api.js";
import { emptyState, reconcile, setAnswer, setEdit } from "../core/state.js";
import { orderedItems, questionsFor, valueOf, answersOf, asked, progress, pendingActions, snapshot, rowTargets } from "../core/derive.js";
import { scaleRange } from "../controls/scale.js";
import { bindings, match } from "../core/keys.js";
import { loadSuiteRenderers } from "../registry.js";
import { fmtTime } from "./bits.js";
import { Sidebar } from "./Sidebar.js";
import { ItemView } from "./ItemView.js";
import { Toolbar, SendSheet, HelpSheet, Toast, Banner } from "./chrome.js";

const isTyping = el => !!el?.matches?.("textarea, input, select, [contenteditable]");
const hashId = () => decodeURIComponent(location.hash.slice(1)) || null;

export function App() {
  const [loaded, setLoaded] = useState(null); // { suite, problems, version, fatal, done }
  const [state, setState] = useState(null);
  const [currentId, setCurrentId] = useState(hashId); // the item on screen, by id so suite edits never move it
  const [sheet, setSheet] = useState(null); // "send" | "help" | null
  const [saveLabel, setSaveLabel] = useState("");
  const [toast, setToast] = useState({ message: "", show: false });
  const [stale, setStale] = useState(false);
  const [loadErrors, setLoadErrors] = useState([]);
  const [cursor, setCursor] = useState(null); // { artifact, row } under the keyboard row cursor
  const armed = useRef(null); // { qid, row, until }: a question key waiting for a number
  const dirty = useRef(false);
  const rev = useRef(0); // the server's state revision this tab last saw
  const saving = useRef(Promise.resolve()); // saves run one at a time, in order
  const saveTimer = useRef();
  const scroller = useRef();

  const suite = loaded?.suite;
  const items = useMemo(() => (suite ? orderedItems(suite) : []), [suite]);
  const found = items.findIndex(i => i.id === currentId);
  const index = found >= 0 ? found : 0;
  const item = items[index];

  const showToast = message => {
    setToast({ message, show: true });
    clearTimeout(showToast.t);
    showToast.t = setTimeout(() => setToast(t => ({ ...t, show: false })), 3200);
  };

  // refresh loads the suite and moves answers for revised items into history.
  async function refresh() {
    const res = await api.suite();
    // Action results recorded with `evalkit done` arrive beside the suite.
    for (const it of res.suite?.items ?? []) if (res.done?.[it.id]) it.done = { ...it.done, ...res.done[it.id] };
    if (res.suite) setLoadErrors(await loadSuiteRenderers(res.suite.renderers));
    setLoaded(res);
    setStale(false);
    if (res.suite) {
      setState(s => {
        if (!s) return s;
        const r = reconcile(s, res.suite.items);
        if (r.changed) dirty.current = true;
        return r.changed ? r.state : s;
      });
    }
  }

  async function loadState() {
    const saved = await api.state().catch(() => ({}));
    rev.current = saved.rev ?? 0;
    setState({ ...emptyState(), ...saved });
  }

  useEffect(() => {
    (async () => {
      await loadState();
      await refresh();
    })().catch(err => setLoaded({ fatal: true, problems: [{ message: err.message }] }));
    const onHash = () => setCurrentId(hashId());
    addEventListener("hashchange", onHash);
    return () => removeEventListener("hashchange", onHash);
  }, []);

  // save posts the state with the revision this tab last saw. A 409 means another
  // tab saved since: reload the newest state instead of overwriting it.
  function save(st) {
    saving.current = saving.current.then(async () => {
      try {
        rev.current = (await api.saveState({ ...st, rev: rev.current })).rev;
      } catch (err) {
        if (err.status !== 409) throw err;
        await loadState();
        await refresh();
        showToast("This review changed in another tab. Showing the newest answers.");
      }
    });
    return saving.current;
  }

  // Autosave 400 ms after the last change. The server is the source of truth.
  const change = fn => {
    dirty.current = true;
    setSaveLabel("Saving…");
    setState(s => fn(s));
  };
  useEffect(() => {
    if (!dirty.current) return;
    saveTimer.current = setTimeout(() => {
      save(state).then(() => { dirty.current = false; setSaveLabel("Saved"); }, () => setSaveLabel("Not saved: server stopped"));
    }, 400);
    return () => clearTimeout(saveTimer.current);
  }, [state]);

  // Live reload: poll the suite version; apply quietly unless the human is mid-edit.
  const live = useRef({});
  live.current = { loaded, sheet };
  useEffect(() => {
    const t = setInterval(async () => {
      const { loaded, sheet } = live.current;
      if (!loaded?.version) return;
      const v = await api.version().catch(() => null);
      if (!v || v.version === loaded.version) return;
      if (isTyping(document.activeElement) || dirty.current || sheet) setStale(true);
      else refresh();
    }, 3000);
    return () => clearInterval(t);
  }, []);

  useEffect(() => {
    setCursor(null);
    if (scroller.current) scroller.current.scrollTop = 0;
  }, [item?.id]);

  const go = i => {
    if (i < 0 || i >= items.length) return;
    setCurrentId(items[i].id);
    history.replaceState(null, "", `#${encodeURIComponent(items[i].id)}`);
  };

  const act = {
    answer: (it, qid, value, row) => change(s => setAnswer(s, it, qid, value, row)),
    edit: (it, a, text) => change(s => setEdit(s, it, a.id ?? "edit", text, a.text)),
  };

  async function confirmSend() {
    setSheet(null);
    clearTimeout(saveTimer.current);
    try {
      await saving.current.catch(() => {});
      const res = await api.submit({ ...state, rev: rev.current }, snapshot(suite, state));
      rev.current = res.rev;
      dirty.current = false;
      setSaveLabel(`Sent at ${fmtTime(new Date())}`);
      showToast("Sent. Keep going and send again anytime.");
    } catch (err) {
      if (err.status === 409) {
        await loadState();
        await refresh();
        showToast("This review changed in another tab. Check the newest answers, then send again.");
      } else {
        showToast("Couldn't reach the server. Your work is still on this page.");
      }
    }
  }

  const questions = item ? questionsFor(suite, item) : [];
  // Keys only reach questions that are on screen: `when` can hide one.
  const onScreen = item ? questions.filter(q => q.each || asked(state, item, questions, q)) : [];
  const targets = item ? rowTargets(item, questions) : [];
  const binds = item ? bindings(suite, item, onScreen, targets.length) : [];

  // The row a per-row key applies to: the cursor row, else the first row of q's artifact.
  function rowFor(q) {
    const source = q.each ?? q.from;
    if (source !== undefined && cursor?.artifact === source) return cursor.row;
    const first = targets.find(t => t.artifact === source);
    if (first) setCursor(first);
    return first?.row;
  }

  function run(action) {
    const q = questions.find(x => x.id === action.qid);
    const row = action.perRow ? rowFor(q) : undefined;
    if (action.perRow && row === undefined) return;
    // value is what shows (answer, else proposal); answer is what the human set.
    const value = q && valueOf(state, item, q, row);
    const answer = q && (row === undefined ? answersOf(state, item)[q.id] : answersOf(state, item)[q.id]?.[row]);
    const proposed = q && (row === undefined ? item.proposed?.[q.id]?.value : item.proposed?.[q.id]?.value?.[row]);
    const set = v => act.answer(item, q.id, v, row);
    switch (action.type) {
      case "move": return go(index + action.by);
      case "help": return setSheet("help");
      case "row": {
        const i = targets.findIndex(t => t.artifact === cursor?.artifact && t.row === cursor?.row);
        return setCursor(targets[i < 0 ? 0 : Math.max(0, Math.min(targets.length - 1, i + action.by))]);
      }
      case "set": return set(answer === action.value ? undefined : action.value);
      // Turning off a proposed "yes" stores an explicit false, so the proposal does not come back.
      case "toggle": return set(value === true ? (proposed === true ? false : undefined) : true);
      case "arm":
        armed.current = { qid: q.id, row, until: Date.now() + 1500 };
        return;
      case "cycle": {
        armed.current = { qid: q.id, row, until: Date.now() + 1500 };
        const i = q.options.findIndex(o => o.value === value);
        return set(q.options[(i + 1) % q.options.length].value);
      }
      case "option": {
        if (q.kind !== "multi") return set(answer === action.value ? undefined : action.value);
        const list = value || [];
        const next = list.includes(action.value) ? list.filter(v => v !== action.value) : [...list, action.value];
        return set(next.length ? next : proposed !== undefined ? null : undefined);
      }
      case "pick": {
        const picked = rowFor(q);
        if (picked !== undefined) act.answer(item, q.id, answer === picked ? undefined : picked);
        return;
      }
      case "focus": {
        const el = document.querySelector(`[data-focus="${action.target}"]`);
        el?.focus();
        el?.setSelectionRange?.(el.value.length, el.value.length);
      }
    }
  }

  // pickNumber completes a chord: a choice's key then n picks option n; a scale's key then n sets n.
  function pickNumber(n) {
    const { qid, row } = armed.current;
    armed.current = null;
    const q = questions.find(x => x.id === qid);
    const value = q.kind === "scale" ? scaleRange(q).find(v => v === n) : q.options[n - 1]?.value;
    if (value !== undefined) act.answer(item, q.id, value, row);
  }

  useEffect(() => {
    if (cursor) document.querySelector(`[data-row="${cursor.artifact}:${cursor.row}"]`)?.scrollIntoView({ block: "nearest" });
  }, [cursor]);

  const onKey = useRef();
  onKey.current = e => {
    if (!suite) return;
    const typing = isTyping(e.target);
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      if (e.repeat) return; // holding ⌘↵ must not open the sheet and then confirm it
      return sheet === "send" ? confirmSend() : setSheet("send");
    }
    if (e.key === "Escape") return sheet ? setSheet(null) : typing ? e.target.blur() : setCursor(null);
    if (sheet) {
      // Enter confirms, except on a focused Cancel button, which cancels.
      if (e.key === "Enter" && !e.repeat) {
        e.preventDefault();
        if (e.target.closest?.("[data-role=cancel]") || sheet !== "send") setSheet(null);
        else confirmSend();
      }
      return;
    }
    if (typing || e.metaKey || e.ctrlKey || e.altKey || !item) return;
    if (/^[1-9]$/.test(e.key) && armed.current && Date.now() < armed.current.until) {
      e.preventDefault();
      return pickNumber(Number(e.key));
    }
    armed.current = null;
    const b = match(binds, e);
    if (b) {
      e.preventDefault();
      run(b.action);
    }
  };
  useEffect(() => {
    const h = e => onKey.current(e);
    document.addEventListener("keydown", h);
    return () => document.removeEventListener("keydown", h);
  }, []);

  if (!loaded) return null;
  if (loaded.fatal) return html`<div class="fatal">
    <h1>This suite can't be shown</h1>
    <p>Fix these problems and reload. <code>evalkit validate</code> prints the same list.</p>
    <pre>${loaded.problems.map(p => `${p.file ?? ""}${p.line ? `:${p.line}:${p.col}` : ""} ${p.path ?? ""} ${p.message}`).join("\n")}</pre>
  </div>`;
  if (!state) return null;

  const errors = loaded.problems.filter(p => !p.warning);
  const sendLabel = suite.send ?? "Send";
  const actionQs = [...new Map(items.flatMap(i => questionsFor(suite, i)).filter(q => q.kind === "action").map(q => [q.id, q])).values()];

  return html`<div class="app">
    <${Sidebar} suite=${suite} items=${items} state=${state} current=${index} go=${go} openHelp=${() => setSheet("help")} />
    <main class="main">
      <${Toolbar} index=${index} count=${items.length} go=${go} saveLabel=${saveLabel} sendLabel=${sendLabel} openSend=${() => setSheet("send")} />
      <div class="scroll" ref=${scroller}><div class="content">
        ${stale ? html`<${Banner} tone="accent" title="The suite changed." action=${html`<button class="btn-secondary" onClick=${() => refresh()}>Refresh</button>`} />` : null}
        ${errors.length ? html`<${Banner} tone="negative" title=${`${errors.length} problem${errors.length > 1 ? "s" : ""} in the suite. Items with errors are hidden.`}
            lines=${errors.map(p => `${p.item ? `${p.item}: ` : ""}${p.path ? `${p.path}: ` : ""}${p.message} (line ${p.line})`)} />` : null}
        ${loadErrors.length ? html`<${Banner} tone="negative" title="Custom renderers failed to load." lines=${loadErrors} />` : null}
        ${item ? html`<${ItemView} key=${item.id} suite=${suite} item=${item} state=${state} act=${act} cursor=${cursor} />` : html`<p class="muted">This suite has no items.</p>`}
      </div></div>
    </main>
    ${sheet === "send" ? html`<${SendSheet} sendLabel=${sendLabel} progress=${progress(suite, state)} actions=${actionQs}
        pending=${pendingActions(suite, state)} close=${() => setSheet(null)} confirm=${confirmSend} />` : null}
    ${sheet === "help" ? html`<${HelpSheet} binds=${binds} sendLabel=${sendLabel} description=${suite.description} close=${() => setSheet(null)} />` : null}
    <${Toast} ...${toast} />
  </div>`;
}
