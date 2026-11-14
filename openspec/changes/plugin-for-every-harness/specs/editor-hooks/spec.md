# editor-hooks Specification

## MODIFIED Requirements

### Requirement: Hook commands
`hook [<editor>]` SHALL take an editor name of lower-case letters, digits, and hyphens, and SHALL handle the `PreToolUse`, `PostToolUse`, and `PostToolUseFailure` events of the file tools of every supported harness. It SHALL take the files of a call from the path field of the tool input, or from the `Update File`, `Add File`, `Delete File`, and `Move to` headers of an `apply_patch` patch in any field of the tool input. Without an editor name, it SHALL pick one as the requirement "Editor without a name" says.

#### Scenario: Codex patch
- **WHEN** Codex runs `apply_patch` with `Update File: a.go` and `Add File: b.go`
- **THEN** the hook records edits for `a.go` and `b.go`

#### Scenario: Other tools
- **WHEN** the hook receives an event for a tool that edits no file, such as `Bash`
- **THEN** it records nothing and exits with code 0

#### Scenario: Patch in another harness
- **WHEN** `hook opencode` receives an `apply_patch` call whose `patchText` holds `Update File: a.go`
- **THEN** the hook records an edit for `a.go` in the stage `opencode-<session ID>`

#### Scenario: Path field of another harness
- **WHEN** the hook receives an Antigravity `write_to_file` call whose `TargetFile` names `a.go`
- **THEN** the hook records an edit for `a.go`

#### Scenario: Editor name with other characters
- **WHEN** the user runs `git multistage hook Claude.Code`
- **THEN** the command prints its usage and exits with code 2

### Requirement: Source of a recorded change
For Claude Code `Edit`, `MultiEdit`, and `Write`, the hook SHALL record the change from the original content in the tool response to the content that the tool wrote, or from the file before the call when the response holds no original content. For an `apply_patch` patch in any harness, it SHALL record the change from the file before the call to that content with the patch applied. For any other file tool, it SHALL record the change from the file before the call to the file after the call.

#### Scenario: Claude Code edit
- **WHEN** a Claude Code `Edit` response holds original content `one two three` and a patch that changes `two` to `TWO`
- **THEN** the record changes `one two three` to `one TWO three`, whatever the file on disk holds after the call

#### Scenario: Edit of another harness
- **WHEN** a Grok `search_replace` call changes line 2 of `f.txt`
- **THEN** the record changes `f.txt` from its content before the call to its content after the call

### Requirement: Codex deletions and moves
The hook SHALL record an `apply_patch` `Delete File` as a deletion and a `Move to` as a move, in Codex and in any other harness. `add --edits` SHALL stage them as `rm` and `mv` would, without touching the disk.

#### Scenario: Codex moves a file
- **WHEN** Codex runs `apply_patch` with `Update File: x.go` and `Move to: y.go`, and the agent runs `add --edits`
- **THEN** the stage holds the move of `x.go` to `y.go` with the patch's change, and a commit carries `Multistage-Moved: x.go -> y.go`

## ADDED Requirements

### Requirement: Hook input of each harness
The hook SHALL read the event, the tool name, the tool input, the session ID, the subagent ID, the tool call ID, and the working directory under the field names of each supported harness. It SHALL read each harness's event names as `SessionStart`, `PreToolUse`, `PostToolUse`, or `PostToolUseFailure`, and a post-tool event that reports a failed call as `PostToolUseFailure`. Without a tool call ID, it SHALL pair the events of one call by the tool name and the tool input.

#### Scenario: Antigravity edit
- **WHEN** the hook receives Antigravity `PreToolUse` and `PostToolUse` inputs with `conversationId` `c1` and `toolCall` `{"id": "t1", "name": "replace_file_content", "args": {"TargetFile": "<file>"}}`
- **THEN** the edit is recorded in stage `antigravity-c1`

#### Scenario: Event names of other harnesses
- **WHEN** the hook receives `BeforeTool` and `AfterTool` from Gemini CLI, `pre_tool` and `post_tool` from Mistral Vibe, or `preToolUse` and `postToolUse` from Cursor
- **THEN** it reads them as `PreToolUse` and `PostToolUse`

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

### Requirement: Output format of each harness
The hook SHALL write the session start notice and the first-edit message in the output format that the editor's harness reads. For a harness without a format of its own, it SHALL write `hookSpecificOutput.additionalContext` with the input's event name.

#### Scenario: Copilot and OpenHands
- **WHEN** a hook of GitHub Copilot CLI or OpenHands records the first edit of a stage
- **THEN** the hook prints `{"additionalContext": "<message>"}`

#### Scenario: Cursor
- **WHEN** `hook cursor` records the first edit of a stage
- **THEN** the hook prints `{"additional_context": "<message>"}`

#### Scenario: Antigravity
- **WHEN** the hook prints the session start notice for Antigravity
- **THEN** it prints `{"injectSteps": [{"systemMessage": {"systemMessage": "<notice>"}}]}`, and after a tool call it prints nothing

#### Scenario: Mistral Vibe
- **WHEN** `hook vibe` records the first edit of a stage
- **THEN** the hook prints `{"hook_specific_output": {"additional_context": "<message>"}}`

#### Scenario: Kiro CLI and Cline
- **WHEN** `hook kiro` or `hook cline` prints a message
- **THEN** Kiro gets the message as plain text, and Cline gets `{"contextModification": "<message>"}`

#### Scenario: Gemini CLI first edit
- **WHEN** `hook gemini` records the first edit of a stage on `AfterTool`
- **THEN** the hook prints `hookSpecificOutput` with `hookEventName` `AfterTool` and the message in `additionalContext`

### Requirement: Stage in the session start notice
In Antigravity and Kiro CLI, whose output after a tool call does not reach the model, the session start notice SHALL also name the stage that the session's edits go to and the commands to commit them.

#### Scenario: Antigravity session start
- **WHEN** the hook receives an Antigravity `SessionStart` input with `conversationId` `c1` in an opted-in checkout
- **THEN** the notice in `injectSteps` names the stage `antigravity-c1`

#### Scenario: Claude Code session start
- **WHEN** the hook receives a Claude Code `SessionStart` input in an opted-in checkout
- **THEN** the notice names no stage
