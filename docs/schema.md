# Suite schema

A suite is one YAML (or JSON) file. It lists **items** for a human to review. Each item **shows** artifacts and **asks** questions. The kit owns the layout, so every suite looks and behaves the same. `evalkit validate` checks a suite; `evalkit schema` prints the JSON Schema for editors.

## Core

```yaml
title: Code review findings            # sidebar title
description: Triage each finding …     # what the reviewer does; under the title and in the help sheet
send: Send to Claude                   # submit button label
reviewed: [review]                     # an item is reviewed once one of these is answered (default: the primary question)
groups: [{ id: checkout, label: acme/checkout }]   # sidebar sections, in order
questions: [ … ]                       # asked of every item (narrow with a question's `groups`)
renderers: { flamegraph: renderers/flame.js }     # optional custom artifact kinds
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
    done: { post: { at: …, url: … } }  # results of actions (usually recorded with `evalkit done`)
```

**Page layout**, top to bottom: header (badges, meta, title) → `show` → a card with one row per compact question (choice, multi, toggle, scale, number, pick, action) → suite-level text questions → item-level text questions under "Questions" → `context` as disclosures.

**Revision.** Each item gets a hash of its `show`. When the agent changes what is under review, the item starts a fresh review and the old answers move to history. Editing `context` or metadata keeps the answers.

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

`J`/`K` (or `→`/`←`) move between items. `1`–`9` answer the primary question. `↑`/`↓` move the row cursor when the item has per-row questions. Letter keys come from the suite; pressing a selected option's key again clears it. `E` edits, `?` shows help (generated from the suite), `Esc` leaves a field or closes a sheet, `⌘↵` sends from anywhere. Letter keys are off while typing. The validator rejects conflicting keys.

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

Only items with input appear. `answers` holds what the human set (a missing answer means the proposal stands); `changed` lists overridden proposals; `edits` holds rewritten editable artifacts; `actions` lists approved actions that are not done yet. `evalkit wait` and `evalkit summarize` print the same thing as a diff against the previous send.

## Custom kinds

```yaml
renderers: { flamegraph: renderers/flame.js }
```

The module's default export is a Preact component. It receives `{ a, item, inContext, rowControls, cursor, answers, questions }` and imports `html` from `/kit/html.js`. Use the CSS variables in `/styles/tokens.css` so it follows light and dark mode. Row renderers call `rowControls(row)`, which returns `{ lead, below, cells }`, and add `data-row` and the `row-cursor` class.
