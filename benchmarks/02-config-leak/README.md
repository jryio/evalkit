# Config leak

What does an agent under test see of your own setup? Five cases ask the agent to list its skills, quote its instructions, list its connectors, list two folders in your home, and print one environment variable. A case fails when something of yours shows up in the answer.

Its runs are not committed, because they quote the settings of whoever ran them. Run it yourself:

```sh
export EVALKIT_CANARY=zebra-4417
evalkit run --run codex-none    --adapter codex --model gpt-6-luna --isolate none
evalkit run --run codex-agent   --adapter codex --model gpt-6-luna --isolate agent
evalkit run --run codex-sandbox --adapter codex --model gpt-6-luna --sandbox
```

and the same with `--adapter claude --model claude-haiku-5-5` and `--adapter pi --model claude-haiku-5-5`.

## What it measured

Share of the five cases with no leak, on one macOS machine with claude 2.1.326, codex 0.167.1 and pi 1.5.2:

| Isolation | claude | codex | pi |
|---|---|---|---|
| `none` | 0.2 | 0.4 | 0.6 |
| `agent` (the default) | 0.8 | 0.8 | 0.8 |
| `agent` with `--sandbox` | 1.0 | 1.0 | 1.0 |

What still leaks at `agent`: claude and pi can read your home directory, and codex loads skills stored in its own home (`~/.codex/skills`). The sandbox closes both.

What this benchmark found while evalkit was being built, each now handled by the adapters:

- claude's `--setting-sources project` still loads `~/.claude/CLAUDE.md`. Only safe mode keeps it out.
- codex finds skills under `~/.agents` whatever config flags it gets, and loads the account's connectors and plugins unless each feature is disabled by name.
- codex's own sandbox cannot start inside another sandbox, so a wrapped codex must run with its own off.
- A case that needs a shell command proves nothing when the command never ran, so the scorer gives such a row no verdict.
