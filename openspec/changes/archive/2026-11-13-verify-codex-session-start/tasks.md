# Tasks

## 1. Verify

- [x] 1.1 Install the plugin from the local marketplace into a scratch `CODEX_HOME`, so `~/.codex/config.toml` stays untouched, and run with `--dangerously-bypass-hook-trust` for that one invocation
- [x] 1.2 Run `codex exec` in a scratch repository and record whether the model quotes the session start notice
- [x] 1.3 Delete the scratch `CODEX_HOME` and the scratch repository

## 2. Act on the result

- [x] 2.1 If the notice arrived: add a test of the hook's `SessionStart` output for a Codex input (`TestHookReplay/codex-session-start` already covers it)
- [x] 2.2 Not needed, the notice arrived. If it had not: add the notice to the skill description, test that the skill front matter carries it, and update the spec scenario for Codex
