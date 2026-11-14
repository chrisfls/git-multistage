# editor-hooks Specification

## Purpose
Editor hooks record each file edit that a coding agent makes through its editor tools, so that the agent can stage exactly its own edits, even on lines next to another agent's edits.

## Requirements

### Requirement: Hook commands
`hook [<editor>]` SHALL take an editor name of lower-case letters, digits, and hyphens, and SHALL handle the `PreToolUse`, `PostToolUse`, and `PostToolUseFailure` events of each supported harness's file tools. It SHALL take a call's files from the path field of the tool input, or from the file headers of an `apply_patch` patch in any field of it. Without an editor name, it SHALL pick one as "Editor without a name" says.

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

### Requirement: Stage of a recorded edit
The hook SHALL record into the stage that `--stage` names. Otherwise a subagent SHALL record into `<editor>-<agent_id>`, and a main session SHALL record into `GIT_MULTISTAGE_ID` or `<editor>-<session_id>`. The first recorded edit of a stage SHALL tell the agent its stage ID and the commands to commit.

#### Scenario: Claude Code subagent
- **WHEN** a Claude Code subagent with `agent_id` `x1` edits a file
- **THEN** the edit is recorded in stage `claude-x1`

#### Scenario: First edit message
- **WHEN** a stage records its first edit
- **THEN** the hook's message to the model names the stage and the `add --edits` and `commit` commands

### Requirement: Stage recorded edits
`add --edits [<path>...]` SHALL apply the recorded edits, oldest first, to the stage's index, from the records alone and whatever the working tree now holds. Each edit SHALL be placed at its own lines, so other writers' changes stay out, even on neighbouring lines. If any record does not fit the stage's index, the command SHALL stage nothing and SHALL fail with error kind `edit-conflict`. On success the records SHALL be deleted.

#### Scenario: Edits next to another agent's edits
- **WHEN** agent A edited line 5 and agent B edited line 6 of one file
- **THEN** agent A's `add --edits` stages only line 5

#### Scenario: Edit erased
- **WHEN** agent A changed line 5 to `a`, agent B then changed line 5 to `b`, and A runs `add --edits`
- **THEN** A's stage holds `a` on line 5, and the working tree still holds `b`

#### Scenario: File deleted on disk
- **WHEN** agent A edited `x.go`, another writer then deleted `x.go`, and A runs `add --edits`
- **THEN** A's stage holds `x.go` with A's edit

#### Scenario: Edit of lines that another stage has not committed
- **WHEN** agent B added a function to `x.go` and has not committed it, agent A changed a line inside that function, and A runs `add --edits`
- **THEN** the command fails with `edit-conflict`, stages nothing, and keeps A's records

#### Scenario: Edit of lines that another stage then committed
- **WHEN** agent B added a function to `x.go`, agent A changed a line inside it, B commits, and A then runs `add --edits`
- **THEN** A's stage holds A's change of that line

### Requirement: Recorded edits and ignored or linked files
A recorded new file that `.gitignore` ignores SHALL need `add --edits -f`. An edit through a symbolic link SHALL be recorded for the link's target.

#### Scenario: Ignored new file
- **WHEN** an agent writes a new file that `.gitignore` ignores and runs `add --edits` without `-f`
- **THEN** the command fails with `edit-conflict` and stages none of the recorded edits

#### Scenario: Ignored new file with force
- **WHEN** the agent runs `add --edits -f`
- **THEN** the ignored file is staged

### Requirement: Forget recorded edits
`drop --edits [<path>...]` SHALL delete the recorded edits of the stage, or only those of the given paths.

#### Scenario: Forget one file
- **WHEN** the user runs `drop --edits a.go`
- **THEN** the records for `a.go` are gone and other records stay

### Requirement: Shell edits are not recorded
The hook SHALL record only the editor's own file tools. Files changed through shell commands SHALL be staged with `add` or `add --hunk`.

#### Scenario: Formatter
- **WHEN** an agent runs `gofmt -w` through a shell tool
- **THEN** no edit is recorded for the files that the formatter changed

### Requirement: Session start notice
On a `SessionStart` event in an opted-in checkout, the hook SHALL print `additionalContext` that says other agents may edit and commit in the checkout, that their changes are theirs to leave alone, that the agent edits files with its file-editing tools, which Git commands change shared state and must not be run, and that the agent stages and commits with `git multistage`. Elsewhere it SHALL print nothing. The first-edit message SHALL leave the notice out. The hook SHALL exit with code 0.

