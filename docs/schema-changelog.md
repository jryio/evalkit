# Schema changelog

How each demo reshaped the suite schema, and why. Newest last.

## v0: demo 1, status updates (port of the original review page)

Extracted from the original page's hardcoded layout:

- `title`, `send`, `reviewed`, `groups`, `questions`, `items`: suite-level shape.
- Item: `id`, `group`, `title`, `short`, `subtitle`, `badges`, `meta`, `show`, `ask`, `context`, `proposed`, `done`, `revision`.
- Question kinds `choice`, `multi`, `toggle`, `action`, `text`. `primary` binds 1–9. `status` drives the sidebar dot. `with` puts a question in another's row (the original page's flag chips beside the verdict). `each` asks a question per row (the original page's "Should be in").
- Artifact kinds `text` (with `editable`, `limit`), `list`, `table`. `width: half` pairs two artifacts (the original page's draft beside "You wrote").
- Tones, not colors: `positive | warning | negative | neutral | accent`.
- `revision` defaults to a hash of `show` only, so editing context or metadata keeps the human's answers.

Concepts specific to the original page did not survive as schema keys; each maps to a general kind: a status question, a list with per-row toggles, a table, or an action's `done`.

## v1: kitchen sink

- **Action labels grouped by state.** Eight flat keys (`button`, `on`, `note`, `note_on`, `badge`, `done`, `done_note`, `summary`) became `states: {off, on, done}`, each with `button`, `note` and `badge`. It reads like the three states the button really has.
- **New artifact kinds** `code`, `diff`, `tests`, `metrics`, `chart`, `markdown`, `json`, `transcript`, `image`. Each kind is one renderer module, registered in one line.
- **New question kinds** `scale`, `number`, `pick`. `pick` (`from: <artifact>`) picks one row, e.g. the first wrong step.
- **One row vocabulary.** Every artifact with selectable rows (`list`, `table`, `transcript`) uses `rows` with a row `id`, so `each` and `from` work the same everywhere.
- **Keyboard: row cursor and chords.** ↑/↓ move a cursor over the rows that per-row questions target. Per-row keys apply to the cursor row. A question's key followed by a number picks option N (`c` `4` sets Confidence to 4). Keys on per-row questions became legal.
- A standalone `toggle` renders as a switch. It renders as a pill only beside another question or on a row, so its label never repeats.

## v2: demos 2 and 3 (code review, perf canary)

- **`at` on list rows** (demo 2): a finding links to `{artifact, line}` and clicking it scrolls to the highlighted diff line. Renderers mark lines with `data-line`, and the kit wraps every artifact in `data-artifact`.
- **Per-row proposals** (demo 2): `proposed.<per-row question>.value` is a `{rowId: value}` map, so the reviewer agent's severity shows as a default per finding. The snapshot's `changed` reports overrides per row.
- **Row controls carry labels** (demo 2): two segmented controls on a row (Finding, Severity) are ambiguous without them.
- Demo 3 needed no schema change: `metrics`, `chart`, a proposed primary answer, `scale` and `action` covered it. That confirms the v1 kinds are the right size.

## v3: demo 5 (extraction)

- **`when`** shows a question only while another question's answer is one of the given values, evaluated per row for per-row questions (the "Correct value" field appears only for Wrong or Missing). Answers whose condition stops holding are left out of the snapshot. Prior art: Label Studio `visibleWhen`.
- **`tally`** counts a per-row question's answers live (`count`, optional `of` for a percentage), which gives precision and recall without the agent computing them.
- **Table rows take per-row controls** as extra columns. Choices whose options all have icons render icons-only there.

## v4: demos 4 and 6 (pairwise preference, agent trajectories)

No schema change. That is the signal the schema has converged:

- Demo 4's per-criterion rubric (correctness, helpfulness, tone; 1–5 for A and for B) is a `list` of criteria with two per-row `scale` questions. No `pairwise` or matrix kind was needed. A/B/tie/both-bad is a `choice`, and the LLM judge's pick is a `proposed` value, which is the calibration signal.
- Demo 6's "first wrong step" is `pick` over a `transcript`, the failure taxonomy is `multi` with letter keys, and the auto-grader verdict is `proposed`.
- Renderer-only change: long tool calls and results in a transcript clamp to 12 lines behind "Show all", following the progressive-disclosure rule.

## Rejected after the prior-art review (docs/research/prior-art.md)

- Per-option `score` (Braintrust, Inspect): agents map option values to numbers themselves; no demo needed it.
- A `pairwise` kind with side randomization (Chatbot Arena, Prodigy): demo 4 works with `choice`; randomizing sides is the suite author's job when it matters.
- An answer envelope `{value, comment, correction}` (LangSmith): `edits` cover corrections of editable artifacts, `when` + `text` cover per-answer comments.
- Undo of a send (HIG): sends are snapshots; the next send supersedes the last, and `evalkit summarize` shows the diff.

## v5: demo 7 and the simplification pass

- **Demo 7 (price recommendations)** gave `number` its first real use: a per-row `number` whose proposal is the model's price, revealed by `when: {approve: [true]}` on a per-row `toggle`. No schema change.
- **Deleted, because no demo used them:** an explicit item `revision` (it is always the hash of `show`), `limit.unit` (word counts), and the icons `up`, `down`, `flag`, `star`.
- **Kept and now rendered:** suite `description`. Every demo had one and it was invisible; it now sits under the sidebar title (two lines) and at the top of the help sheet, as instructions for the reviewer.
- **Unconfirmed proposals look tentative.** A per-row choice or number that only holds the agent's proposal renders outlined or dashed until the human confirms it, so reviewers can see what they still have to check.
- **Shared helpers** moved to `ui/bits.js` (`withUnit`), and tiles share one style.

## v6: hardening after the final review

- **Validation catches what used to break the UI:** `with` pointing at a text or per-row question; `each`/`from` on rows without ids; proposals for actions (the human approves those); multi proposals that are not option values; scale and number proposals out of range; per-row proposals for rows that do not exist.
- **An empty answer can override a proposal.** Clearing a proposed text, number or multi stores `null`, and turning off a proposed toggle stores `false`, so the proposal does not come back. `null` in a snapshot means "explicitly empty".
- **Actions hidden by `when` cannot be approved or sent.**
- Protocol, not schema: state saves carry a revision (a stale tab gets 409 and reloads), the server only accepts same-origin JSON on loopback, action results are one file each under `feedback/done/`, and `evalkit wait` lists an approved action once, then under "Still approved" until it is recorded done.

## v7: demo 8 (an LLM judge's round) and the table view

Evals graded by a judge loop have dozens to hundreds of rows, most of which need no human. Reviewing them one by one does not scale, and the loop writes rows as it goes.

- **A table view for every suite.** Columns come from what the suite already declares: each suite-level question, a group column and a note column. No per-view configuration. Review and table share filters and sort, so J and K walk the rows the table shows.
- **Judge scores are proposals.** No `score` kind: a verdict is a `choice`, a 0–10 or 0–1 score is a `number` with `min` and `max`. Overrides already come back as `changed`, which is the calibration signal (as demo 4 showed for pairwise).
- **`columns` and item `values`** hold what nobody should edit: cost, latency, word counts, deterministic checks. A value may carry a `baseline`, and so may a proposal; the table adds a Δ column toned by the column's `better` and `noise`.
- **`flag`** marks the items the loop wants a human on. When any item is flagged, progress counts only those.
- **`items_from`** reads items from JSONL files, the format harnesses already append to. A later line with the same id replaces the earlier one; a partial last line is skipped; a missing file is a warning. Problems point at the JSONL file and line.
- `scale` gained a judge reason beside it (`why`), like `choice` and `number`; a bounded `number` shows a slider.

## v8: runs

An eval loop runs the same cases again and again: a new model, a new prompt, a new skill version. Comparing two rounds meant two suites or a `baseline` on every value.

- **`runs`** lists the runs of one suite, in order. Each run has an `id` and optionally a `label`, a `note`, `baseline: true`, its own `items_from`, `dims` and run-level `values`. Each run is compared with the run marked `baseline: true`, else with the run before it.
- **`dimensions`** names what varies between runs (model, effort, date), in legend order. `order` makes a dimension ordinal.
- **Items gain `run` and `rep`.** A run's `items_from` sets `run` on each line, so harness output needs no change. `rep` numbers repetitions of the same case within a run.
- **Composite ids.** The loader rewrites the id of an item in a run to `<run>/<id>` (plus `.r<rep>`) and adds `pair`, the key that pairs items across runs. State, feedback and `evalkit wait` keep unique ids without a change.
- Identity is (run, id, rep): a duplicate is an error, and a later JSONL line replaces the earlier one in place. Runs that items name but `runs` does not declare are added, with a warning. With runs, inline `baseline` keys are ignored, with one warning. `view: runs` opens the runs view.
- **`primary: true` on a column** makes it the score runs are ranked by, when that is a measured value rather than the primary question.
- **The Runs view** (`view: runs`): tiles for the selected run, a table of every run, run-by-run and cost charts, and each case before and after. The table and review show one run at a time.
- In a suite with runs, an item without `run` is an error.
- A suite without runs loads exactly as before.
