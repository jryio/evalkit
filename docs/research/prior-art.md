# Prior art for evalkit

Verified against primary docs and source on 2026-11-09. "Take" lists concepts with their exact names. "Reject" lists what does not fit a local, single-reviewer, agent-read tool.

## Argilla

Take:
- `rg.Settings` splits `fields` (read-only content: `TextField`, `ImageField`, `ChatField`, `CustomField`) from `questions`. Guidance lives in two places: `guidelines` for the dataset and a per-question `description` shown as a tooltip.
- Question types are `LabelQuestion`, `MultiLabelQuestion` (`visible_labels`, `labels_order`), `RatingQuestion` (`values`), `TextQuestion` (`use_markdown`), `RankingQuestion` and `SpanQuestion` (`field`, `allow_overlapping`). Every type has `name`, `title`, `description` and `required`.
- `Suggestion(question_name, value, score, agent)` prefills an answer and marks it with a sparkle. The human accepts it or edits it.
- `Response{question_name, value, user_id, status}` uses status `draft|submitted|discarded`. Records are `pending|draft|submitted|discarded`.
- Shortcuts: `Tab` activates the form, `↑/↓` moves between questions, `1,2,3` selects a label or rating, `Tab/Shift-Tab` moves between options, `Shift-Enter` enters a text area and `Esc` leaves it, `Enter` submits, `Backspace` discards, `Cmd-S` saves a draft, `←/→` moves between records.

Reject: vectors, typed metadata properties for filtering, `TaskDistribution(min_submitted)`, multi-user overlap, bulk view and the server/SDK model. Also `SpanQuestion`: evalkit has no NER-style need.

URLs: https://docs.argilla.io/latest/how_to_guides/dataset/ · https://docs.argilla.io/latest/how_to_guides/record/ · https://docs.argilla.io/latest/reference/argilla/records/responses/ · https://docs.argilla.io/latest/how_to_guides/annotate/

## Label Studio

Take:
- Object tags (data) and control tags (annotation) are linked by `name`/`toName`. `value="$field"` binds a tag to a data field. Each question names its target explicitly.
- `perRegion` / `perItem` on `Choices` and `Rating` ask the same question once per item. `Repeater on="$list" indexFlag="{{idx}}" mode="list|pagination"` repeats an object+control block for each array element.
- `visibleWhen` (`choice-selected`, `region-selected`, …) with `whenTagName` gives conditional questions.
- One `Choices` tag covers single and multi select with `choice="single|single-radio|multiple"`. It also has `layout="select|inline|vertical"` and `required`. `Rating` has `maxRating` and `hotkey`.
- Result shape is `result[]{id, from_name, to_name, type, value}`. Each annotation has `was_cancelled`, `lead_time` and `completed_by`.
- Each `Choice` takes a `hotkey`. The global keymap can be remapped with `EDITOR_KEYMAP`.