#### Scenario: Session starts in a worktree
- **WHEN** the hook receives `SessionStart` with `cwd` in an opted-in checkout
- **THEN** it prints additional context about the other agents that names the `git-multistage` skill, and exits with code 0

#### Scenario: Session starts outside a worktree
- **WHEN** the hook receives `SessionStart` with `cwd` outside any Git worktree
- **THEN** it prints nothing and exits with code 0

#### Scenario: Session starts in a checkout that did not opt in
- **WHEN** the hook receives `SessionStart` with `cwd` in a Git worktree whose config does not set `multistage.shared`
- **THEN** it prints nothing and exits with code 0

#### Scenario: First edit of a subagent
- **WHEN** a Claude Code subagent with `agent_id` `x1` makes the first recorded edit of stage `claude-x1`
- **THEN** the first-edit message names the stage and the commands, and leaves out the note about other agents, which the subagent start notice gave

#### Scenario: First edit of a main session
- **WHEN** a main session makes the first recorded edit of its stage
- **THEN** the first-edit message names the stage and the commands, and leaves out the note about other agents

### Requirement: Session start notice delivery
In Claude Code and in Codex, the model SHALL receive the session start notice before its first tool call.

#### Scenario: Claude Code session start
- **WHEN** a Claude Code session starts in a Git worktree with the plugin installed
- **THEN** the model can quote the shared-checkout notice

#### Scenario: Codex session start
- **WHEN** a Codex session starts in a Git worktree with the plugin installed and its hooks trusted
- **THEN** the model can quote the shared-checkout notice before its first tool call

### Requirement: Failed edits
On a `PostToolUseFailure` event, the hook SHALL delete the pending record of the call and SHALL record nothing.

#### Scenario: Edit fails
- **WHEN** an `Edit` fails after its `PreToolUse` and the editor sends `PostToolUseFailure`
- **THEN** no edit is recorded

### Requirement: Recorded change of a tool call
The hook SHALL record only the change that the tool call made, without locking the file. A change that another writer makes to the file during the call SHALL stay out of the record.

#### Scenario: Two agents edit one file at the same time
- **WHEN** agent A's `Edit` of line 2 and agent B's `Edit` of line 8 of one file run at the same time
- **THEN** neither edit waits, A's record holds only the line 2 change, and B's record holds only the line 8 change

#### Scenario: Another writer changes the file during a Codex patch
- **WHEN** another writer changes line 9 of `f.txt` between the PreToolUse and PostToolUse events of a Codex `apply_patch` that changes line 2
- **THEN** the record holds only the line 2 change

### Requirement: Source of a recorded change
For Claude Code `Edit`, `MultiEdit`, and `Write`, the hook SHALL record the change from the original content in the tool response to the content that the tool wrote, or from the file before the call when the response holds no original content. For an `apply_patch` patch in any harness, it SHALL record the change from the file before the call to that content with the patch applied. For any other file tool, it SHALL record the change from the file before the call to the file after the call.

#### Scenario: Claude Code edit
- **WHEN** a Claude Code `Edit` response holds original content `one two three` and a patch that changes `two` to `TWO`
- **THEN** the record changes `one two three` to `one TWO three`, whatever the file on disk holds after the call

#### Scenario: Edit of another harness
- **WHEN** a Grok `search_replace` call changes line 2 of `f.txt`
- **THEN** the record changes `f.txt` from its content before the call to its content after the call

### Requirement: Recorded edits follow moves and deletions
`add --edits` SHALL apply each recorded edit of a path that the stage's index moved, through `mv` or a refresh that followed a move, at the new path. A recorded edit of a path that a commit since the record deleted SHALL fail with error kind `path-deleted`, stage nothing, keep the records, and name the deleting commit. A recorded edit of a path that no commit removed and that the stage does not hold SHALL fail with `edit-conflict`.

#### Scenario: Several edits after another stage's move
- **WHEN** stage `a` recorded two edits of `x.go`, stage `b` committed `mv x.go y.go`, and stage `a` runs `add --edits`
- **THEN** both edits are staged in `y.go`

### Requirement: Commits that a recorded edit follows
For a recorded edit, `add --edits` SHALL follow the moves of the commits that the branch reached after the record was made, whatever their dates: their `Multistage-Moved` trailers, and otherwise Git's rename detection, as a commit does. For a record without the branch tip, written by an earlier version, it SHALL follow the commits dated after the record.

#### Scenario: Edit after a plain Git rename
- **WHEN** stage `a` recorded an edit of `x.go`, and a commit made with `git mv x.go y.go` and `git commit` landed on the branch
- **THEN** stage `a`'s `add --edits` stages the edit in `y.go`

