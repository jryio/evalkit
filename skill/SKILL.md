---
name: evalkit
description: Collect structured human feedback on agent output through a keyboard-first review page, and get the answers back in the conversation. Use when a human should review, grade, approve or correct a batch of outputs — drafts, code review findings, perf results, model responses, extracted fields, agent trajectories — or approve an outward-facing action before you take it.
---

# evalkit

You write a suite (YAML), serve it, and wait in the background. The human reviews in the browser and presses Send (⌘↵) whenever they like; each send wakes you with only what changed. The server keeps running, so they can keep reviewing and send again.

If `evalkit` is not on PATH: `go install github.com/jryio/evalkit/cmd/evalkit@latest`.

## Write for someone who has not been following along

This rule comes before every other one, every time. The reviewer has not seen your work. Every text they read (the suite `title` and `description`, each item `title`, every `label`, `note` and option, and each proposal's `why`) must:

- Say in plain, natural sentences what this is and why it matters, with the context that reader is missing.
- Stay within its budget: a title fits on one line, a `why` is one sentence, a `description` is one line telling the reviewer what to do.
- Make sense spoken out loud to a high-school student: explain every term the reader needs, and use no metaphors and no analogies.
- Be plain prose: no headings, lists, bold or other formatting inside it.

Not this: `R: pins exported AsRange int inputs; public API rule forces R despite the dup flag and 3/3 co-killers.`
This: `Another test already checks that whole numbers work as range limits, so deleting this one loses nothing.`

## The loop

1. **Write the suite.** `evalkit init <dir>` writes a starter `<dir>/suite.yaml`; edit it. `evalkit docs` is the full reference; `evalkit init --example <tmpdir>` writes one item for every artifact and question kind to copy from.
2. **Validate:** `evalkit validate <dir>/suite.yaml`. Fix every error; each names `file:line:col`, the item and the key.
3. **Serve** with Bash `run_in_background: true`: `evalkit serve <dir>/suite.yaml`. It opens the browser and prints the URL (ports 8790 and up). Tell the human the URL.
4. **Wait** with Bash `run_in_background: true`: `evalkit wait <dir>/suite.yaml`. Its exit is your notification; its output is everything sent since your last wait: approved actions first, then per item only the answers and edits that changed. Never poll and never `sleep`.
5. **Act.**
   - Answers are keyed by question id. Per-row answers are `{rowId: value}` maps. `edited <artifact>` is the human's rewrite. A missing answer means your proposal stands. The full snapshot path is printed at the top if you need more.
   - For each approved action, do it (post, merge, deploy …), then record it: `evalkit done <dir>/suite.yaml <item> <action> --url <link>`. The page then shows a done link and the action cannot fire twice. Never act on an action the human did not approve.
   - To revise an item, edit what it shows (`show`). The page picks it up within 3 seconds, and the item starts a fresh review; old answers move to history.
6. **Start a new waiter at once** (step 4), before you finish acting, so no send is missed. One waiter per suite.

## Writing a good suite

- `show` is what is under review (always open), `context` is supporting material (collapsed). `questions` are asked of every item, `ask` of one item.
- Give one choice question `primary: true`: keys 1–9 answer it, and answering it marks the item reviewed (or set `reviewed: [question ids]`).
- Put your own judgment in `proposed: {question: {value, why}}`. The human sees your reason and only changes what they disagree with; overrides come back as `(proposed X)`. Keep `why` to one sentence; put evidence in `show`, where it has room.
- Code read side by side (before and after, a deleted test beside the test that keeps its proof) goes in one `compare` artifact, which spans the full page width; not in two `width: half` code artifacts.
- The controls card (the verdict) sits right under the title on every item, then `show`. Put long code last: `below: true` shows an artifact after the text questions.
- Per-row questions (`each: <artifact id>`) put a control on every row of a `list`, `table` or `transcript`; `pick` (`from: <artifact id>`) selects one row. Rows need stable `id`s.
- `when: {question: [values]}` asks a follow-up only when it applies (a correction only when a field is Wrong).
- Use an `action` question for anything outward-facing; its `summary` tells the human exactly what you will do.
- Keys `j k ? ↑ ↓ 1–9 e m f /` and Tab belong to the kit (M and E edit the editable text; F and / filter the table); give other questions single-letter `key`s. The validator reports conflicts.

## Automated judges

When a script or an LLM judge already scored the outputs (an eval run, a hill-climbing round), the human only spot-checks:

- Set `view: table`. Put judge verdicts and scores in `proposed` (pass/fail as `choice`, 0–10 or 0–1 as `number` with `min`/`max`), with the judge's reason in `why`. Overrides come back as `changed`: that is where the judge is wrong.
- Put measurements nobody should edit (cost, latency, length, deterministic checks) in item `values`, declared under suite `columns`. Add `baseline` to a value or proposal to show the change from the baseline run.
- `flag: <reason>` the rows that need a human (failures, low confidence, judge errors). Progress counts only flagged rows.
- Let the loop append one item per line to a JSONL file and point `items_from` at it. A later line with the same id replaces the earlier one. Serve before the first row lands; the table fills in live.
- Add a `text` question (e.g. "Note to Claude") so the reviewer can say what the judge missed.
- Many rounds or configurations of the same eval: give each its own JSONL and list them under `runs:`, with `baseline: true` on the reference run and `dims` such as `round`, `date` or `model`. The page pairs each case across runs, adds a Runs view, and needs no inline `baseline`. If a measured value is the score (not the verdict), mark that column `primary: true`.

## Good to know

- Feedback lives in `feedback/` beside the suite (`state.json`, `sent-NN.json`, `done/` with one file per action result). Do not edit it; keep it out of git.
- The suite is re-read on every page load: no restart after edits.
- A broken item is skipped with a banner; the page never goes blank. `evalkit validate` lists the same problems.
- Custom artifact kinds: map a kind to an ES module beside the suite with `renderers: {kind: file.js}`; see "Custom kinds" in `evalkit docs`.
