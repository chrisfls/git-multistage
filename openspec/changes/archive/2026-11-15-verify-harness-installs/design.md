# Design

## Context

See proposal.md for the defects. Each harness was installed under a scratch home, with its configuration in scratch folders, and given the README's install step, or a local clone where the private repository refused the fetch. Where a harness can use `OPENROUTER_API_KEY`, one real session in a scratch shared checkout changed `c.txt` and committed it with `git multistage`. The checkout held another stage's staged change of `b.txt` and another agent's unstaged edit of `a.txt`. The results are in tasks.md.

## Decisions

### Session start notice per session

The JavaScript plugins send `SessionStart` at a session's first file-tool call, and add the notice to the result of a call. OpenCode, Pi, and Amp report no result for a call that fails before it runs, so a notice tied to the first call's ID was lost when that call failed. The plugins now keep the notice by session ID, and remove it once a call's result carries it.

### Stage prefix in forks

The OpenCode plugin names the editor after the host binary. The npm package of Kilo Code runs `.kilo`, so the name is read without a leading `.`. Oh My Pi loads the Pi extension that `package.json` names, and sets `process.title` to `omp`; the extension takes the editor `omp` there. `hooks/pre/git-multistage.js` made Oh My Pi load the extension a second time, and is removed.

### One source of hooks for Qwen Code

`qwen extensions install` converts the Claude Code plugin with its `hooks/hooks.json`, which runs `git multistage hook` without an editor. The binary takes `qwen` from `QWEN_PROJECT_DIR`, which Qwen sets for hook commands, before the Gemini and Claude Code variables that Qwen also sets. `hooks/qwen.json` added a second set of hooks and is removed.

### Hooks without SubagentStart

OpenHands rejects a hooks file with `SubagentStart`; Droid and ZCode skip that event with a warning and run the others, which a marker hook in Droid confirmed. Claude Code needs the event, so `hooks/hooks.json` keeps it, and OpenHands gets `hooks/openhands.json`, the same file without it, which a test keeps in step.

### Cline CLI and Kiro CLI

The Cline CLI sends the Cline payload with `hookName` `agent_start`, `tool_call`, and `tool_result`, and the file tool `editor`. The binary maps the names, and finds the nested event object by the mapped name. Kiro CLI 3.3 reads hooks only from an agent's configuration, with camelCase triggers, so `hooks/kiro.json` is that `hooks` object; Kiro's agent validator needs a login, so the format is from the binary's strings and stays unverified.

### Hermes working directory

Hermes changes to its own install folder, so `os.getcwd()` named the wrong repository. The plugin asks `agent.runtime_cwd.resolve_agent_cwd()`, and falls back to `os.getcwd()` in an older Hermes or a test host.

### Live check script

`scripts/live-check.sh` builds the binary into a scratch folder, puts it first on `PATH`, and runs each named agent in a scratch shared checkout: Claude Code with `--plugin-dir`, Codex with the plugin installed into a scratch `CODEX_HOME` that holds a copy of the login, and OpenCode with the plugin named by its `file://` path in a scratch `XDG_CONFIG_HOME`. It runs by hand only, because each run costs a model session.

## Risks / Trade-offs

- [Harnesses that need an account here] → Cursor, Kiro, Devin, Amp, CodeBuddy, MiniMax, Muse, and CodeArts were not run live; tasks.md lists what was checked for each.
- [The private repository] → every remote install step fails for an account without access. The owner decides the visibility.
