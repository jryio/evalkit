# Suite schema

A suite is one YAML (or JSON) file. It lists **items** for a human to review. Each item **shows** artifacts and **asks** questions. The kit owns the layout, so every suite looks and behaves the same. `evalkit validate` checks a suite; `evalkit schema` prints the JSON Schema for editors.

## Core

```yaml
title: Code review findings            # sidebar title
description: Triage each finding …     # what the reviewer does; under the title and in the help sheet
send: Send to Claude                   # submit button label
reviewed: [review]                     # an item is reviewed once one of these is answered (default: the primary question)
view: table                            # the view the page opens in: review (default), table or runs
groups: [{ id: checkout, label: acme/checkout }]   # sidebar sections, in order
questions: [ … ]                       # asked of every item (narrow with a question's `groups`)
renderers: { flamegraph: renderers/flame.js }     # optional custom artifact kinds
columns: [{ id: cost_usd, label: Cost, unit: $, better: lower }]   # read-only value columns for the table
items_from: runs/round-3.jsonl         # JSONL, one item per line, appended after items (see Automated judges)
items:
  - id: pr-318                         # stable; answers are keyed by it
    group: checkout
    title: Typed params for four carriers   # page heading
    short: "#318 Typed params"         # sidebar title (default: title)
    subtitle: 4 findings · dana        # sidebar second line
    badges: [{ label: "#318", tone: accent }]
    meta: dana · +212 −88 · opened Nov 3    # secondary text beside the badges
    show: [ …artifacts ]               # what is under review; always open
    ask: [ …questions ]                # questions for this item only
    context: [ …artifacts ]            # supporting material; collapsed
    proposed: { review: { value: changes, why: "2 valid findings" } }   # the agent's answers; why is one sentence
    values: { cost_usd: { value: 0.21, baseline: 0.24 } }   # measured values for the columns
    flag: Judge confidence 0.52           # the agent wants a human on this item
    done: { post: { at: …, url: … } }  # results of actions (usually recorded with `evalkit done`)
```

**Page layout**, top to bottom: header (badges, meta, title) → a card with one row per compact question (choice, multi, toggle, scale, number, pick, action) → `show` → suite-level text questions → item-level text questions under "Questions" → `below` artifacts → `context` as disclosures. The card comes first so the verdict sits in the same place on every item. Under a proposed answer the card shows what was proposed and why, and that line stays the same when the reviewer picks something else.

**Revision.** Each item gets a hash of its `show`. When the agent changes what is under review, the item starts a fresh review and the old answers move to history. Editing `context` or metadata keeps the answers.

**Flags.** When any item has a `flag`, progress counts flagged items only ("3 of 7 flagged reviewed"). The rest need no human; the reviewer can still open and answer them.

**Tones**, never colors: `positive`, `warning`, `negative`, `neutral`, `accent`.

### Questions

Every question has `id` and `kind`, and optionally:

| Key | Meaning |
|---|---|
| `label` | Row label, section title, or (item questions) the question text |
| `key` | One letter. Choice: cycles; then a digit picks option N. Scale: then a digit sets the value. Multi: then each digit toggles option N (options 1–9). Toggle/action: flips. Text/number: focuses. Pick: picks the cursor row. |
| `groups` | Ask only items in these groups |
| `with` | Render inside another question's row (e.g. flag chips beside the verdict) |
| `each` | Ask once per row of the artifact with this id; the answer is `{rowId: value}` |
| `when` | `{question: [values]}`: ask only while that answer holds (per row for per-row questions) |

| Kind | Answer | Extra keys |
|---|---|---|
| `choice` | option value | `options`, `primary` (number keys 1–9), `status` (colors the sidebar dot) |
| `multi` | list of values | `options` (each may have its own `key`) |
| `toggle` | `true` | — (a switch on its own row, a pill per row or beside another question) |
| `scale` | number | `min` (1), `max` (5), `labels: [low, high]` |
| `number` | number | `min`, `max`, `step`, `unit`, `placeholder` |
| `text` | string | `placeholder` (per row: a one-line input) |
| `pick` | row id | `from`: the artifact whose row is picked |
| `action` | `true` = approved | `summary`, `states: {off, on, done}` each `{button, note, badge}` |

