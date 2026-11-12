# Coding agents

Several agents can share one checkout when each one has its own stage. Give every agent a stage ID, usually its own agent ID, and the instructions in [AGENTS.md](../AGENTS.md). From there an agent either selects hunks by ID, or, with an editor hook installed, stages exactly the edits it made.

## Editor hooks

Hunk selection asks an agent to recognize its own lines in a diff, and it cannot split edits on adjacent lines. An editor hook removes both problems. It runs before and after each file edit, records the file's content on both sides of the edit in the agent's stage, and `add --edits` replays those recorded edits:

```bash
git multistage --stage <id> add --edits && git multistage --stage <id> commit -m "Add input validation"
```

While it runs, the hook holds a short lease on the edited file, so two hooked agents never write one file at the same moment. A second agent's edit waits for the lease, up to 20 seconds, and is then blocked with a message that asks the agent to retry.

The first recorded edit of a stage tells the agent its stage ID, through the hook's message to the model:

```text
git-multistage records your file edits in stage claude-a3e884bee24c6a0de. To commit them:
`git multistage --stage claude-a3e884bee24c6a0de add --edits` then
`git multistage --stage claude-a3e884bee24c6a0de commit -m "<message>"`.
```

The hook records only the editor's own file tools. Files that an agent changes through shell commands, such as `sed`, a formatter, or a code generator, are staged with `add` or `add --hunk` as usual.

### Claude Code

Add the hook for the `Edit`, `MultiEdit`, and `Write` tools to the project's `.claude/settings.json`:

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

Add the hook for the `apply_patch` tool to the project's `.codex/hooks.json`, or to `~/.codex/hooks.json` for every project:

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

### Overriding the stage

When `GIT_MULTISTAGE_ID` is set in the agent's environment, or the hook command passes `--stage <id>`, the hook records into that stage instead.

## --agentic

`--agentic` is an opt-in shortcut for agents that have no stage ID of their own. When no stage ID is given, it takes one from the environment: `CLAUDE_CODE_SESSION_ID` as `claude-<id>`, or else `PASEO_AGENT_ID` as `paseo-<id>`. Without the flag, git-multistage ignores both variables.

Claude Code sets `CLAUDE_CODE_SESSION_ID` in some launch modes, such as the Agent SDK, but not in all of them. Where it is set, it names the same stage that the Claude Code hook uses for the main session. Subagents inherit their parent's environment, so `--agentic` cannot tell subagents apart. A subagent uses the stage ID that the hook gives it, or an explicit `--stage`.

## Limits

[Limitations](limitations.md#editor-hooks) covers what the hooks cannot see: edits made through shell commands, leases that expire during a long permission prompt, and Codex's trust step.
