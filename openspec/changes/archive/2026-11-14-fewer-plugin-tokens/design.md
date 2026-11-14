# Design

## Context

The plugin puts four texts into an agent's context. See proposal.md for their cost.

| Text | Source | When the agent receives it |
| --- | --- | --- |
| Skill description | `skills/git-multistage/SKILL.md` front matter | Every session, in the skill list |
| Skill body | `skills/git-multistage/SKILL.md` | Each time the agent loads the skill, usually before its first commit |
| Session start notice | `announceSharedCheckout` in `internal/ms/hook.go` | Each `SessionStart`: start, resume, clear, and compaction in Claude Code |
| First-edit message | `announceStage` in `internal/ms/hook.go` | Once per stage, after its first recorded edit |

Claude Code runs `SessionStart` for a session and not for its subagents. A subagent's hook input carries `agent_id`, and its edits go to the stage `<editor>-<agent_id>`.

## Goals / Non-Goals

**Goals:**

- Fewer tokens in each of the four texts, with the same instructions and the same triggers.

**Non-Goals:**

- The plugin manifests' descriptions. Claude Code and Codex show them in their plugin screens and do not give them to the model.
- A rule that asks the agent to write in a shorter style to the user.

## Decisions

### Compress by removing repeats, not grammar

The rewrite applies the Caveman compression rules that keep meaning: it merges text that says the same thing twice, keeps one example where several show the same pattern, and copies code, commands, paths, and every `not`, `never`, and `only` verbatim. It leaves out the rules that drop articles and write fragments. The research that Caveman cites found that compressed prompts make answers worse, and a model tends to write in the style of its context, which would leak the style into replies to the user.

From ASD-STE100, the rewrite keeps one term for each concept (stage, stage ID, recorded edits), active voice, and every condition and hedge. The current text already follows most of it.

Alternative: a fragment-style rewrite. It saves more tokens, but risks both effects above.

### Send the agent to the CLI's hints

Each error that the failure table described prints a `hint:` line with the same step, or the `hint` field with `--json`: `edit-conflict` and `path-deleted` (`internal/ms/edits.go`), `conflict` and `path-deleted` at commit (`internal/ms/integrate.go`), and `stale-hunk` (`internal/ms/stage.go`). The skill sends the agent to the hint, and keeps its own step only where the hint does not fit an agent in a shared checkout:

| Failure | The hint says | The skill says instead |
| --- | --- | --- |
| `shared-path` | stage your hunks, or pass `--allow-shared` | Stage your hunks. |
| `branch-mismatch`, `operation-in-progress` | check out the branch again, or finish or abort the merge or rebase | Stop and report it. Do not change branches. |
| `index-locked`, `busy`, `too-many-retries` | delete the lock file if no Git process runs, or no hint | Retry after a few seconds, then run `doctor` and report its output. |
| `hook-modified` | stage your changes again | Stage them again with `hunks` and `add --hunk`. |
| `nothing new to commit` | no error | It is a success. |
| Codex sandbox | an operating system error | Run with escalated permissions. |

Each row keeps the current skill's step. The skill names a failure by its exit code and its cause, because the text output prints the message and the hint, and only `--json` prints the kind.

Alternative: move the failure steps to a second file that the agent reads after a failure. The steps that stop an agent from changing branches or deleting a lock would then depend on a read that the agent can skip, so they stay in the skill body.

### Order the stage ID sources

The skill tells the agent to use, in order: the stage that a git-multistage message names, its agent ID, and a stage from `init`. The hook records into the stage that its message names, so `add --edits` with any other ID finds no records. The message arrives only after the first edit, so the skill tells the agent to make that edit first. In the smoke runs of task 4.1, agents that chose an ID before their first edit ran `init`, which cost a tool call and left an empty stage behind.

### Note about other agents in the first-edit message

`announceStage` adds `otherAgentsNote` only when the hook input has an `agent_id`. A main session received the note in its session start notice, which Claude Code sends again after each compaction. A subagent has no other way to receive it.

### Plugin version

Claude Code caches an installed plugin in `~/.claude/plugins/cache/<marketplace>/<plugin>/<version>/`, one folder for each version, so an install keeps the files of the version it has until the version changes. The manifests have named `0.1.0` since the plugin shipped, so earlier changes to the skill and hooks have not reached existing installs. This change sets the three manifests to the version of its release, and a test checks that the three agree.

## Measurement

The `o200k_base` tokenizer stands in for the model's tokenizer, which has no public counter. It is close enough to compare a text before and after. The measured results are in tasks.md.

## Risks / Trade-offs

- [A future hint that does not fit a shared checkout] → The skill's exceptions cover every hint that the CLI prints today. A new hint that changes shared state needs a row in the skill.
- [A main session that never received `SessionStart`, such as a session that started before the plugin was installed] → It still receives the first-edit message with its stage and commands. It misses the note about other agents until its next session start.