An option is `{value, label, tone?, icon?, key?}`; icons: `check`, `approx`, `cross`, `question`.

### Artifacts

Every artifact has `kind`, and optionally `id` (needed for `each`, `pick`, `at` and edits), `label`, `note` (secondary text), `status: {label, tone}` (a dot and label), `width: half` (two halves sit side by side; they stack below 1240 px) and `below: true` (shown after the text questions, for long material such as code that the reviewer checks after a first judgment).

| Kind | Keys |
|---|---|
| `text` | `text`, `editable`, `limit: {target: [lo, hi], max}`, `key` (edit key, default `e`) |
| `markdown` | `text` (raw HTML is shown as text) |
| `code` | `code`, `language`, `start`, `highlight: [lines]`, `max_lines` (default 250; longer code scrolls inside its panel) |
| `compare` | `panes: [{label, note, code, language, start, highlight, tone}]` (2 or 3, left to right), `language` (for every pane), `wrap`, `max_lines`, `diff` (two panes open as a split diff of left against right; `false` opens them as plain panes; a Panes / Diff switch toggles it). Spans the full page width; pane headers pin under the toolbar while the panes scroll by |
| `diff` | `diff` (unified), `language`, `highlight: [new-side lines]`, `max_lines`, `split` (start side by side). A Unified / Split switch draws Split with @pierre/diffs. Spans the full page width |
| `list` | `rows: [{id, title, detail, badge, links: [{label, url}], at: {artifact, line, label}, ask: false}]` |
| `table` | `columns: [{key, label, align, muted}]`, `rows` (a cell is a value or `{value, tone}`; rows with `id` take per-row questions) |
| `transcript` | `rows: [{id, role, text, name, input, output, error}]`; roles `system`, `user`, `assistant`, `tool_call`, `tool_result` |
| `tests` | `tests`: a tree of `{name, status: pass/fail/skip, duration, output, children}` |
| `metrics` | `metrics: [{label, value, baseline, unit, better: higher/lower, decimals, tolerance}]`, `layout: tiles/table`, `columns: {baseline, value}` |
| `chart` | `type: line/bar`, `x` (numbers, ISO times or labels), `series: [{label, values, tone}]`, `unit`, `y_min`, `marks: [{x, label}]` |
| `json` | `data`, `open` (levels expanded) |
| `image` | `src` (relative to the suite, or a URL), `alt`, `caption` |
| `tally` | `question` (a per-row question), `stats: [{label, count: [values], of: [values] or "rows"}]` |

Put code that is read side by side (before and after, a deleted test beside the test that keeps its proof) in one `compare`, not in two `width: half` code artifacts: `compare` leaves the reading column and gives each pane half the page. A pane's `tone` colors its dot and its `highlight` lines. Wrap folds long lines.

A proposal's `why` is one sentence; the page shows two lines of a longer one until the reviewer opens it. Evidence belongs in `show` or `context`, where it has room.

Rows of `list`, `table` and `transcript` share one vocabulary: a stable `id` makes a row answerable by `each` and `pick`.

### Keyboard

`J`/`K` (or `→`/`←`) move between items. `Esc` goes back to the table (and from the table to Runs, in a suite with runs); in the table `↑`/`↓` (or `J`/`K`) select a row, `↵` opens it in the sidebar, floating panel or full screen, `1`–`9` answer its primary question and `/` filters. `1`–`9` answer the primary question. `↑`/`↓` move the row cursor when the item has per-row questions. Letter keys come from the suite; pressing a selected option's key again clears it. `[`/`]` step to the previous or next run in a suite with runs. `E` edits, `?` shows help (generated from the suite), `Esc` leaves a field or closes a sheet, `⌘↵` sends from anywhere. Letter keys are off while typing. The validator rejects conflicting keys.

## Table view

Every suite has two views. **Review** shows one item at a time. **Table** shows every item as a row. Switch with the toolbar or open the page at `?view=table`; `view: table` makes the table the default.

