# editor-hooks Specification

## MODIFIED Requirements

### Requirement: Hook input of each harness
The hook SHALL read the event, the tool name, the tool input, the session ID, the subagent ID, the tool call ID, and the working directory under the field names of each supported harness. It SHALL read each harness's event names as `SessionStart`, `PreToolUse`, `PostToolUse`, or `PostToolUseFailure`, and a post-tool event that reports a failed call as `PostToolUseFailure`. Without a tool call ID, it SHALL pair the events of one call by the tool name and the tool input.

#### Scenario: Antigravity edit
- **WHEN** the hook receives Antigravity `PreToolUse` and `PostToolUse` inputs with `conversationId` `c1` and `toolCall` `{"id": "t1", "name": "replace_file_content", "args": {"TargetFile": "<file>"}}`
- **THEN** the edit is recorded in stage `antigravity-c1`

#### Scenario: Event names of other harnesses
- **WHEN** the hook receives `BeforeTool` and `AfterTool` from Gemini CLI, `pre_tool` and `post_tool` from Mistral Vibe, or `preToolUse` and `postToolUse` from Cursor
- **THEN** it reads them as `PreToolUse` and `PostToolUse`

#### Scenario: Cline CLI input
- **WHEN** the hook receives Cline CLI inputs with `hookName` `tool_call` and `tool_result`, `taskId` `t1`, and the tool `editor` with `path` `<file>`
- **THEN** the edit is recorded in stage `cline-t1`

#### Scenario: Failed call in Mistral Vibe
- **WHEN** the hook receives a Vibe `post_tool` input with `tool_status` `failure`
- **THEN** it records nothing and deletes the pending record of the call

#### Scenario: Cline input
- **WHEN** the hook receives a Cline `PreToolUse` input with `taskId` `t1` and `preToolUse` `{"toolName": "write_to_file", "parameters": {"path": "<file>"}}`, and the matching `PostToolUse`
- **THEN** the edit is recorded in stage `cline-t1`

#### Scenario: Input without a tool call ID
- **WHEN** the hook receives OpenHands `PreToolUse` and `PostToolUse` inputs of one `file_editor` call, without a tool call ID
- **THEN** the edit is recorded once

### Requirement: Editor without a name
Without an editor name, the hook SHALL take the editor from, in order: the shape of the hook input; the variables that a harness sets for its hook commands; `codex` for an `apply_patch` call whose patch is in `command`; and `claude`.

#### Scenario: Antigravity input
- **WHEN** `git multistage hook` receives an input with `conversationId`
- **THEN** it uses the editor `antigravity`

#### Scenario: Harness variable
- **WHEN** `git multistage hook` runs with `MINIMAX_PLUGIN_ROOT` set and receives a Claude-shaped input
- **THEN** it uses the editor `minimax`

#### Scenario: Claude Code input
- **WHEN** `git multistage hook` receives a Claude Code input and no harness variable is set
- **THEN** it uses the editor `claude`

#### Scenario: Droid and Qwen Code
- **WHEN** `git multistage hook` receives a Claude-shaped input with `DROID_PLUGIN_ROOT` set, or with `QWEN_PROJECT_DIR` set
- **THEN** it uses the editor `droid`, or `qwen`
