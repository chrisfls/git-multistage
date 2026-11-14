# Design

## Context

The plugin has three parts: the `git-multistage` skill, a session start notice, and the hooks around file tools that record each edit in the agent's stage. See proposal.md for the harnesses and why they get nothing today. After the change `shared-checkout-rules`, the hooks act only in a checkout that sets `multistage.shared`, and the hooks file also runs at `SubagentStart` and before `Bash`.

The facts below come from each harness's documentation, its source code, or its npm package, read on 2026-11-14. Antigravity's are also from the local `agy` 1.5.8: `agy plugin install` of this repository's Claude Code plugin into a scratch home, and one `SessionStart` input that a capture hook received. Its account could not run a model turn, so no tool hook input was captured.

| Harness | Install | Skill | Notice | Edits recorded | Hook input | Source of the facts |
| --- | --- | --- | --- | --- | --- | --- |
| Claude Code | Claude plugin | yes | yes | yes | Claude | in use |
| Codex | Codex plugin | yes | yes | yes | Claude, `apply_patch` | in use |
| Antigravity | `agy plugin install` converts the Claude plugin | yes | yes, with the stage | yes | camelCase, `toolCall{id,name,args}`, `conversationId` | local `agy` |
| OpenCode | root `package.json`, JS plugin | yes | in a tool result | yes | from the plugin | `@opencode-ai/plugin` types |
| Devin CLI | reads `.claude-plugin/` | yes | unverified | unverified | Claude | docs |
| Grok Build | reads `.claude-plugin/` | yes | ignored by Grok | yes | camelCase and Claude | source |
| Kimi Code | `.kimi-plugin/` with hooks | yes | discarded by Kimi | yes | Claude, `path` | source |
| Muse Code | `.muse-plugin/` with hooks | yes | yes | if Muse runs `PostToolUse` | Claude | third-party plugins |
| DeepSeek Harness | its Claude hooks package reads `hooks/hooks.json` | no loader | yes | yes | Claude | source |
| ZCode | reads `.claude-plugin/` | yes | yes | yes | Claude | source |
| Qwen Code | converts the Claude plugin; `hooks/qwen.json` into settings | yes | yes | yes | Claude | docs |
| Copilot CLI | reads `.claude-plugin/` and `hooks/hooks.json` | yes | yes | yes | Claude (PascalCase config) | docs, npm package |
| Cursor CLI | `.cursor-plugin/`, `hooks/cursor.json` | yes | yes | yes, unverified in the CLI | Cursor | docs |
| Kiro CLI | `hooks/kiro.json` into `~/.kiro/hooks/` | by agent resources | yes, with the stage | yes | Claude, `fs_write` | docs, Q CLI source |
| Mistral Vibe | `plugins/vibe/` folder | yes | yes | yes | Vibe | source |
| MiniMax Code | the repository in `~/.minimax/plugins/` | yes | yes | yes | Claude | source |
| MiMo Code | the OpenCode plugin file | `.agents/skills` | in a tool result | yes | from the plugin | source |
| Qoder CLI | reads `.claude-plugin/` | yes | yes | yes | Claude | docs, npm package |
| Pi | root `package.json`, JS extension | yes | in a tool result | yes | from the plugin | Pi types |
| Hermes Agent | `.hermes-plugin/` | yes | before the next model call | yes | from the plugin | docs, source |
| Factory Droid | reads `.claude-plugin/` | yes | yes | yes | Claude | docs |
| CodeBuddy | reads `.claude-plugin/` | yes | yes | yes | Claude | source |
| Amp | single JS file, `amp skill add` | yes | in a tool result | yes | from the plugin | Amp types |
| Cline | `hooks/cline/*` into the hooks folder | rule file | unverified | yes, extension only | Cline | repo README |
| Kilo Code | `kilo plugin git:…` | yes | in a tool result | yes | from the plugin | source |
| Goose | `goose plugin install` reads `hooks/hooks.json` and `skills/` | yes | shown to the user only | yes | Goose | source |
| Oh My Pi | marketplace reads `.claude-plugin/`; `hooks/pre/` | yes | in a tool result | yes | from the plugin | source |
| Aider | `--read` the skill, `--no-auto-commits` | read-only file | no | no | none | docs |
| OpenHands | reads `.claude-plugin/` and `hooks/hooks.json` | yes | yes | yes | OpenHands | source |
| CodeArts Agent | the OpenCode plugin file | no loader found | in a tool result | yes | from the plugin | docs |
| Gemini CLI | `plugins/gemini/` extension | yes | yes | yes | Gemini | docs |