#### Scenario: Move commit with an older date
- **WHEN** stage `a` recorded an edit of `x.go`, and a commit dated before the record that moves `x.go` to `y.go` then lands on the branch
- **THEN** stage `a`'s `add --edits` stages the edit in `y.go`

#### Scenario: Edit after another stage's move
- **WHEN** stage `a` recorded an edit of `x.go`, stage `b` committed `mv x.go y.go`, and stage `a` runs `add --edits`
- **THEN** the edit is staged in `y.go`

#### Scenario: Own move after a followed move
- **WHEN** stage `a` recorded an edit of `x.go`, stage `b` committed `mv x.go y.go`, stage `a` runs `mv y.go z.go`, then `add --edits`
- **THEN** the edit is staged in `z.go`

#### Scenario: Edit of another stage's new file
- **WHEN** stage `b` added `new.go` and has not committed it, stage `a` recorded an edit of `new.go`, and stage `a` runs `add --edits`
- **THEN** the command fails with `edit-conflict` and keeps the records

#### Scenario: Edit after another stage's deletion
- **WHEN** stage `a` recorded an edit of `x.go`, stage `b` committed `rm x.go`, and stage `a` runs `add --edits`
- **THEN** the command fails with `path-deleted`, names stage `b`'s commit, and keeps the records

### Requirement: Recreate a deleted file from its records
`add --edits --recreate <path>` SHALL stage the named path as a new file with the content that its newest record wrote, whatever the number of its records. `--recreate` SHALL be given once for each path. The `path-deleted` error SHALL hint at `--recreate <path>` and `drop --edits <path>`.

#### Scenario: Recreate from the records
- **WHEN** stage `b` committed `rm x.go` after stage `a` recorded an edit of `x.go`, and stage `a` runs `add --edits --recreate x.go`
- **THEN** stage `a` holds `x.go` as a new file with the content of its newest record

### Requirement: Codex deletions and moves
The hook SHALL record an `apply_patch` `Delete File` as a deletion and a `Move to` as a move, in Codex and in any other harness. `add --edits` SHALL stage them as `rm` and `mv` would, without touching the disk.

#### Scenario: Codex moves a file
- **WHEN** Codex runs `apply_patch` with `Update File: x.go` and `Move to: y.go`, and the agent runs `add --edits`
- **THEN** the stage holds the move of `x.go` to `y.go` with the patch's change, and a commit carries `Multistage-Moved: x.go -> y.go`

### Requirement: Shared checkout opt-in
The hooks SHALL act only in a checkout whose Git config sets `multistage.shared` to `true`. In any other checkout they SHALL record no edit, create no stage, print nothing, and allow every command.

#### Scenario: Edit in a checkout that did not opt in
- **WHEN** an agent edits a file with `Edit` in a Git worktree whose config does not set `multistage.shared`
- **THEN** the hook records nothing, creates no stage, and prints nothing

#### Scenario: Edit in an opted-in checkout
- **WHEN** an agent edits a file with `Edit` in a checkout whose config sets `multistage.shared` to `true`
- **THEN** the hook records the edit in the agent's stage

### Requirement: Subagent start notice
On a `SubagentStart` event in an opted-in checkout, the hook SHALL print the session start notice as `additionalContext` for the subagent, and exit with code 0.

#### Scenario: Subagent starts
- **WHEN** the hook receives `SubagentStart` with `cwd` in an opted-in checkout
- **THEN** it prints the notice with `hookEventName` `SubagentStart`

### Requirement: Shared-state Git commands
On a `PreToolUse` event of a shell tool in an opted-in checkout, the hook SHALL deny a command that runs `git add`, `git commit`, `git rm`, `git mv`, `git stash` other than `list` and `show`, `git restore`, `git reset`, `git checkout`, or `git switch`, with a reason that says what to do instead. It SHALL allow every other command, and every command that runs in another checkout.

#### Scenario: Stash
- **WHEN** an agent in an opted-in checkout runs `git stash` through `Bash`
- **THEN** the hook denies the command, and its reason says why and what to do instead

#### Scenario: Read-only Git commands
- **WHEN** an agent in an opted-in checkout runs `git status && git stash list && git multistage --stage a add --edits`
- **THEN** the hook allows the command

#### Scenario: Another checkout
- **WHEN** an agent in an opted-in checkout runs `git -C /tmp/scratch commit -m x`, and `/tmp/scratch` is a repository that did not opt in
- **THEN** the hook allows the command

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
