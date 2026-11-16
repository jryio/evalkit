// App owns the loaded suite, the review state, the view (runs, table or review) with its
// filters, the run shown, navigation, autosave, sending, the keyboard and live reload of the suite.
import { html, useState, useEffect, useRef, useMemo } from "../html.js";
import { api } from "../core/api.js";
import { emptyState, reconcile, setAnswer, setEdit } from "../core/state.js";
import { orderedItems, questionsFor, valueOf, answersOf, asked, progress, pendingActions, snapshot, rowTargets } from "../core/derive.js";
import { columns, visible, scopeCounts } from "../core/table.js";
import { acrossRuns, findRuns, metricColumn, metricOf, pairsOf, runList, spendOf, summarize, vsOf, withBaselines } from "../core/runs.js";
import { scaleRange } from "../controls/scale.js";
import { bindings, tableBindings, panelBindings, runsBindings, match } from "../core/keys.js";
import { loadSuiteRenderers } from "../registry.js";
import { cx, fmtTime } from "./bits.js";
import { Sidebar } from "./Sidebar.js";
import { ItemView } from "./ItemView.js";
import { ItemsTable } from "./ItemsTable.js";
import { RunsView } from "./RunsView.js";
import { ReviewPanel } from "./ReviewPanel.js";
import { Toolbar, SuiteHead, SendSheet, HelpSheet, Toast, Banner } from "./chrome.js";

// Only fields the reviewer types into keep the keys; sliders, checkboxes and buttons pass them on.
const isTyping = el => !!el?.matches?.("textarea, select, [contenteditable], input:not([type=range], [type=checkbox], [type=radio], [type=button], [type=submit], [type=reset], [type=color], [type=file])");
const hashId = () => decodeURIComponent(location.hash.slice(1)) || null;
const NO_FILTERS = { scope: "all", group: null, search: "", where: [], sort: null };
const NO_RUN_FILTERS = { search: "", group: null, judgeOnly: false };
const VIEWS = ["runs", "table", "review"];
// The table's Δ filter for the cases a run moved up, kept the same or moved down.
const DIRECTION_TONES = { up: "positive", same: "neutral", down: "negative" };
const OPEN_IN = ["sidebar", "floating", "full"];
const savedOpenIn = () => OPEN_IN.find(m => m === localStorage.getItem("evalkit.openIn")) ?? "sidebar";