## Goals / Non-Goals

**Goals:**

- Each harness records edits wherever its hooks or plugin API allow, with one implementation in the binary.
- The Claude Code and Codex behavior stays as it is.

**Non-Goals:**

- Live runs in harnesses that are not installed here. The tests replay hook inputs in each harness's documented shape.
- A guard of shell commands outside Claude Code and Codex; that belongs to `shared-checkout-rules`.

## Decisions

### One reader of hook inputs in the binary

A new file, `internal/ms/hookinput.go`, turns the hook input of any harness into the `hookInput` that the hook uses now. It decodes the input as a map and takes each value from the first field name that holds it:

| Value | Field names |
| --- | --- |
| event | `hook_event_name`, `hookEventName`, `event_type`, `event`, `hookName` |
| tool | `tool_name`, `toolName`, `toolCall.name`, `<event>.toolName` (Cline) |
| tool input | `tool_input`, `toolInput`, `toolArgs` (a JSON string), `toolCall.args`, `tool_args`, `args`, `<event>.parameters` (Cline) |
| tool response | `tool_response`, `toolResponse`, `toolResult`, `tool_output` |
| session | `session_id`, `sessionId`, `conversationId`, `conversation_id`, `taskId` |
| subagent | `agent_id`, `agentId` |
| tool call | `tool_use_id`, `toolUseId`, `tool_call_id`, `toolCallId`, `toolCall.id` |
| directory | `cwd`, `working_dir`, `workspaceRoot`, the first of `workspace_roots`, `workspaceRoots`, `workspacePaths` |

Event names map to the four that the hook handles: `SessionStart` from `sessionStart`, `session_start`, `agentSpawn`, and `TaskStart`; `PreToolUse` from `preToolUse`, `BeforeTool`, and `pre_tool`; `PostToolUse` from `postToolUse`, `AfterTool`, and `post_tool`; `PostToolUseFailure` from `postToolUseFailure`. A post-tool input that reports a failure becomes `PostToolUseFailure`: a non-empty `error` (Antigravity), `tool_status` `failure` (Vibe), `success` false (Cline). The input keeps the harness's own event name for the output.

Alternative: one adapter script for each harness, which rewrites the input into Claude Code's shape. Each would need a runtime on every system, and the shell scripts would not run on Windows.

### Files of a call

The hook finds the edited files in this order:

1. A patch: any string of the tool input, or the tool input itself when it is a string, that holds `*** Begin Patch`. This is the `apply_patch` format of Codex, OpenCode (`patchText`), Oh My Pi (`input`), Amp (`patchText`), Hermes (`patch`), and Copilot (the raw input). The headers give the files, as they do for Codex now.
2. A file tool: a tool whose name, in lower case and after the last `.`, is one of `edit`, `multiedit`, `multi_edit`, `write`, `create`, `applypatch`, `apply_patch`, `patch`, `str_replace_editor`, `str_replace`, `search_replace`, `write_file`, `edit_file`, `create_file`, `write_to_file`, `replace_file_content`, `multi_replace_file_content`, `replace`, `replace_in_file`, `file_editor`, `fs_write`, `fs_append`. The file is the first of `file_path`, `filePath`, `path`, `TargetFile`, `target_file`, `targetFile`. Vibe's `file_system.edit` gives `edit`.

The patch replaces the checks for the tool name `apply_patch` in the snapshot, the move, and the own change.

### The change that a record holds

The Claude Code rule, the original content of the response with the call's replacements, stays for the editor `claude` only. A `Write` of Kimi Code with `mode: append` holds only the appended text, so the rule would write a wrong record in another harness. A patch applies the same way in every harness, and the current code falls back when it does not apply, so the patch rule holds for every editor. Every other call records the file before the call and the file after it, as Claude Code and Codex calls do now when their own rule cannot apply. Another writer's change to the same file during the call then enters the record; the window is the length of one tool call.

### Calls without a tool call ID

OpenHands, Gemini CLI, Kiro, and Cline send no tool call ID. The hook then keys the pending record of a call by a hash of the session, the tool name, and the tool input, which the pre-tool and post-tool inputs repeat. Go's JSON encoder sorts map keys, so the encoding of the decoded tool input is stable.

### Editor without a name

`hooks/hooks.json` serves many harnesses with one command, `git multistage hook`. The hook picks the editor in this order, so that every event of one session gets the same editor:

1. The input's shape: `toolCall` or `conversationId` gives `antigravity`, `cursor_version` `cursor`, `clineVersion` `cline`, `event_type` `openhands`, `event` without `hook_event_name` `goose`, the event names `pre_tool`, `post_tool`, and `session_start` give `vibe`, and `BeforeTool` and `AfterTool` give `gemini`.
2. A variable that the harness sets for its hook commands: `GROK_HOOK_EVENT` (grok), `COPILOT_CLI` (copilot), `MINIMAX_PLUGIN_ROOT` (minimax), `ZCODE_PLUGIN_ROOT` (zcode), `QODER_PLUGIN_ROOT` (qoder), `CODEBUDDY_PLUGIN_ROOT` (codebuddy), `DEVIN_PROJECT_DIR` (devin), `FACTORY_PROJECT_DIR` (droid), `GEMINI_SESSION_ID` (gemini).
3. `codex` for an `apply_patch` call with the patch in `tool_input.command`, as now.
4. `claude`.

Several of these harnesses also set `CLAUDE_PLUGIN_ROOT`, so the harness's own variable must come before any Claude Code variable. A harness that the hook does not name gets `claude`, which still keeps its stages apart, because the stage holds the session ID.

The rule for `apply_patch` now needs the patch in `command`, which only Codex uses. OpenCode, Oh My Pi, Amp, and Copilot have `apply_patch` and other file tools, and the old rule would put one session's edits into two stages.

### Output format of each harness

| Editor | Session start notice | First-edit message |
| --- | --- | --- |
| copilot, openhands | `{"additionalContext": …}` | the same |
| cursor | `{"additional_context": …}` | the same |
| antigravity | `{"injectSteps": [{"systemMessage": {"systemMessage": …}}]}` | nothing; Antigravity reads `{}` after a tool call |
| vibe | `{"hook_specific_output": {"additional_context": …}}` | the same |
| kiro | plain text | plain text |
| cline | `{"contextModification": …}` | the same |
| any other | `{"hookSpecificOutput": {"hookEventName": <input's event>, "additionalContext": …}}` | the same |

Copilot reads `additionalContext` at the top level when it runs the Claude Code hooks file; Superpowers' session start hook relies on the same `COPILOT_CLI` test. OpenHands reads `additionalContext` at the top level (`hooks_executor.py`). Claude Code reads two fields, so the hook never writes both.

### Stage in the session start notice

Antigravity reads no output after a tool call, so its first-edit message cannot reach the model. Kiro adds a session start hook's output to the context and documents nothing after a tool call. For these two, the session start notice also names the stage and the commands. The stage is the one that the session's edits go to: `--stage`, `GIT_MULTISTAGE_ID`, or `<editor>-<session ID>`. The notice of other harnesses stays as it is: Gemini CLI, for one, may run the notice without the edit hooks, and the stage would then hold nothing.

Grok ignores a session start hook's output, Kimi discards it, and Goose shows it to the user only. Their first-edit message uses the default format, and whether it reaches the model is not known. An agent that sees no message stages by path, as the skill says.

### One package for the Claude Code format

The harnesses in the "Claude Code plugin" rows read `.claude-plugin/`, `hooks/hooks.json`, and `skills/`. The repository adds no root `plugin.json` and no root `gemini-extension.json`: with either, `agy plugin install` takes the repository as a native or Gemini extension, keeps `hooks/hooks.json` in Claude Code's form, and skips it, because its `SessionStart` entry has no `command` at the top. Both results were checked with `agy plugin install` into a scratch home. A root `hooks.json` breaks the install too: `agy` merges the converted hooks into it, and then skips the whole file for the Claude Code entries. Devin documents plugin hooks in a root `hooks.json`, so Devin's loading of `hooks/hooks.json` from a Claude Code plugin stays unverified.

`hooks/hooks.json` keeps its events. Its matcher grows to the file tools of the harnesses that read it: `Edit|MultiEdit|Write|Create|ApplyPatch|apply_patch|edit|write|create|str_replace_editor|search_replace|write_file|edit_file|write_to_file|replace_file_content|multi_replace_file_content|file_editor`. Claude Code reads a matcher of names and `|` as a list of exact names. `PostToolUseFailure` stays out of the file: OpenHands' event enum lacks it, and an unknown event can make a harness reject the whole file.

### Harness files

