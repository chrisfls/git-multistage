# Proposal

## Why

The change `plugin-for-every-harness` tested most harnesses only by replaying their documented hook input, or by running the plugins in a fake host. Real sessions in the harnesses that can be installed here found defects that those tests could not show:

- OpenCode 1.19.5's `opencode plugin` installs only packages from the npm registry, and skips a GitHub package named in its config, so the README's install step loads nothing.
- The JavaScript plugins attach the session start notice to the session's first file-tool call. When that call fails before it runs, the host reports no result, and the notice is lost.
- The npm package of Kilo Code runs its binary as `.kilo`, so the OpenCode plugin names Kilo's stages `opencode-…`.
- Oh My Pi loads both `hooks/pre/git-multistage.js` and the Pi extension of `package.json`, so one session records into two stages.
- Qwen Code runs the hooks of the Claude Code plugin that it converts, so the extra `hooks/qwen.json` of the README runs every hook twice, into two stages.
- The Cline CLI names its hook events `agent_start`, `tool_call`, and `tool_result`, and its file tool `editor`, so the hook records nothing.
- Kiro CLI reads hooks from an agent's configuration, with the triggers `agentSpawn`, `preToolUse`, and `postToolUse`, not from `~/.kiro/hooks/`.
- Grok Build has a third file tool, `hashline_edit`.
- Several README steps need a step more: CodeBuddy installs the plugin disabled, Gemini CLI and Qwen Code ask for confirmation, Copilot deprecates the direct install, MiMo reads its plugins from `mimocode.json`, and Kilo loads no skills from a plugin.

There is also no repeatable way to run the flagship harnesses, Claude Code, Codex, and OpenCode, with the plugin before a release.

## What Changes

- The hook reads the Cline CLI's event names and its `editor` tool, and Grok's `hashline_edit`. Without an editor name, it takes `droid` from `DROID_PLUGIN_ROOT` and `qwen` from `QWEN_PROJECT_DIR`.
- The JavaScript plugins keep the session start notice of a session until the result of one of its calls carries it. The OpenCode plugin strips a leading `.` from the host's binary name. The Pi extension takes the editor `omp` in Oh My Pi, and `hooks/pre/git-multistage.js` is removed.
- `hooks/qwen.json` is removed. `hooks/kiro.json` holds the `hooks` object of a Kiro agent configuration.
- The README's install steps follow what the real CLIs accept.
- `scripts/live-check.sh` runs one real session of Claude Code, Codex, and OpenCode with this checkout's plugin, and checks that each agent commits only its own change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `editor-hooks`: the Cline CLI's hook input, Grok's `hashline_edit`, and the variables of Droid and Qwen Code.
- `agent-plugins`: the session start notice of the JavaScript plugins, the stage prefix in Kilo Code and Oh My Pi, and the hook files of Qwen Code and Kiro CLI.
- `local-checks`: the live check of the flagship harnesses.

## Impact

- `internal/ms/hookinput.go`, `hooks/hooks.json`, `hooks/kiro.json`, `plugins/opencode/`, `plugins/pi/`, `plugins/amp/`, `README.md`, and the tests.
- Removed: `hooks/qwen.json`, `hooks/pre/git-multistage.js`.
- New: `scripts/live-check.sh`. It costs one short model session for each agent, and runs only by hand.
- The GitHub repository is private, so every install step that fetches it works only for an account with access. Its visibility is the owner's decision and out of scope.
