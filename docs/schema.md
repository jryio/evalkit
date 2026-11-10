# Suite schema

A suite is one YAML (or JSON) file. It lists **items** for a human to review. Each item **shows** artifacts and **asks** questions. The kit owns the layout, so every suite looks and behaves the same. `evalkit validate` checks a suite; `evalkit schema` prints the JSON Schema for editors.

## Core

```yaml
title: Code review findings            # sidebar title
description: Triage each finding …     # what the reviewer does; under the title and in the help sheet
send: Send to Claude                   # submit button label
reviewed: [review]                     # an item is reviewed once one of these is answered (default: the primary question)
view: table                            # the view the page opens in: review (default) or table
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
    proposed: { review: { value: changes, why: "2 valid findings" } }   # the agent's answers
    values: { cost_usd: { value: 0.21, baseline: 0.24 } }   # measured values for the columns
    flag: Judge confidence 0.52           # the agent wants a human on this item
    done: { post: { at: …, url: … } }  # results of actions (usually recorded with `evalkit done`)
```

**Page layout**, top to bottom: header (badges, meta, title) → `show` → a card with one row per compact question (choice, multi, toggle, scale, number, pick, action) → suite-level text questions → item-level text questions under "Questions" → `context` as disclosures.

**Revision.** Each item gets a hash of its `show`. When the agent changes what is under review, the item starts a fresh review and the old answers move to history. Editing `context` or metadata keeps the answers.

**Flags.** When any item has a `flag`, progress counts flagged items only ("3 of 7 flagged reviewed"). The rest need no human; the reviewer can still open and answer them.

**Tones**, never colors: `positive`, `warning`, `negative`, `neutral`, `accent`.

### Questions

Every question has `id` and `kind`, and optionally:

| Key | Meaning |
|---|---|
| `label` | Row label, section title, or (item questions) the question text |
| `key` | One letter. Choice: cycles; then a digit picks option N. Scale: then a digit sets the value. Toggle/action: flips. Text/number: focuses. Pick: picks the cursor row. |
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

Every artifact has `kind`, and optionally `id` (needed for `each`, `pick`, `at` and edits), `label`, `note` (secondary text), `status: {label, tone}` (a dot and label) and `width: half` (two halves sit side by side; they stack below 1240 px).

| Kind | Keys |
|---|---|
| `text` | `text`, `editable`, `limit: {target: [lo, hi], max}`, `key` (edit key, default `e`) |
| `markdown` | `text` (raw HTML is shown as text) |
| `code` | `code`, `language`, `start`, `highlight: [lines]` |
| `diff` | `diff` (unified), `language`, `highlight: [new-side lines]` |
| `list` | `rows: [{id, title, detail, badge, links: [{label, url}], at: {artifact, line, label}, ask: false}]` |
| `table` | `columns: [{key, label, align, muted}]`, `rows` (a cell is a value or `{value, tone}`; rows with `id` take per-row questions) |
| `transcript` | `rows: [{id, role, text, name, input, output, error}]`; roles `system`, `user`, `assistant`, `tool_call`, `tool_result` |
| `tests` | `tests`: a tree of `{name, status: pass/fail/skip, duration, output, children}` |
| `metrics` | `metrics: [{label, value, baseline, unit, better: higher/lower, decimals, tolerance}]`, `layout: tiles/table`, `columns: {baseline, value}` |
| `chart` | `type: line/bar`, `x` (numbers, ISO times or labels), `series: [{label, values, tone}]`, `unit`, `y_min`, `marks: [{x, label}]` |
| `json` | `data`, `open` (levels expanded) |
| `image` | `src` (relative to the suite, or a URL), `alt`, `caption` |
| `tally` | `question` (a per-row question), `stats: [{label, count: [values], of: [values] or "rows"}]` |

Rows of `list`, `table` and `transcript` share one vocabulary: a stable `id` makes a row answerable by `each` and `pick`.

### Keyboard

`J`/`K` (or `→`/`←`) move between items. `Esc` goes back to the table; in the table `↑`/`↓` (or `J`/`K`) select a row, `↵` opens it, `1`–`9` answer its primary question and `/` filters. `1`–`9` answer the primary question. `↑`/`↓` move the row cursor when the item has per-row questions. Letter keys come from the suite; pressing a selected option's key again clears it. `E` edits, `?` shows help (generated from the suite), `Esc` leaves a field or closes a sheet, `⌘↵` sends from anywhere. Letter keys are off while typing. The validator rejects conflicting keys.

## Table view

Every suite has two views. **Review** shows one item at a time. **Table** shows every item as a row. Switch with the toolbar, with Enter (open the selected row) and Esc (back to the table), or open the page at `?view=table`. `view: table` makes the table the default.

The table's columns come from the suite. Nothing is configured per view:

| Column | From | Cell |
|---|---|---|
| Group | `groups` | the group label |
| One per suite-level question | `choice`, `multi`, `toggle`, `scale`, `number`, `action` (not per-row, not `pick`) | the answer, else the proposal. An overridden proposal is outlined; hover shows the old value. Choices whose options all have icons show only the icon |
| One per `columns` entry | `values` | numbers with `unit` and `decimals`; booleans as ✓/✕; anything else as text. `min` and `max` add a bar |
| Δ after any of the above | `baseline` on a proposal or a value | the difference, toned by `better` once it exceeds `noise`; a categorical change reads "was Fail" |
| Note | the suite's text questions | the first line of the first answer |

Each column header has a profile of the rows in view: a histogram and the average for numbers, a share bar for choices, ▲/▼ counts for a Δ. Click a bar or a count to filter by it; click the label to sort. The bar above the table filters by flagged items, items with your input, and overridden proposals, by group, and by text. Review follows the table's filters and sort, so J and K walk the same rows.

A column definition:

| Key | Meaning |
|---|---|
| `id` | Key in each item's `values`; must not repeat a question id |
| `label`, `unit`, `decimals` | Header and number format. Currency units (`$`, `€`, `£`, `¥`) come first |
| `better` | `higher` or `lower`: which way a Δ is an improvement (booleans: whether true is good) |
| `noise` | A Δ no larger than this stays neutral |
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
