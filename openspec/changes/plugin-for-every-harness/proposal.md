# Proposal

## Why

git-multistage ships its agent plugin for Claude Code and Codex only, and the hook reads only their two hook inputs. The supported harnesses are the 30 of the P0, P1, and P2 tiers, plus Gemini CLI for compatibility: Claude Code, Codex, Antigravity, OpenCode, Devin CLI, Grok Build, Kimi Code, Muse Code, DeepSeek Harness, ZCode, Qwen Code, GitHub Copilot CLI, Cursor CLI, Kiro CLI, Mistral Vibe, MiniMax Code, MiMo Code, Qoder CLI, Pi, Hermes Agent, Factory Droid, CodeBuddy, Amp, Cline, Kilo Code, Goose, Oh My Pi, Aider, OpenHands, CodeArts Agent, and Gemini CLI.

Research into each harness's documentation and source shows three facts:

- Most of them can install the existing Claude Code plugin. Antigravity converts it at install, and Droid, Copilot, Devin, Grok, ZCode, MiniMax, Qoder, CodeBuddy, OpenHands, Goose, Oh My Pi, Qwen, and DeepSeek Harness read `.claude-plugin/`, `hooks/hooks.json`, or `skills/`.
- Their hooks send JSON in other shapes, such as camelCase fields, a nested `toolCall`, or the event names `BeforeTool` and `pre_tool`. They name their file tools differently, such as `write_to_file`, `search_replace`, `file_editor`, and `fs_write`. The hook records no edit from them, and the matcher in `hooks/hooks.json` does not select their tools.
- The rest load plugins of another kind: JavaScript modules (OpenCode, Kilo, MiMo, CodeArts, Pi, Oh My Pi, Amp), a Python package (Hermes), their own manifests (Cursor, Kimi, Muse, Vibe, Gemini), or hook files that the user copies (Kiro, Cline). Aider has no plugins and no hooks.

## What Changes

- `git multistage hook [<editor>]` accepts any editor name in lower case. The name is the prefix of the stage ID, as `claude` and `codex` are now.
- The hook reads the hook input of every supported harness: its event names, its fields for the tool, the tool input, the session, the subagent, the tool call, and the working directory, and its signs of a failed tool call. It finds the edited files from the path field of a file tool, or from the file headers of an `apply_patch` patch in any harness.
- Without an editor name, the hook names the harness from its input and from the variables that the harness sets for hook commands, so one `hooks/hooks.json` serves every harness that reads it.
- The hook writes its messages in the output format of each harness. In Antigravity and Kiro, whose post-tool output does not reach the model, the session start notice also names the stage.
- New install files: `.cursor-plugin/`, `.kimi-plugin/`, `.muse-plugin/`, `.hermes-plugin/`, a root `package.json` for OpenCode and Pi, JavaScript plugins for OpenCode, Pi, and Amp, a Vibe plugin folder and a Gemini CLI extension folder, a root `hooks.json` for Devin, and hook files for Qwen, Kiro, and Cline. The matcher in `hooks/hooks.json` selects the file tools of every harness that reads it.
- The README gives the install steps of each harness. For Aider, it gives the read-only conventions file and the setting that turns off Aider's own commits.
- Every manifest carries the one plugin version.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-plugins`: an install path for each supported harness, the tools that the shared hooks file selects, and one version in every manifest.
- `editor-hooks`: editor names, the hook input of each harness, the harness without an editor name, the output format of each harness, and where the stage ID reaches the model.
- `agent-stage-detection`: a stage ID that the hook builds starts with the name of any editor.

## Impact

- `internal/ms/hook.go`, `internal/ms/ownchange.go`, `internal/ms/cli.go`, and a new `internal/ms/hookinput.go`.
- `hooks/hooks.json`, `skills/git-multistage/SKILL.md` (the sentence that names the file tools), `README.md` (plugin install), and the existing manifests' version.
- New files: the manifests, plugins, and hook files above; hook inputs in `test/testdata/hooks/`; tests for each harness.
- The change shares `internal/ms/hook.go`, `hooks/hooks.json`, `SKILL.md`, and `README.md` with the change `shared-checkout-rules`, and builds on it: the hooks act only in a checkout that sets `multistage.shared`.
- Out of scope: `--agentic` for the new harnesses' environment variables, a guard of shell commands in harnesses other than Claude Code and Codex, and edits that harnesses make through shell commands.
