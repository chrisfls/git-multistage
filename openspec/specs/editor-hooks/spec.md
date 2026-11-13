# editor-hooks Specification

## Purpose
Editor hooks record each file edit that a coding agent makes through its editor tools, so that the agent can stage exactly its own edits, even on lines next to another agent's edits.

## Requirements

### Requirement: Hook commands
`hook claude` SHALL handle Claude Code `PreToolUse` and `PostToolUse` events for `Edit`, `MultiEdit`, and `Write`. `hook codex` SHALL handle the same events for Codex `apply_patch` and SHALL take files from its `Update File`, `Add File`, `Delete File`, and `Move to` headers. `hook` without an editor SHALL pick `codex` for `apply_patch` and `claude` otherwise.

#### Scenario: Codex patch
- **WHEN** Codex runs `apply_patch` with `Update File: a.go` and `Add File: b.go`
- **THEN** the hook records edits for `a.go` and `b.go`

#### Scenario: Other tools
- **WHEN** the hook receives an event for a tool that edits no file, such as `Bash`
- **THEN** it records nothing and exits with code 0

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
On a `SessionStart` event whose `cwd` is in a Git worktree, the hook SHALL print `additionalContext` that says other agents may edit and commit in the checkout, that their changes are theirs to leave alone, and that the agent stages and commits with `git multistage`. Outside a Git worktree it SHALL print nothing. The first-edit message SHALL carry the same note about other agents. The hook SHALL exit with code 0.

#### Scenario: Session starts in a worktree
- **WHEN** the hook receives `SessionStart` with `cwd` in a Git worktree
- **THEN** it prints additional context about the other agents that names the `git-multistage` skill, and exits with code 0

#### Scenario: Session starts outside a worktree
- **WHEN** the hook receives `SessionStart` with `cwd` outside any Git worktree
- **THEN** it prints nothing and exits with code 0

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
For Claude Code `Edit`, `MultiEdit`, and `Write`, the hook SHALL record the change from the original content in the tool response to the content that the tool wrote, or from the file before the call when the response holds no original content. For Codex `apply_patch`, it SHALL record the change from the file before the call to that content with the patch applied.

#### Scenario: Claude Code edit
- **WHEN** a Claude Code `Edit` response holds original content `one two three` and a patch that changes `two` to `TWO`
- **THEN** the record changes `one two three` to `one TWO three`, whatever the file on disk holds after the call