Enter (or a double-click) opens the selected row where the toolbar's **Open rows in** control says, and the choice is remembered:

- **Sidebar**: the review docks beside the table, which narrows.
- **Floating**: the review floats over the table body, below the column headers.
- **Full screen**: the Review view. Esc goes back to the table.

While a panel is open it follows the selection: J and K (and ↑/↓, unless the item has rows to answer) move through the rows, and the item's own keys answer it. Esc closes the panel.

The table's columns come from the suite. Nothing is configured per view:

| Column | From | Cell |
|---|---|---|
| Group | `groups` | the group label |
| One per suite-level question | `choice`, `multi`, `toggle`, `scale`, `number`, `action` (not per-row, not `pick`) | the answer, else the proposal. An overridden proposal is outlined; hover shows the old value. Choices whose options all have icons show only the icon |
| One per `columns` entry | `values` | numbers with `unit` and `decimals`; booleans as ✓/✕; anything else as text. `min` and `max` add a bar |
| Δ after any of the above | `baseline` on a proposal or a value | the difference, toned by `better` once it exceeds `noise`; a categorical change reads "was Fail" |
| Note | the suite's text questions | the first line of the first answer |

Each column header has a profile of the rows in view: a histogram and the average for numbers, a share bar for choices, ▲/▼ counts for a Δ. Click a bar or a count to filter by it; click the label to sort. The bar above the table filters by flagged items, items with your input, and overridden proposals, by group, and by text. Opening review from a filtered or sorted table pins its rows: J and K walk them in the table's order, even after an answer moves an item out of the filter ("Show all" in the sidebar unpins).

A column definition:

| Key | Meaning |
|---|---|
| `id` | Key in each item's `values`; must not repeat a question id |
| `label`, `unit`, `decimals` | Header and number format. Currency units (`$`, `€`, `£`, `¥`) come first |
| `better` | `higher` or `lower`: which way a Δ is an improvement (booleans: whether true is good) |
| `noise` | A Δ no larger than this stays neutral |
| `primary` | With runs: the score the Runs view ranks runs by, instead of the primary question. At most one column |
| `min`, `max` | The range for the cell bar and the histogram |

A value is a number, string, boolean or `null`, or `{value, baseline}` to show the change from the baseline run.

## Automated judges

A judge loop (an eval harness, a hill-climbing round) feeds evalkit the same way an agent does, with two additions: rows arrive as JSONL, and most rows need no human.

```yaml
# suite.yaml: written once
view: table
questions:
  - { id: verdict, kind: choice, primary: true, options: [ … ] }   # the judge's verdict
  - { id: quality, kind: number, min: 0, max: 10, step: 1 }        # a judge score
  - { id: note, kind: text, label: Note to Claude }
columns:
  - { id: cost_usd, label: Cost, unit: $, better: lower }
items_from: runs/round-3.jsonl
```

```json
{"id": "refund.r0", "title": "Refund for an annual plan", "group": "test",
 "show": [ … ], "flag": "Judge confidence 0.52",
 "proposed": {"verdict": {"value": "fail", "why": "Invents a credit", "baseline": "pass"},
              "quality": {"value": 4, "baseline": 7}},
 "values": {"cost_usd": {"value": 0.012, "baseline": 0.019}}}
```

- **Judge answers are proposals.** Pass/fail and labels are `choice`, 1–9 ratings are `scale`, 0–10 or 0–1 scores are `number` with `min` and `max` (review shows a slider). The reviewer changes only what they disagree with; each change comes back under `changed` as `{from, to}`, which is the judge's calibration data.
- **Measurements are values.** Cost, latency, word counts and deterministic checks go in `values`; the reviewer cannot change them.
- **`items_from`** takes a path or glob relative to the suite, or a list of them. Each line is one item. A later line with the same id replaces the earlier one in place, so a loop can append a regraded row. A last line without a newline is skipped as still being written, and a missing file is a warning, so `evalkit serve` can start before the first row. A bad line hides only that row; problems name the JSONL file and line.
- **The page updates live** as lines are appended, like any suite edit.
- **Flag** the rows a human should see: failures, low judge confidence, judge errors.

