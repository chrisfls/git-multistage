# Design

## Context

The plugin puts four texts into an agent's context. Token counts use the `o200k_base` tokenizer, for a stage ID of `claude-` and a UUID.

| Text | When the agent receives it | Tokens |
| --- | --- | ---: |
| Skill description, with its `name` line | Every session of every repository, in the skill list | 48 |
| Skill file | Each time the agent loads the skill, usually before its first commit | 746 |
| Session start notice | Each `SessionStart` and `SubagentStart` in a shared checkout | 180 |
| First-edit message | Once per stage | 94 |

The skill also sets which commands the agent runs, so its procedure decides how much command output the agent reads.

## Goals / Non-Goals

**Goals:**

- Fewer tokens in the skill and in the output of the commands that it tells the agent to run, with the same instructions and the same behavior in Claude Code and Codex.

**Non-Goals:**

- A shorter session start notice or first-edit message. See the decisions below.
- A rule that asks the agent to write in a shorter style to the user, or text in a compressed style.
- The CLI's output, the stage ID format, and the number of Git calls.

## Decisions

### Drop `diff --cached` from the commit procedure

`add --edits` stages only the edits that the hook recorded, and prints each staged path, such as `staged: M  c.txt`. `diff --cached` then prints the whole change, which the agent wrote itself, so the agent reads its change a second time. The cost grows with the size of the change. The agent can still run `diff --cached` when it wants to see the change; the skill just no longer makes it a step.

Alternative: replace it with `status`. Its output repeats the paths that `add` printed.

### Description triggers

The trigger "when a git-multistage message names your stage" comes only in a shared checkout, after an edit, and the agent that acts on the message stages or commits, which the first trigger covers. The description keeps that trigger and the failed-command trigger: 48 to 35 tokens, in every session.

### Skill sentences

Each sentence that repeats an instruction goes, such as "It stages what you wrote", which restates the sentence before it, and "Use that ID from then on", which "Pass your stage ID on every command" covers. The `--json` line goes: the agent learns of `--json` from `--help` when it parses output, and the text output is shorter. Code, commands, paths, and every `not`, `never`, and `only` stay verbatim. The sentences stay whole, with their articles.

### Leave the notice and the first-edit message

A rewrite of the notice that lists the Git commands once, `the Git commands add, commit, ...`, and keeps every rule saves 7 of 180 tokens. The notice's wording came from the s7 ablations of `shared-checkout-rules`, so a change would need those runs again for under 4%. The first-edit message is paid once per stage, and most of its tokens are the stage ID, which the agent must copy.

## Evaluation

`claude -p` with Haiku 5.5 and the plugin from `--plugin-dir`, and `codex exec` with its plugin installed in a scratch `CODEX_HOME`, as `scripts/live-check.sh` runs them. Each scenario runs in a scratch shared checkout where stage `other` holds a staged change of `b.txt`, and `a.txt` and line 5 of `c.txt` hold another agent's unstaged edits.

| Scenario | Task | Good behavior |
| --- | --- | --- |
| s1 | Change line 2 of `c.txt` with the file tool and commit | The commit holds line 2 only; the other agents' changes stay |
| s2 | Change line 2 of `c.txt` with a shell command and commit | The commit holds line 2 only, staged by hunk |

Before is v0.1.13; after is this change. Claude Code runs 3 times per cell, Codex 2. The change ships when each scenario's success count is equal or better in both agents. The results record the input and output tokens of each run.

## Risks / Trade-offs

- [An agent commits something it did not mean to, because it no longer reads the diff] → `add --edits` stages only recorded edits, and `add` prints the staged paths. s1 and s2 check the commit's content.
- [An agent no longer loads the skill when the first-edit message arrives] → the message names the commands itself, and s1 records whether the agent loaded the skill.
