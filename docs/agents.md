# Coding agents

Several agents can share one checkout when each one has its own stage. Give every agent a stage ID, usually its own agent ID, and the instructions in [the agent guide](agent-guide.md). From there an agent either selects hunks by ID, or, with an editor hook installed, stages exactly the edits it made.

## Plugins

The repository is a plugin for Claude Code and for Codex. The plugin installs the editor hook below and a `git-multistage` skill that holds [the agent guide](agent-guide.md). It calls the `git-multistage` binary, so install that first.

Claude Code:

```text
/plugin marketplace add chrisfls/git-multistage
/plugin install git-multistage@git-multistage
```

Codex:

```bash
codex plugin marketplace add chrisfls/git-multistage
codex plugin add git-multistage@git-multistage
```

The next time Codex starts, it asks you to review the plugin's two hooks; trust them, or the hooks do not run. Codex installs a plugin from a local marketplace path from that repository's committed files.

To configure the hooks by hand instead, use the sections below.

## Editor hooks

Hunk selection asks an agent to recognize its own lines in a diff, and it cannot split edits on adjacent lines. An editor hook removes both problems. It runs before and after each file edit, records the file's content on both sides of the edit in the agent's stage, and `add --edits` replays those recorded edits:

```bash
git multistage --stage <id> add --edits && git multistage --stage <id> commit -m "Add input validation"
```

While it runs, the hook holds a short lease on the edited file, so two hooked agents never write one file at the same moment. A second agent's edit waits for the lease, up to 20 seconds, and is then blocked with a message that asks the agent to retry. A lease expires after 15 seconds. When another agent then takes the file, the edits of both agents are recorded as uncertain, and `add --edits` refuses them; stage that file with `hunks` and `add --hunk`.

The first recorded edit of a stage tells the agent its stage ID, through the hook's message to the model:

```text
git-multistage records your file edits in stage claude-a3e884bee24c6a0de. To commit them:
`git multistage --stage claude-a3e884bee24c6a0de add --edits` then
`git multistage --stage claude-a3e884bee24c6a0de commit -m "<message>"`.
```

The hook records only the editor's own file tools. Files that an agent changes through shell commands, such as `sed`, a formatter, or a code generator, are staged with `add` or `add --hunk` as usual.

### Claude Code

Without the plugin, add the hook for the `Edit`, `MultiEdit`, and `Write` tools to the project's `.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [{"matcher": "Edit|MultiEdit|Write", "hooks": [{"type": "command", "command": "git multistage hook claude"}]}],
    "PostToolUse": [{"matcher": "Edit|MultiEdit|Write", "hooks": [{"type": "command", "command": "git multistage hook claude"}]}]
  }
}
```

The main session records into `claude-<session_id>`. A subagent records into `claude-<agent_id>`, because Claude Code passes each subagent's `agent_id` to the hook.

### Codex

Without the plugin, add the hook for the `apply_patch` tool to the project's `.codex/hooks.json`, or to `~/.codex/hooks.json` for every project:

```json
{
  "hooks": {
    "PreToolUse": [{"matcher": "^apply_patch$", "hooks": [{"type": "command", "command": "git multistage hook codex"}]}],
    "PostToolUse": [{"matcher": "^apply_patch$", "hooks": [{"type": "command", "command": "git multistage hook codex"}]}]
  }
}
```

The hook reads the edited files from the patch headers (`Update File`, `Add File`, `Delete File`, and `Move to`). A session records into `codex-<session_id>`, and a subagent into `codex-<agent_id>`.

Codex runs a hook only after you trust its definition. Start `codex` in the project once and review the hooks it lists, or open `/hooks`. Codex reads a project's `.codex/hooks.json` only when the project itself is trusted.

Codex's default sandbox (`workspace-write`) makes `.git` read-only, so `git multistage` fails there, as `git commit` does. A rule lets Codex run it outside the sandbox without asking. Add it to `.codex/rules/multistage.rules` in the project, or to a file in `~/.codex/rules/`:

```python
prefix_rule(pattern = ["git", "multistage"], decision = "allow")
```

Like project hooks, project rules load only when the project is trusted. The rule also covers commands that the agent should not run on its own, such as `drop --force`; use `pattern = ["git", "multistage", "add"]` and similar lines to allow single commands instead.

### Overriding the stage

A hook command with `--stage <id>` records every edit into that stage.

`GIT_MULTISTAGE_ID` in the agent's environment renames the main session's stage only. Subagents inherit that variable, so it cannot tell them apart; they keep recording into `claude-<agent_id>` or `codex-<agent_id>`.

## --agentic

`--agentic` is an opt-in shortcut for agents that have no stage ID of their own. With no stage ID given, it reads the agent's ID from the environment. Without the flag, git-multistage ignores these variables.

| Priority | Variable | Set by | Stage |
| --- | --- | --- | --- |
| 1 | `CLAUDE_CODE_SESSION_ID` | Claude Code, for each agent's commands | `claude-<id>` |
| 1 | `CODEX_THREAD_ID` | Codex, for each agent's commands | `codex-<id>` |
| 2 | `ORCA_AGENT_SESSION_ID` | Orca, for each agent session it starts | `orca-<id>` |
| 2 | `PASEO_AGENT_ID` | Paseo, for each agent it starts | `paseo-<id>` |

Priority 1 names the same stage that the editor hook uses for the agent's main session.

A priority-1 variable counts only when its tool is one of git-multistage's parent processes: `CLAUDE_PID` or a process named `claude` for Claude Code, a process named `codex` for Codex. A process started from a tool inherits the tool's variable, such as a daemon started from a Claude Code shell, and so do the agents that the daemon starts. When the tool is not an ancestor, the next variable that is set wins, such as `PASEO_AGENT_ID`. A tool's variable that is the only one set is still used. When the process list cannot be read (no `ps`), the priority order alone decides.

Agents can be nested, as when an agent runs `claude -p` or `codex exec`. A nested Claude Code or Codex agent sets its own variable, and also inherits the outer tool's. When both priority-1 variables are set, git-multistage takes the tool that is the nearer ancestor. If neither is an ancestor, a priority-2 variable decides; without one, it refuses with exit code 2.

Priority 2 applies only when no priority-1 variable counts, for example for OpenCode or Pi started by Orca. Orca and Paseo set one ID per agent they start, and the agent's subagents and nested agents inherit it, so these IDs cannot tell those apart. When both are set, `--agentic` refuses.

Subagents inherit their parent's variables at every level. A subagent uses the stage ID that the hook gives it, or an explicit `--stage`.

## Limits

[Limitations](limitations.md#editor-hooks) covers what the hooks cannot see: edits made through shell commands, leases that expire during a long permission prompt, and Codex's trust step.
