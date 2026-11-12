# Spec Delta

## Purpose

Editor hooks record each file edit that a coding agent makes through its editor tools, so that the agent can stage exactly its own edits, even on lines next to another agent's edits.

## ADDED Requirements

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

### Requirement: File leases
Before an edit, the hook SHALL take a lease of 15 seconds on each edited file, in a fixed order. A hook of another stage SHALL wait for the lease up to 20 seconds, then block its edit with exit code 2 and a message that asks the agent to retry.

#### Scenario: Two agents edit one file
- **WHEN** agent A holds the lease on `x.go` and agent B's edit of `x.go` waits more than 20 seconds
- **THEN** agent B's edit is blocked with exit code 2 and a retry message

### Requirement: Uncertain edits
When a lease expires and another stage takes the file before the edit is recorded, the edits of both stages SHALL be recorded as uncertain, and `add --edits` SHALL refuse them.

#### Scenario: Lease expires during a permission prompt
- **WHEN** agent A's lease expires during a long prompt and agent B then edits the file
- **THEN** `add --edits` refuses the uncertain records

### Requirement: Stage recorded edits
`add --edits [<path>...]` SHALL apply the recorded edits, oldest first, to the stage's index. Each edit SHALL be placed at its own lines, so other writers' changes stay out, even on neighbouring lines. If any record cannot apply, the command SHALL stage nothing. On success the records SHALL be deleted.

#### Scenario: Edits next to another agent's edits
- **WHEN** agent A edited line 5 and agent B edited line 6 of one file
- **THEN** agent A's `add --edits` stages only line 5

#### Scenario: Edit erased
- **WHEN** a later write removed the lines that a recorded edit added
- **THEN** `add --edits` fails with `edit-lost` or `edit-conflict` and stages nothing

### Requirement: Recorded edits and ignored or linked files
A recorded new file that `.gitignore` ignores SHALL need `add --edits -f`. An edit through a symbolic link SHALL be recorded for the link's target.

#### Scenario: Ignored new file
- **WHEN** an agent writes a new file that `.gitignore` ignores and runs `add --edits` without `-f`
- **THEN** that file is not staged

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