Reject: the XML config, region drawing on media, Visual layout tags, ML backends and projects/users. Repeater is index-based and fragile: its doc page went 404 (issue #4389) and issue #3013 reports the editor dropping its indexing.

URLs: https://labelstud.io/guide/setup · https://labelstud.io/tags/choices · https://labelstud.io/tags/rating · https://labelstud.io/tags/pairwise · https://labelstud.io/guide/export · https://labelstud.io/guide/hotkeys · http://labelstud.io.s3-website-us-east-1.amazonaws.com/tags/repeater.html

## Prodigy

Take:
- The recipe (stream + config) is separate from the interface (`view_id`). Evalkit: the suite is the stream, and the artifact and question kinds are the view_ids.
- Every task gets one top-level `answer: accept|reject|ignore`, separate from content answers (for example `accept: [option ids]` in `choice`). This is the model for evalkit's gate decision.
- `blocks` stacks several `view_id`s into one task (html + choice + text_input). This is the same as an evalkit item.
- `choice`: `options[{id, text}]`, `choice_style: single|multiple`, `choice_auto_accept`. `text_input`: `field_id`, `field_label`, `field_placeholder`, `field_rows`, `field_autofocus`, `field_suggestions`. `diff`: `added`/`removed`, `diff_style: words|chars|sentences`. `review`: `versions[]` for adjudication.
- Keys: `a` accept, `x` reject, `space` ignore, `backspace/del` undo, `cmd/ctrl+s` save, `f` flag, `1–9,0` options and `shift+num` for options 11–20. Keys are remapped with `keymap` in prodigy.json. Undo always works. `flagged: true` is a bookmark, separate from the answer.
- `compare` records randomized sides as `mapping: {A: accept, B: reject}`.

Reject: active learning and model-in-loop, spaCy coupling, `compare` (being deprecated), the commercial license and the DB layer.

URLs: https://prodi.gy/docs/api-interfaces · https://prodi.gy/docs/api-web-app

## LangSmith annotation queues

Take:
- A feedback record has `key`, `score` (number), `value` (categorical string), `comment`, `correction` (object) and `feedback_source{type: api|app|evaluator, user_id}`.
- In the rubric, every feedback key needs a description, and every category needs one too. Instructions stay pinned in the sidebar.
- Side panel statuses are "Needs Review", "Needs Others' Review" and "Completed". "Requeue" moves an item to the end of the queue.
- Pairwise queues offer "A is better" / "B is better" / "Equal" for each rubric item, with hotkeys `A`/`B`/`E`. `Enter` on the last rubric item advances.

Reject: reservations, reviewers-per-run, assigned reviewers and tracing coupling.

URLs: https://docs.langchain.com/langsmith/annotation-queues · https://docs.langchain.com/langsmith/feedback-data-format

## Braintrust human review

Take:
- Categorical scores map each option to a value from 0 to 1. Continuous scores use a slider from 0 to 1. Free-form input is written to `metadata` or to `expected` at a path.
- "Write to expected field instead of score/metadata" turns an answer into a gold label or correction (`expected.rubric.notes`).
- "Require all scores" completes the row automatically. "Blind human reviews" hides peer scores.

Reject: per-group score visibility conditions, permission groups and project-wide score management.

URL: https://docs.braintrust.dev/guides/human-review

## promptfoo

Take: each cell offers "Pass" (1.0), "Fail" (0.0), "Score" (custom 0 to 1), "Comment" and "Highlight" (mark for review). Ratings persist into exports.

Reject: actions appear only on hover (not keyboard-first), and a providers × tests grid is too dense for item-focused review.

URL: https://www.promptfoo.dev/docs/usage/web-ui/

## Inspect AI (UK AISI)

Take:
- `Sample{id, input, target, choices, metadata, files}`. `Score{value, answer, explanation, reason, metadata}`.
- Value constants `CORRECT/INCORRECT/PARTIAL/NOANSWER` are `"C"/"I"/"P"/"N"`, and `value_to_float` maps them to 1/0/0.5/0.
- `edit_score` adds a `ScoreEdit{value, explanation, provenance{author, reason}}` and keeps `history[]`. The original score stays at `history[0]`.
- Log viewer: the "Messages", "Scoring" and "Metadata" tabs, a filter by score value, and an Answer column that shows the extracted answer.

Reject: solvers and sandboxes. The viewer has no human-input path.

URLs: https://inspect.aisi.org.uk/datasets.html · https://inspect.aisi.org.uk/custom-scorers.html · https://inspect.aisi.org.uk/eval-logs.html · https://inspect.aisi.org.uk/log-viewer.html

## Chatbot Arena

Take: four buttons, "A is better", "B is better", "Tie" and "Both are bad", logged as `leftvote|rightvote|tievote|bothbad_vote`. Each vote stores both sides' state and model names, and model identity stays hidden until the vote.

Reject: Elo/Bradley-Terry aggregation and crowd voting.

URL: https://github.com/lm-sys/FastChat/blob/main/fastchat/serve/gradio_block_arena_anony.py

## OpenAI Evals

Take: samples are JSONL with `input` (chat messages) and `ideal`. Ids are versioned as `<eval_name>.<split>.<version>`.

Reject: the class-path registry and the automated templates (`Match`, `Includes`, `FuzzyMatch`).

URL: https://github.com/openai/evals/blob/main/docs/build-eval.md

## SWE-bench / HumanEval

Take:
- SWE-bench: `instance_id`, `repo`, `base_commit`, `patch`, `test_patch`, `problem_statement`, `hints_text`, `FAIL_TO_PASS`, `PASS_TO_PASS`. Test results should split into "should flip" and "must not regress".
- HumanEval: `task_id`, `prompt`, `canonical_solution`, `test`, `entry_point`. This is a code artifact with a reference solution.

Reject: the execution harnesses.

URLs: https://huggingface.co/datasets/princeton-nlp/SWE-bench · https://huggingface.co/datasets/openai/openai_humaneval

## τ-bench / SWE-agent

Take:
- τ-bench: `Task{user_id, actions[{name, kwargs}], instruction, outputs}` and `EnvRunResult{task_id, reward, info, traj, trial}`.
- τ-bench fault taxonomy: owner `user|agent|environment`, type `goal_partially_completed|used_wrong_tool|used_wrong_tool_argument|took_unintended_action`. This is a ready-made multi-select preset.
- SWE-agent `.traj`: `trajectory[]{thought, action, observation, response, state, query}` and `info{exit_status, submission, model_stats}`. This is the transcript step shape.

Reject: user simulators, and rendering `query` (the full prompt on every step, which is very large).

URLs: https://github.com/sierra-research/tau-bench · https://raw.githubusercontent.com/sierra-research/tau-bench/main/tau_bench/types.py · https://swe-agent.com/latest/usage/trajectories/

## Apple HIG

- Sidebars: "show no more than two levels of hierarchy". "Use succinct, descriptive labels." "Consider letting people hide the sidebar." "Avoid putting critical information or actions at the bottom of a sidebar." Icon colors only when they "serve a clear purpose".
- Segmented controls: "closely related choices". "No more than about five to seven segments in a wide interface". Text or images, "not a mix". Consistent segment size. "Use nouns or noun phrases."
- Sheets: "Display only one sheet at a time." Consider alternatives "for complex or prolonged user flows". macOS: "Use a panel instead of a sheet if people need to repeatedly provide input and observe results." Avoid showing Cancel, Done and Back together.
- Keyboard: "Respect standard keyboard shortcuts" (`Cmd-Z` undo, `Esc` cancel, `Cmd-W`). Custom shortcuts "for only the most frequently used" commands. Command is the main modifier. Avoid Control. Order modifiers Control, Option, Shift, Command. Support Full Keyboard Access.

URLs: https://developer.apple.com/design/human-interface-guidelines/sidebars · …/segmented-controls · …/sheets · …/keyboards (read through `developer.apple.com/tutorials/data/design/human-interface-guidelines/<page>.json`)

## Implications for the evalkit schema

1. **Keep artifacts and questions separate.** Argilla (fields/questions) and Label Studio (object/control tags) both split read-only content from what the reviewer answers. Every question gets `id`, `title`, `help` (markdown, like Argilla `description`) and `required`. All choice-like kinds share one option shape: `{id, label, help, tone, key, score}`.
2. **Give options a numeric value.** Add an optional `score` (0 to 1) to each option and scale point so the agent can aggregate. This follows Braintrust categorical scores, promptfoo Pass/Fail = 1/0 and Inspect `C/I/P/N` → 1/0/0.5/0.
3. **Add the missing kinds `pairwise` and `rank`.** `pairwise` offers `a|b|tie|both_bad` (Chatbot Arena), can be asked once per rubric criterion (LangSmith), randomizes sides and records `mapping` in the snapshot (Prodigy). `rank` follows Argilla `RankingQuestion`.
4. **Bind per-row questions to stable row keys.** Use `for_each: <artifact id>` with a `row_key` field instead of Label Studio's `perItem` and index-based Repeater. The answer is `{qid: {row_key: value}}`, so the agent can map answers back after rows are reordered.
5. **Add conditional visibility.** `show_if: {question, in: [option ids]}` (Label Studio `visibleWhen`) handles "if reject, why?". This is missing today.
6. **Keep the gate decision separate from content answers.** An item-level `decision: approve|reject|skip` works like Prodigy `answer: accept|reject|ignore`. The `action` kind runs only on `approve`. `flagged: bool` stays independent of both (Prodigy `flag`).
7. **Answer envelope.** `{value, comment?, correction?, suggested?, changed}`. `comment` and `correction` follow LangSmith. `correction` holds an expected value, like Braintrust "write to expected". `suggested` is an agent prefill with `agent` and `score` (Argilla `Suggestion`), and `changed` records whether the human edited it. Each snapshot carries `suite_id@version`, `item_id`, `status: draft|sent|skipped` and `author`. Snapshots never edit an answer in place: history is preserved, as in Inspect `ScoreEdit.provenance` + `history[]`.
8. **Keyboard model.** `1–9,0` pick options and `shift+num` reaches 11–20 (Prodigy). `↑/↓` or `Tab` move between questions (Argilla). `Enter` enters a text field and `Esc` leaves it. `Cmd-Enter` sends. `j/k` move between items. `f` flags. `Cmd-Z` undoes the last send (HIG). Bare-letter keys are inactive while a text field has focus. Never bind Control. Each option shows its key next to it. Per-option `key` overrides the default.
9. **Layout rules.** The sidebar is a list of items with status badges: pending/draft/sent/skipped/flagged, like LangSmith's statuses. It has at most two levels (group → item) and nothing critical at the bottom. A `single` question renders as a segmented control only up to 5–7 short noun options, and falls back to a list beyond that. Use a sheet only to confirm an `action`, never to answer questions.
10. **Artifact presets from benchmarks.** The `transcript` step uses SWE-agent `{thought, action, observation}` and takes per-step questions through `for_each`. `tests` splits into SWE-bench `FAIL_TO_PASS`/`PASS_TO_PASS` buckets. Ship a τ-bench fault-taxonomy multi-select preset for agent-trajectory review.
