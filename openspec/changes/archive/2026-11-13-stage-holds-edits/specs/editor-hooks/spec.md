## ADDED Requirements

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

## MODIFIED Requirements

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

### Requirement: Failed edits
On a `PostToolUseFailure` event, the hook SHALL delete the pending record of the call and SHALL record nothing.

#### Scenario: Edit fails
- **WHEN** an `Edit` fails after its `PreToolUse` and the editor sends `PostToolUseFailure`
- **THEN** no edit is recorded

## REMOVED Requirements

### Requirement: File leases
**Reason**: The record holds only the tool call's own change, so another writer's change cannot enter it, and the agents no longer need to take turns on a file.
**Migration**: None. Agents no longer wait for each other, and the hook never blocks an edit with exit code 2.

### Requirement: Uncertain edits
**Reason**: Without leases, no record is uncertain. Each record holds only its tool call's change.
**Migration**: Records written as uncertain by an earlier version are staged like any other record.

### Requirement: Deletion next to a later change
**Reason**: `add --edits` no longer reads the working tree, so a later change on disk never makes a deletion lost.
**Migration**: None. A recorded deletion is staged when it fits the stage's index.
