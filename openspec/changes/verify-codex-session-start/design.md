# Design

## Verification

Install the plugin from the local marketplace into Codex, trust only its hooks, and start `codex exec` in a scratch Git repository with a prompt that asks the model to quote any message about other agents before it calls a tool. Read the result and the session's hook log.

## Decision rule

- The notice arrives: add the regression test of `git multistage hook` with a Codex-shaped `SessionStart` input, and mark the scenario done.
- The notice does not arrive: the hook's output for Codex is not the problem to solve in the hook alone. Put the same text in the `git-multistage` skill description, which Codex shows to the model from the start, and keep the `SessionStart` output for Claude Code. The spec scenario for Codex then names that path.

## Cleanup

Restore `~/.codex/config.toml` from a copy made before the test, and uninstall the plugin and the local marketplace.

## Result

`codex exec` 0.168.0 with the plugin installed in a scratch `CODEX_HOME` and hook trust bypassed for the invocation: the `SessionStart` hook ran, and the model quoted the notice before any tool call. The spec scenario for Codex holds, and `TestHookReplay/codex-session-start` is its regression test.