## Runs

A run is one pass of an eval: one model, prompt or skill version over the same cases. With `runs`, one suite holds several runs and compares the same case across them.

```yaml
view: runs                 # review | table | runs
dimensions:                # optional: what varies between runs, in legend order
  - id: model
    label: Model           # optional; default: the id
  - id: effort
    order: [low, medium, high, xhigh]   # optional: the values' order (an ordinal dimension)
  - id: date               # values may be strings, numbers or YYYY-MM-DD dates
runs:
  - id: baseline           # an item id: letters, digits, '_', '.' and '-'; unique
    label: Baseline        # optional; shown instead of the id
    baseline: true         # optional; at most one run; without one, each run is compared with the run before
    note: no skill         # optional one-line note
    items_from: baseline.jsonl          # as the top-level items_from
    dims: {round: 0, date: "2026-10-14", model: opus-5.5}
    values: {cost_usd: 0.79}            # run-level column values, used when an item lacks the column
runs_from: runs/*/run.json  # JSON files, each one run as above; appended to runs in started_at order
```

`runs_from` lets a runner add a run without editing the suite: `evalkit run` writes one such file per run (see Benchmarks). A run may also carry `started_at` and `meta`, provenance the page does not interpret.

Items gain two optional keys:

| Key | Meaning |
|---|---|
| `run` | The run the item belongs to. The loader sets it on every line a run's `items_from` reads; such a line that names another run is an error. A line in the top-level `items` or `items_from` can set it itself, so one file can hold many runs |
| `rep` | The repetition of the same case within a run: an integer, default 0 |

**Ids.** The loader gives every item that has a `run` the id `<run>/<id>`, with `.r<rep>` appended when `rep` is above 0 (`v9/auth`, `v9/auth.r1`). The id stays unique in the state, the feedback and `evalkit wait` output. The item also gets `pair`: its own id, with the same `.r<rep>`. Items in two runs pair when their `pair` values are equal.

**Identity** is (run, id, rep). A duplicate is an error, like a duplicate item id. A later JSONL line with the same run, id and rep replaces the earlier one in place.

**Runs the suite does not declare.** A run that items name but `runs` does not list is added to `runs` as `{id}`, after the declared runs, in the order the items name them. When the suite has `runs`, each such run is a warning. A suite without `runs` gets its runs from the items and no warning.

**Baseline.** Each run is compared with the run marked `baseline: true`, else with the run before it. With runs, every item belongs to a run (an item without one is an error), and a `baseline` in an item's values or proposals is ignored, with one warning.

A run's `dims` should name `dimensions` (a warning otherwise, when `dimensions` is set), and its `values` must name `columns`. `view: runs` without runs is a warning, and the page shows the table.

**The score.** Runs are ranked by one score: the column marked `primary: true`, else the primary question, else the first column with `min` and `max` (or with `better`). A choice scores its options by tone: positive 1, warning 0.5, negative 0; other options don't count. A toggle scores 1 when on. Spend is the first `$` column, else a token column, else a seconds column, and never the score's own column.

**Statistics.** Every score has a 95% interval: Wilson for a rate, and a seeded bootstrap that resamples cases for a mean (and for a rate whose cases have reps). A paired change resamples cases too, and McNemar's exact test backs a pass/fail change. The page computes all of it, so an override updates every number at once. A Δ stays gray within the metric's `noise` or when its interval crosses zero.

**Table and review.** They show one run at a time: the latest, or `?run=<id>` (a link to an item opens its run). A suite with runs opens on the table unless `view: review` or `view: runs`. The toolbar names the run, its date or round, and the run it is compared with; `[` and `]` (or ‹ ›) step between runs and keep the same case selected. Δ columns compare each item with its pair in the compared run, using your answer where you gave one, else the proposal. An item's review lists its case in every run under **Across runs**; click a run to open the case there. Send still sends every run.

**The Runs view.** A third view, before Table and Review (`?view=runs`, or `view: runs` to open on it). Esc climbs from Review to the table and from the table to Runs. Top to bottom:

- **Tiles** for the selected run: the run, its score and interval with its change from the run before and from the compared run, the paired items that moved up, stayed the same or moved down, and its spend.
- **Runs**, a table of every run, newest first: varying dimensions, note, paired items, score, interval, both changes, the up, same and down counts, and spend.
- **Charts:** the score run by run (across an ordered dimension with one line per value of another, else by round, date or run order) and the score against spend, with its frontier.
- **Item by item:** the selected run against the compared run, as a scatter for a mean or a bar per run of improved, same and regressed for a rate.

The bar above filters runs by id, label or note, recomputes everything for one group, and switches between your answers and the judge's only. Click a run, a dot or a count to open the table on it; with Judge only on, counts are not links, since the table shows your answers. Keys: `↑`/`↓` or `J`/`K` select a run, `↵` opens it, `[`/`]` step runs, `/` filters and `?` shows help.

## Benchmarks

A suite with a `bench` block can be run, not only reviewed. `evalkit run` puts every case through an agent, each attempt in its own workspace, then through the stages that grade it, and writes the result as one run of rows.

```yaml
runs_from: runs/*/run.json     # each run registers itself here; no edit to this file
dimensions: [{ id: model }, { id: effort }]
bench:
  cases: cases.jsonl           # one case per line: {id, prompt, group, title, expect, pin, …}
  reps: 1                      # attempts per case
  concurrency: 4               # attempts at once, each in its own slot
  timeout: 600                 # seconds an agent or a stage may take
  budget_usd: 5                # stop starting attempts once the run has spent this
  retries: 0                   # extra tries after a timeout or a harness failure
  workspace:
    sources: [{ path: ../my-repo, pin: HEAD }]
  agent: { adapter: claude, model: claude-haiku-5-5, effort: low }
  score: [score.py]            # bin/ beside the suite is on PATH
```

### Commands

| Command | What it does |
|---|---|
| `evalkit check` | Makes one slot and times it, estimates the disk every slot needs, then runs `bench.oracle` (must score every case as a pass) and `bench.null` (must score every case as a fail). Run it before paying for a run. |
| `evalkit run --run ID` | Makes or resumes one run. `--baseline`, `--label`, `--note`, `--dim k=v` describe it; `--adapter`, `--model`, `--effort`, `--isolate`, `--sandbox` override the agent for a sweep; `--only`, `--limit`, `--reps`, `--concurrency` narrow it. |
| `evalkit grade --run ID --score --judge` | Grades a finished run's stored attempts again, without the agent, after a scorer or the rubric changed. |
| `evalkit climb --rounds N` | Hill-climbs `bench.climb.targets`. |
| `evalkit stats [--json] [--group g]` | Each run's score, 95% interval and paired change, the same numbers as the Runs view. |
| `evalkit clean [--run ID]` | Deletes the suite's workspace slots, so the next run provisions them again; with `--run`, deletes that run too. Slots a running evalkit holds are left. |

### What a run writes

```
runs/<run>/run.json        the run as runs_from reads it, with provenance under meta
runs/<run>/rows.jsonl      one row per finished attempt, appended as it finishes
runs/<run>/errors.jsonl    one line per failed try: id, rep, class, error, cost
runs/<run>/attempts/       each attempt's full payload, for grade and climb
```

An attempt that fails before it has an answer never becomes a row, so it never counts as a fail. Its class says why: `timeout` (the wall-clock limit), `agent_error` (the agent ran and reported an error) or `harness` (a stage or the workspace broke). Timeouts and harness failures are retried `retries` times; an agent error is not. Running the same `--run` again makes only the attempts with no row.

`meta` records the commit, the agent, model, effort and CLI version, the spend, and two hashes: `harness` changes when the cases, the stage scripts or the rubric change, and `variant` changes when the thing under test changes. Compare runs only when their `harness` is the same.

A run fills the dimensions the suite declares under a name it knows: `agent`, `model`, `effort`, `isolate`, `sandbox`, `reps`. A dimension named after a variable in `env` takes that variable's value: with `env: [IMPL]` and a dimension `impl`, `IMPL=fast evalkit run` is the run where `impl` is `fast`.

### Cases