// Below this window width the table's review panel opens as a bottom sheet.
const SHEET_AT = "(max-width: 1000px)";
function useNarrow() {
  const [narrow, setNarrow] = useState(() => matchMedia(SHEET_AT).matches);
  useEffect(() => {
    const mq = matchMedia(SHEET_AT);
    const on = () => setNarrow(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return narrow;
}

// runSuite is the suite as the table and review show it: one run's items, when it has runs, in sidebar order.
function runSuite(suite, state, runs, run) {
  const s = run ? withBaselines(suite, state, run.id, vsOf(suite, runs, run)?.id ?? null) : suite;
  return { ...s, items: orderedItems(s) };
}

// animated applies update, a change of view, as a view transition: the sidebar slides away or back
// while the main column takes its space (kit.css). Preact renders in a microtask, so the new view is
// in the DOM by the next task. Without the API, or with reduced motion, the view switches at once.
// A switch made during another's transition skips it, and the browser may still run the skipped
// update after the newer one, so only the latest update applies.
let latestSwitch = 0;
function animated(update) {
  if (!document.startViewTransition || matchMedia("(prefers-reduced-motion: reduce)").matches) return update();
  const id = ++latestSwitch;
  document.startViewTransition(() => {
    if (id === latestSwitch) update();
    return new Promise(resolve => setTimeout(resolve));
  });
}

export function App() {
  const [loaded, setLoaded] = useState(null); // { suite, problems, version, fatal, done }
  const [navOpen, setNavOpen] = useState(false); // the sidebar drawer, on narrow screens
  const narrow = useNarrow(); // below SHEET_AT the review panel is a bottom sheet
  const [state, setState] = useState(null);
  const [currentId, setCurrentId] = useState(hashId); // the item on screen, by id so suite edits never move it
  const [sheet, setSheet] = useState(null); // "send" | "help" | null
  const [saveLabel, setSaveLabel] = useState("");
  const [toast, setToast] = useState({ message: "", show: false });
  const [stale, setStale] = useState(false);
  const [loadErrors, setLoadErrors] = useState([]);
  const [cursor, setCursor] = useState(null); // { artifact, row } under the keyboard row cursor
  const [mode, setModeState] = useState(() => VIEWS.find(v => v === new URLSearchParams(location.search).get("view")) ?? null); // null: the suite's default
  const [filters, setFilters] = useState(NO_FILTERS); // the table's scope, group, search, column filters and sort
  const [walk, setWalk] = useState(null); // ids review walks, pinned from the filtered table; null: every item
  const [openIn, setOpenInState] = useState(savedOpenIn); // where Enter opens a table row: sidebar, floating or full
  const [panel, setPanel] = useState(false); // the table's review panel is open
  const [runParam, setRunParam] = useState(() => new URLSearchParams(location.search).get("run")); // null: the linked item's run, else the latest
  const [runFilters, setRunFilters] = useState(NO_RUN_FILTERS); // the Runs view's search, group and judge-only switch
  const selIndex = useRef(0); // the selected row's position, so the selection survives the row leaving the filter
  const armed = useRef(null); // { qid, row, until }: a question key waiting for a number
  const dirty = useRef(false);
  const rev = useRef(0); // the server's state revision this tab last saw
  const saving = useRef(Promise.resolve()); // saves run one at a time, in order
  const saveTimer = useRef();
  const scroller = useRef();

  const suite = loaded?.suite;
  const runs = useMemo(() => (suite ? runList(suite) : []), [suite]);
  const linkedRun = suite?.items.find(i => i.id === currentId)?.run;
  const shownRun = runs.find(r => r.id === runParam) ?? runs.find(r => r.id === linkedRun) ?? runs.at(-1);
  const vs = shownRun ? vsOf(suite, runs, shownRun) : null;
  // A suite opens on the view it asks for; else one with runs opens on the table, one without on review.
  // Runs exists only with runs: asked for without them, the page shows the table.
  const wanted = mode ?? suite?.view;
  const views = runs.length ? VIEWS : VIEWS.slice(1);
  const view = views.includes(wanted) ? wanted : wanted === "runs" || runs.length ? "table" : "review";
  // ordered is the suite as shown, with items in sidebar order, the base for the table's rows.
  // Baselines are the compared run's answers, so it follows the state.
  // Without runs it ignores the state, so answers must not rebuild it.
  const baseState = shownRun ? state : !!state;
  const ordered = useMemo(() => suite && state && runSuite(suite, state, runs, shownRun), [suite, baseState, runs, shownRun]);
  const items = ordered?.items ?? [];
  const cols = useMemo(() => (ordered ? columns(ordered) : []), [ordered]);
  const rows = useMemo(() => (ordered && state ? visible(ordered, state, cols, filters) : []), [ordered, state, cols, filters]);
  const filtered = filters !== NO_FILTERS;
  // Review walks the table's rows as they were when review opened, so answering an item
  // out of the filter does not change what J and K visit.
  const list = useMemo(() => {
    if (!walk) return items;
    const byId = new Map(items.map(i => [i.id, i]));
    return walk.map(id => byId.get(id)).filter(Boolean);
  }, [walk, items]);
  const found = list.findIndex(i => i.id === currentId);
  const index = found >= 0 ? found : 0;
  // In the table, keys act on the panel's item, else on the selected row while it is visible.
  const panelOpen = view === "table" && panel && openIn !== "full";
  const item = view === "review" ? list[index] : view === "runs" ? null : panelOpen ? items.find(i => i.id === currentId) : rows.find(r => r.id === currentId);
  const metric = useMemo(() => suite && metricOf(suite), [suite]);
  const spend = useMemo(() => suite && spendOf(suite), [suite]);
  const reviewing = view === "review" || panelOpen;
  const across = useMemo(() => (reviewing && item?.pair !== undefined && runs.length > 1 ? acrossRuns(suite, state, item, metric) : null), [reviewing, suite, state, item, metric, runs]);
  const paired = useMemo(() => (shownRun && vs ? pairsOf(suite, shownRun.id, vs.id).size : 0), [suite, shownRun, vs]);
  // The Runs view's rows in run order, and those its filter shows, newest first.
  const { judgeOnly, group } = runFilters;
  const summary = useMemo(() => (view === "runs" && state ? summarize(suite, state, { judgeOnly, group }) : []), [view, suite, state, judgeOnly, group]);
  const runRows = useMemo(() => findRuns(summary, runFilters.search), [summary, runFilters.search]);

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

  const title = suite?.title && (shownRun ? `${suite.title} · ${shownRun.label}` : suite.title);
  useEffect(() => {
    if (title) document.title = `${title} · evalkit`;
  }, [title]);

  useEffect(() => {
    setCursor(null);
    if (scroller.current) scroller.current.scrollTop = 0;
  }, [item?.id]);

  const show = id => {
    setCurrentId(id);
    history.replaceState(null, "", `${location.search}#${encodeURIComponent(id)}`);
  };
  const go = i => {
    if (i >= 0 && i < list.length) show(list[i].id);
  };
  const setView = v => {
    if (v === "review") setWalk(filtered ? rows.map(r => r.id) : null);
    const url = new URL(location.href);
    url.searchParams.set("view", v);
    history.replaceState(null, "", url);
    setModeState(v);
  };
  const switchView = v => animated(() => setView(v));
  const open = id => animated(() => {
    show(id);
    setView("review");
  });
  // openRow opens a table row where the reviewer chose: the panel, or full screen.
  const openRow = id => {
    if (openIn === "full") return open(id);
    show(id);
    setPanel(true);
  };
  const setOpenIn = mode => {
    localStorage.setItem("evalkit.openIn", mode);
    setOpenInState(mode);
    if (mode === "full" && panel && currentId) open(currentId);
  };
  // setRun shows another run on the same case (itemId, else the selected item's pair), else on
  // its first row. Review walks the whole run: the pinned walk was the other run's rows.
  const setRun = (id, itemId) => {
    const to = runs.find(r => r.id === id);
    if (!to) return;
    const url = new URL(location.href);
    url.searchParams.set("run", id);
    history.replaceState(null, "", url);
    setRunParam(id);
    setWalk(null);
    const next = runSuite(suite, state, runs, to);
    const pair = items.find(i => i.id === currentId)?.pair;
    const first = view === "review" ? next.items[0] : visible(next, state, columns(next), filters)[0];
    const target = itemId ?? next.items.find(i => pair !== undefined && i.pair === pair)?.id ?? first?.id;
    if (target) show(target);
  };
  const stepRun = by => {
    const to = runs[shownRun.index + by];
    if (to) setRun(to.id);
  };
  // selectRun moves the Runs view's selection through its rows; a run the filter hides restarts at the top.
  const selectRun = by => {
    const at = runRows.findIndex(r => r.run.id === shownRun.id);
    const next = runRows[at < 0 ? 0 : Math.max(0, Math.min(runRows.length - 1, at + by))];
    if (next) setRun(next.run.id);
  };
  // openRun opens a run in the table: on one case (pair) in the review panel, or filtered
  // to the cases that moved in one direction against the run it is compared with.
  const openRun = (id, { pair, direction } = {}) => animated(() => {
    const full = pair !== undefined && openIn === "full";
    setView(full ? "review" : "table"); // before setRun, which unpins review's walk
    if (direction && metric) {
      const f = { ...NO_FILTERS, group, where: [{ column: `d:${metricColumn(metric)}`, tone: DIRECTION_TONES[direction] }] };
      const next = runSuite(suite, state, runs, runs.find(r => r.id === id));
      setFilters(f);
      setRun(id, visible(next, state, columns(next), f)[0]?.id);
    } else {
      setRun(id, pair === undefined ? undefined : `${id}/${pair}`);
    }
    if (pair !== undefined && !full) setPanel(true);
  });
  const setTableFilters = f => setFilters(f.scope === "all" && !f.group && !f.search && !f.where.length && !f.sort ? NO_FILTERS : f);
  // selectRow moves the table selection through the visible rows. When the selected row
  // has left the filter, the row now in its place is next and the one before it previous.
  const selected = rows.findIndex(r => r.id === currentId);
  if (selected >= 0) selIndex.current = selected;
  const selectRow = by => {
    const at = selected >= 0 ? selected + by : by > 0 ? selIndex.current : selIndex.current - 1;
    const next = rows[Math.max(0, Math.min(rows.length - 1, at))];
    if (next) show(next.id);
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
  const binds = !suite ? [] : view === "runs" ? runsBindings(runs.length)
    : view === "table" ? (panelOpen && item ? panelBindings(suite, item, onScreen, targets.length, runs.length) : tableBindings(onScreen, runs.length))
    : item ? bindings(suite, item, onScreen, targets.length, runs.length) : [];

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
    if (action.qid && !q) return; // the question does not apply to this item
    switch (action.type) {
      case "move": return go(index + action.by);
      case "select": return selectRow(action.by);
      case "selectRun": return selectRun(action.by);
      case "openRun": return openRun(shownRun.id);
      case "run": return stepRun(action.by);
      case "open": return item && openRow(item.id);
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
      case "option":
        return q.kind === "multi" ? toggleTag(q, row, action.value) : set(answer === action.value ? undefined : action.value);
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

  // toggleTag adds or removes one value of a multi question's answer.
  function toggleTag(q, row, tag) {
    const list = valueOf(state, item, q, row) || [];
    const proposed = row === undefined ? item.proposed?.[q.id]?.value : item.proposed?.[q.id]?.value?.[row];
    const next = list.includes(tag) ? list.filter(v => v !== tag) : [...list, tag];
    act.answer(item, q.id, next.length ? next : proposed !== undefined ? null : undefined, row);
  }

  // pickNumber completes a chord: a choice's key then n picks option n; a scale's key then n
  // sets n; a multi's key then n toggles option n and stays armed for the next tag.
  function pickNumber(n) {
    const { qid, row } = armed.current;
    armed.current = null;
    const q = questions.find(x => x.id === qid);
    if (q.kind === "multi") {
      if (q.options[n - 1]) toggleTag(q, row, q.options[n - 1].value);
      armed.current = { qid, row, until: Date.now() + 1500 };
      return;
    }
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
    // Esc unwinds one layer at a time: sheet, field, row cursor, the review panel, then review back to the table.
    if (e.key === "Escape") {
      if (sheet) return setSheet(null);
      if (typing) return e.target.blur();
      if (cursor) return setCursor(null);
      if (panelOpen) return setPanel(false);
      if (view === "review") switchView("table");
      else if (view === "table" && runs.length) switchView("runs");
      return;
    }
    if (sheet) {
      // Enter confirms, except on a focused Cancel button, which cancels.
      if (e.key === "Enter" && !e.repeat) {
        e.preventDefault();
        if (e.target.closest?.("[data-role=cancel]") || sheet !== "send") setSheet(null);
        else confirmSend();
      }
      return;
    }
    // From a filter field, ↓ or ↵ moves into the results.
    const field = typing && (e.key === "ArrowDown" || e.key === "Enter") ? e.target.dataset?.focus : null;
    if (field === "table-search" || field === "runs-search") {
      e.preventDefault();
      e.target.blur();
      if (field === "runs-search" && runRows[0]) setRun(runRows[0].run.id);
      else if (field === "table-search" && rows[0]) show(rows[0].id);
      return;
    }
    if (typing || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key.startsWith("Arrow") && e.target.matches?.("input[type=range]")) return; // the slider's own arrows
    if (!item && view === "review") return;
    // Tab and Shift+Tab step like J and K instead of moving focus.
    if (e.key === "Tab") {
      const step = match(binds, { key: e.shiftKey ? "k" : "j" });
      if (step) {
        e.preventDefault();
        return run(step.action);
      }
    }
    // A focused control handles its own Enter and Space; one last clicked with the mouse does not.
    if ((e.key === "Enter" || e.key === " ") && e.target.closest?.("button, a, summary")?.matches(":focus-visible")) return;
    if (/^[1-9]$/.test(e.key) && armed.current && Date.now() < armed.current.until) {
      e.preventDefault();
      return pickNumber(Number(e.key));
    }
    armed.current = null;
    const b = match(binds, e);
    if (b && (item || ["select", "selectRun", "openRun", "focus", "help", "run"].includes(b.action.type))) {
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
  const acrossView = across && { rows: across, metric, current: shownRun.id, open: setRun };
  const sendLabel = suite.send ?? "Send";
  const actionQs = [...new Map(orderedItems(suite).flatMap(i => questionsFor(suite, i)).filter(q => q.kind === "action").map(q => [q.id, q])).values()];
  const banners = html`
    ${stale ? html`<${Banner} tone="accent" title="The suite changed." action=${html`<button class="btn-secondary" onClick=${() => refresh()}>Refresh</button>`} />` : null}
    ${errors.length ? html`<${Banner} tone="negative" title=${`${errors.length} problem${errors.length > 1 ? "s" : ""} in the suite. Items with errors are hidden.`}
        lines=${errors.map(p => `${p.item ? `${p.item}: ` : ""}${p.path ? `${p.path}: ` : ""}${p.message} (${p.file ?? ""}${p.line ? ` line ${p.line}` : ""})`)} />` : null}
    ${loadErrors.length ? html`<${Banner} tone="negative" title="Custom renderers failed to load." lines=${loadErrors} />` : null}`;

  // The toolbar's ‹ › and counter walk review's items, or the table's rows as J and K do.
  const nav = view === "review" ? { index, count: list.length, step: by => go(index + by) } : { index: selected, count: rows.length, step: selectRow };

  return html`<div class=${cx(view === "review" ? "app" : "app table-mode", navOpen && "nav-open")}>
    <${Toolbar} view=${view} setView=${switchView} nav=${nav} saveLabel=${saveLabel} sendLabel=${sendLabel} openSend=${() => setSheet("send")}
      run=${shownRun && { run: shownRun, vs, count: runs.length, step: stepRun }} openNav=${view === "review" ? () => setNavOpen(!navOpen) : null} />
    ${view === "review" ? html`<${Sidebar} suite=${ordered} items=${list} state=${state} current=${index} go=${i => { go(i); setNavOpen(false); }} openHelp=${() => setSheet("help")}
      filtered=${!!walk} clearFilters=${() => setWalk(null)} total=${items.length} />` : null}
    ${navOpen ? html`<div class="nav-scrim" onClick=${() => setNavOpen(false)}></div>` : null}
    <main class="main">
      ${view === "runs"
        ? html`<${RunsView} suite=${suite} state=${state} runs=${runs} rows=${summary} shown=${runRows} metric=${metric} spend=${spend}
            filters=${runFilters} setFilters=${setRunFilters} selected=${shownRun.id} vs=${vs} select=${id => setRun(id)} open=${openRun} banners=${banners} />`
        : view === "table"
        ? html`<div class=${panelOpen && openIn === "sidebar" && !narrow ? "tv-layout with-sidebar" : "tv-layout"}>
            <${ItemsTable} suite=${ordered} state=${state} cols=${cols} rows=${rows} view=${filters} setView=${setTableFilters}
              selected=${currentId} select=${show} open=${openRow} openIn=${openIn} setOpenIn=${setOpenIn} counts=${scopeCounts(items, state)} progress=${progress(ordered, state)} banners=${banners}
              run=${shownRun && { run: shownRun, vs, paired }} />
            ${panelOpen ? html`<${ReviewPanel} mode=${narrow ? "tray" : openIn} setMode=${setOpenIn} item=${item} close=${() => setPanel(false)}
                position=${{ index: rows.findIndex(r => r.id === currentId), count: rows.length }} go=${selectRow}
                suite=${ordered} cols=${cols} state=${state} act=${act} cursor=${cursor} across=${acrossView} />` : null}
          </div>`
        : html`<${SuiteHead} title=${title} description=${suite.description} progress=${progress(ordered, state)} />
          <div class="scroll" ref=${scroller}><div class="content">
            ${banners}
            ${item ? html`<${ItemView} key=${item.id} suite=${ordered} cols=${cols} item=${item} state=${state} act=${act} cursor=${cursor} across=${acrossView} />` : html`<p class="muted">This suite has no items.</p>`}
          </div></div>`}
    </main>
    ${sheet === "send" ? html`<${SendSheet} sendLabel=${sendLabel} progress=${progress(suite, state)} actions=${actionQs}
        pending=${pendingActions(suite, state)} close=${() => setSheet(null)} confirm=${confirmSend} />` : null}
    ${sheet === "help" ? html`<${HelpSheet} binds=${binds} sendLabel=${sendLabel} description=${suite.description} view=${view} runs=${runs.length} close=${() => setSheet(null)} />` : null}
    <${Toast} ...${toast} />
  </div>`;
}
