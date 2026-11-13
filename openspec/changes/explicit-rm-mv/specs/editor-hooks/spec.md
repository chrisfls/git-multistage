## ADDED Requirements

### Requirement: Recorded edits follow moves and deletions
`add --edits` SHALL apply a recorded edit of a path that the stage's index moved, through `mv` or a refresh that followed a move, at the new path. A recorded edit of a path that a commit since the record deleted SHALL fail with error kind `path-deleted`, stage nothing, keep the records, and name the deleting commit.

#### Scenario: Edit after another stage's move
- **WHEN** stage `a` recorded an edit of `x.go`, stage `b` committed `mv x.go y.go`, and stage `a` runs `add --edits`
- **THEN** the edit is staged in `y.go`

#### Scenario: Edit after another stage's deletion
- **WHEN** stage `a` recorded an edit of `x.go`, stage `b` committed `rm x.go`, and stage `a` runs `add --edits`
- **THEN** the command fails with `path-deleted`, names stage `b`'s commit, and keeps the records

### Requirement: Recreate a deleted file from its records
`add --edits --recreate <path>...` SHALL stage each named path as a new file with the content that its newest record wrote. The `path-deleted` error SHALL hint at `--recreate <path>` and `drop --edits <path>`.

#### Scenario: Recreate from the records
- **WHEN** stage `b` committed `rm x.go` after stage `a` recorded an edit of `x.go`, and stage `a` runs `add --edits --recreate x.go`
- **THEN** stage `a` holds `x.go` as a new file with the content of its newest record

### Requirement: Codex deletions and moves
The hook SHALL record a Codex `apply_patch` `Delete File` as a deletion and a `Move to` as a move. `add --edits` SHALL stage them as `rm` and `mv` would, without touching the disk.

#### Scenario: Codex moves a file
- **WHEN** Codex runs `apply_patch` with `Update File: x.go` and `Move to: y.go`, and the agent runs `add --edits`
- **THEN** the stage holds the move of `x.go` to `y.go` with the patch's change, and a commit carries `Multistage-Moved: x.go -> y.go`
