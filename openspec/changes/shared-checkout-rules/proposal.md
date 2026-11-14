# Proposal

## Why

The skill holds two kinds of text: the procedure to stage and commit, and the rules of a shared checkout. An agent loads the skill only before it stages or commits, so an agent that runs `git stash` or `git checkout -- <file>` for another reason never sees the rules that forbid it. Neither Claude Code nor Codex loads a rules or instructions file from a plugin. The plugin's channel for rules is the text that its hooks add at session start.

The hooks also act in every Git repository, including a repository that one agent uses alone. There they add the shared-checkout notice to every session, create a stage at the first edit, and steer the agent to `git multistage`.

A baseline run of the current plugin, v0.1.6, found one failure in each of two Codex commit runs. Codex edited the file with a shell command, so the hook recorded nothing, and then staged the whole file with `add <path>`, which took another agent's unstaged edit into the commit. Claude Code agents mentioned the other agents' files in two of three replies, against the notice.

The skill's failure table exists because some CLI hints tell a person to do what an agent in a shared checkout must not do, such as "check out main again" or "delete the lock file".

## What Changes

Each change below ships only if the evaluation in tasks.md shows better or equal behavior in Claude Code and in Codex. A change that does not is dropped before the archive.

- **BREAKING** The hooks act only in a checkout whose Git config sets `multistage.shared` to `true`. Elsewhere they record nothing, create no stage, and print nothing.
- The session start notice carries the rules of a shared checkout: other agents' work is theirs, edits go through the file-editing tools, and the Git commands that change shared state are not run. A `SubagentStart` hook gives a subagent the same notice. The first-edit message names the stage and the commands only.
- A `PreToolUse` hook on shell commands denies those Git commands in an opted-in checkout, with a reason that names the command to use instead.
- When an agent runs the CLI, the hints that would change shared state give steps that leave it alone, and a read-only `.git` in Codex gets a hint to run with escalated permissions.
- The skill describes when to use it in its description, and holds the procedure only. It lists hunks before it stages a whole file, and sends the agent to an error's hint.
- The README describes the opt-in, and the rules to put in AGENTS.md for an agent without the plugin.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `editor-hooks`: the opt-in, the notice's rules, the subagent start notice, the first-edit message, and the shell command check.
- `agent-plugins`: the hook events of the plugin, the skill description, and the skill's failure steps.
- `cli`: hints for an agent.

## Impact

- `internal/ms/hook.go`, a new `internal/ms/guard.go`, the hints in `internal/ms/commit.go`, `indexsync.go`, `stage.go`, `stagearea.go`, and the error output in `cli.go`.
- `hooks/hooks.json`, `skills/git-multistage/SKILL.md`, `README.md`, and the plugin manifests' version.
- Every hook test that edits a file sets `multistage.shared`.
- Out of scope: Git commands outside the current rules, such as `git clean` and `git rebase`, and a CLI refusal of a whole-file `add` that holds another stage's recorded edits.