- **Cursor**: `cursor-agent plugin marketplace add` reads `.cursor-plugin/marketplace.json`, a copy of Claude Code's marketplace entry; no source settled which marketplace file Cursor reads. `.cursor-plugin/plugin.json` points to `./skills/` and `./hooks/cursor.json`, which uses Cursor's `version: 1` form with `sessionStart`, and `preToolUse`, `postToolUse`, `postToolUseFailure` on the `Write` tool.
- **Kimi Code**: `.kimi-plugin/plugin.json` lists the skill and four hooks with `matcher` `^(Write|Edit)$`.
- **Muse Code**: `.muse-plugin/plugin.json` lists the skill and hooks for `SessionStart`, `PreToolUse`, and `PostToolUse` as `["git", "multistage", "hook", "muse"]`. No source shows a matcher, so the hook filters the tools.
- **Gemini CLI**: `plugins/gemini/` holds `gemini-extension.json`, `hooks/hooks.json` with `SessionStart`, `BeforeTool`, and `AfterTool` on `write_file|replace`, and the skill. A separate folder keeps `BeforeTool` out of `hooks/hooks.json`, where `claude plugin validate --strict` reports it as an unknown event.
- **Mistral Vibe**: `plugins/vibe/` holds `plugin.json` with the Agent Plugins schema and `extensions."ai.mistral.vibe"`, `ai.mistral.vibe/hooks.toml` with `session_start`, `pre_tool`, and `post_tool` matching `re:^(edit|write_file)$`, and the skill. Vibe skips the hooks of a Claude Code plugin.
- **Qwen Code, Kiro CLI, Cline**: hook files that the user copies, because Qwen reads hooks only from settings, Kiro from `~/.kiro/hooks/`, and Cline from its hooks folder. Cline's files are shell scripts named after the events, which Cline runs on macOS and Linux only.

The skill copies in `plugins/gemini/` and `plugins/vibe/` exist because both harnesses copy the plugin folder at install. A test compares them with `skills/git-multistage/SKILL.md`.

### JavaScript and Python plugins

Each plugin turns the host's tool events into a hook input in Claude Code's shape, `{hook_event_name, session_id, agent_id, tool_name, tool_input, tool_use_id, cwd}`, with the host's tool name and arguments, and runs `git multistage hook <editor>` with `node:child_process` (`subprocess` in Hermes). It reads `hookSpecificOutput.additionalContext` from the output. At the first tool call of a session it also sends `SessionStart`, and it adds the notice to that call's result. Errors are written to stderr and never block the tool.

| Plugin | Events | Message channel | Editor |
| --- | --- | --- | --- |
| `plugins/opencode/git-multistage.js` | `tool.execute.before`, `tool.execute.after`; `config` adds the skills folder | appended to `output.output` | `kilo`, `mimo`, or `codearts` when the host binary has that name, else `opencode` |
| `plugins/pi/git-multistage.js` | `tool_call`, `tool_result` | a text part added to `content` | `pi`; `hooks/pre/git-multistage.js` passes `omp` |
| `plugins/amp/git-multistage.js` | `tool.call`, `tool.result` | appended to `output` | `amp` |
| `.hermes-plugin/__init__.py` | `pre_tool_call`, `post_tool_call`, `pre_llm_call` | the `context` of the next `pre_llm_call` | `hermes` |

The OpenCode and Amp files are single files without imports of their own, because Kilo, MiMo, CodeArts, and Amp install one file. The root `package.json` names the OpenCode plugin as `main` and the Pi extension and skills under `pi`.

### Version

`TestPluginVersion` reads every manifest: `.claude-plugin/plugin.json` and its marketplace entry, `.codex-plugin/plugin.json`, `.cursor-plugin/plugin.json` and its marketplace entry, `.kimi-plugin/plugin.json`, `.muse-plugin/plugin.json` and its marketplace entry, `.hermes-plugin/plugin.yaml`, `package.json`, `plugins/vibe/plugin.json`, and `plugins/gemini/gemini-extension.json`.

## Risks / Trade-offs

- [A harness changes its hook input or file tools] → The table of field names and tool names is in one file, and a replay test holds each harness's documented input.
- [Facts that no source settled] → Devin's loading of `hooks/hooks.json`, Muse's `PostToolUse`, the argument names of Cursor's `Write`, Copilot's and Devin's file tools, and whether Grok, Kimi, and Goose show a first-edit message to the model. The hook reads every known path field, so an unknown argument name only loses the record of that call. The README marks these harnesses' edit recording as untested.
- [A variable inherited from a parent harness] → A Claude Code session started inside Copilot's shell sees `COPILOT_CLI` and gets Copilot's output format. The stage stays the same for the whole session.
- [Snapshot records in other harnesses] → Another writer's change to the same file during one tool call enters the record. The Claude Code and Codex rules are unchanged.

## Migration Plan

Installed Claude Code and Codex plugins keep working with the new binary, and update with the new version. Users of the other harnesses install as the README says.