A case is a JSON object with an `id`. `title`, `short`, `subtitle` and `group` go on the row. `prompt` is what the agent is asked, unless `bench.agent.prompt` is a template over the case (`"Rename {{.old}} to {{.new}}"`, Go text/template). `expect` holds the answers: stages after the agent see it, and the agent never does. `pin: {source: ref}` starts a git source at another commit for this case.

### Stages

A stage is a shell command. It reads the attempt as JSON on stdin and may print one JSON object, a fragment of the row.

| Stage | Runs | Prints |
|---|---|---|
| `prepare` | before the agent, in its directory | nothing |
| `agent` | the agent under test | its answer |
| `collect` | after the agent | `show` and `context`: what the reviewer sees |
| `score` (a list) | after collect | `values`, `proposed`, `flag`, more `show` |
| `judge` | last | a proposal per question |

The payload on stdin:

```json
{ "case": {…}, "run": "v2", "rep": 0, "suite_dir": "…", "workspace": "…/slots/3", "cwd": "…/slots/3/repo",
  "sources": {"repo": "…/slots/3/repo"}, "out": "…/attempts/auth.r0", "prompt": "…",
  "agent": {"output": "…", "transcript": […], "cost_usd": 0.01, "seconds": 12.3, "turns": 4, "tokens": {…}},
  "diff": {"repo": "diff --git …"}, "row": {…} }
```

The same values are in the environment as `EVALKIT_SUITE_DIR`, `EVALKIT_WORKSPACE`, `EVALKIT_CWD`, `EVALKIT_OUT`, `EVALKIT_RUN`, `EVALKIT_CASE` and `EVALKIT_REP`. A fragment may set `title`, `short`, `subtitle`, `group`, `badges`, `meta`, `show`, `context`, `values`, `proposed` and `flag`. `show` and `context` append; `values` and `proposed` merge by key.

With no `collect` and no `show` from a scorer, the row shows the agent's answer and each git source's diff. Every row gets the transcript and the prompt as context, and the columns the suite declares among `cost_usd`, `latency_s`, `turns`, `tokens_in`, `tokens_out` and `judge_cost_usd`.

### Agents

| `adapter` | Runs | Notes |
|---|---|---|
| `claude` | `claude -p` | reports cost, turns and loaded skills |
| `codex` | `codex exec` | reports tokens, not dollars |
| `pi` | `pi -p` | reports cost per message |
| `command` | any shell command | reads the payload on stdin; prints the answer, or `{output, cost_usd, transcript}` |

Each takes `model`, `effort`, `args` (extra CLI arguments), `tools: none`, `budget_usd` (claude) and `system` (a suite file added to the system prompt). `require_skills` makes an attempt where claude did not load a skill a harness error. evalkit holds no API key: each CLI uses its own login.

### Workspaces

Every attempt gets a slot of its own. Slots live in the user cache, outside the suite, so an agent that walks up from its directory never finds the cases.

| Key | Meaning |
|---|---|
| `sources` | Directories copied into every slot, each under `as` (default: its base name). A git repository becomes a clone that shares the source's objects, at `pin` (default HEAD), with no remote and no other refs. Anything else is copied, copy-on-write where the file system has it. `method: clone` or `copy` forces one. `include` brings untracked paths of a git source along. |
| `setup` | Shell command run once per slot after it is made, such as a dependency install |
| `keep` | Slot-relative paths a reset leaves alone, such as `repo/node_modules` |
| `reset` | `attempt` (default): before every attempt. `run`: once, when a run first uses the slot. `never` |
| `provision`, `wipe` | Shell commands that fill and reset a slot (`$EVALKIT_SLOT`) in place of the above |

A reset puts a git source back on its commit with a clean tree and re-syncs a copied one, so it costs in proportion to what the attempt changed. Only committed files of a git source reach a slot. Slots are reused by later runs while the workspace config is unchanged, and a file lock gives two runs at once different slots. With no `workspace`, an attempt gets an empty directory.

`overlay: [{from, to}]` copies suite files into the slot before each attempt: the thing under test, such as a skill or a config. The agent starts in `cwd`, else the only source, else the slot.

### Isolation

Each attempt gets its own `TMPDIR`, its own git config (no signing, hooks or credential helper of yours) and only the environment variables agents need: `PATH`, `HOME`, the locale and the agent CLIs' own. `env: [NAME]` passes more, and `home: private` gives an empty `HOME` (agent CLIs that log in through `HOME` cannot use it).

| `isolate` | claude | codex | pi |
|---|---|---|---|
| `agent` (default) | safe mode: no settings, skills, memory files or MCP servers | your config and rules ignored, an empty `HOME`, account features off | no extensions, skills, prompt templates, context files or MCP |
| `project` | the workspace's own skills and CLAUDE.md load; so does your `~/.claude/CLAUDE.md` | as `agent` | as `agent` |
| `none` | as you run it | as you run it | as you run it |

`sandbox: true` (macOS) runs the agent under an OS sandbox: it writes only in its slot and temp directory, and cannot read the suite, the runs, other slots, or your keys and personal skills. A stage is not sandboxed, and a scripted `command` agent must then be inline or outside the suite.

Without the sandbox, claude and pi can still read your home directory, and codex still loads skills stored in its own home. `benchmarks/02-config-leak` measures all of this on your machine.

### Judges

`judge: {adapter, model, rubric: rubric.md, questions: […]}` asks a model to grade each attempt. It sees the case with its `expect`, the answer, the diffs and the measured values, in an empty directory with no tools, and must answer every listed question (default: every choice, number, scale and toggle question) with a value and a one-sentence reason. Its answers are proposals, so a reviewer's overrides are the judge's calibration data. A judge that fails leaves the row with no verdict and a flag; `evalkit grade --judge` asks again.

### Hill climbing

```yaml
bench:
  climb:
    targets: [prompt/system.md]            # suite files a round may edit
    train: [train]                         # groups the proposer sees; a round is accepted on these
    test: [test]                           # held-out: scored every round, never shown to the proposer
    propose: { adapter: claude, model: claude-haiku-5-5 }
    rounds: 3
    target: 0.9                            # stop once the train score reaches this
```

`evalkit climb` runs the baseline, then each round: the proposer works in a scratch directory holding the targets, `failures.json` (the train cases the best version still gets wrong, with the grader's reasons), `passing.json` and `history.md`; the benchmark runs again; the change is kept when the train score rises and reverted when it does not. `climb.json` beside the suite records every round, so a climb continues where it stopped. Each run keeps its targets, the round's patch (`change.patch`) and what the proposer said and did (`proposer.json`) under `runs/<run>/`. A proposer that changes nothing ends the climb, and its closing sentence is printed.

## What comes back

Each send writes `feedback/sent-NN.json` next to the suite and wakes `evalkit wait`:

```json
{ "suite": "Code review findings", "sequence": 3, "sent_at": "2026-11-09T23:46:18Z",
  "progress": { "reviewed": 2, "total": 4 },
  "actions": [ { "item": "pr-325", "action": "post", "title": "Encode each carrier request once" } ],
  "items": { "pr-325": {
      "revision": "4be1f0c2a9d3", "reviewed": true,
      "answers": { "review": "changes", "triage": { "f1": "valid", "f2": "false_positive" } },
      "changed": { "severity": { "f2": { "from": "high", "to": "low" } } },
      "edits": {} } } }
```

Only items with input appear. With flagged items, `progress` counts those only and carries `"flagged": true`. `answers` holds what the human set (a missing answer means the proposal stands); `changed` lists overridden proposals; `edits` holds rewritten editable artifacts; `actions` lists approved actions that are not done yet. `evalkit wait` and `evalkit summarize` print the same thing as a diff against the previous send.

## Custom kinds

```yaml
renderers: { flamegraph: renderers/flame.js }
```

The module's default export is a Preact component. It receives `{ a, item, inContext, rowControls, cursor, answers, questions }` and imports `html` from `/kit/html.js`. Use the CSS variables in `/styles/tokens.css` so it follows light and dark mode. Row renderers call `rowControls(row)`, which returns `{ lead, below, cells }`, and add `data-row` and the `row-cursor` class.
