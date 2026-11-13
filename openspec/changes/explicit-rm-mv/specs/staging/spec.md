## ADDED Requirements

### Requirement: Delete files
`rm <path>...` SHALL delete each file from the working tree and stage its deletion in the stage's own index. A file that is already gone from disk SHALL have its deletion staged. A path that the stage's index does not hold SHALL exit with code 1 and change nothing. An unstaged change on disk SHALL need `-f`; without it the command SHALL exit with code 5 and change nothing.

#### Scenario: Delete a file
- **WHEN** stage `a` runs `rm old.go`
- **THEN** `old.go` is gone from disk, stage `a` holds its deletion, and other stages are unchanged

#### Scenario: File already deleted on disk
- **WHEN** an agent deleted `old.go` with a shell command and stage `a` then runs `rm old.go`
- **THEN** stage `a` holds the deletion of `old.go`

#### Scenario: Unstaged change
- **WHEN** `old.go` on disk differs from stage `a`'s index and stage `a` runs `rm old.go` without `-f`
- **THEN** the command exits with code 5, and the file and the stage are unchanged

### Requirement: Move files
`mv <old> <new>` SHALL move the file in the working tree and stage the move in the stage's own index, with the content that the stage holds for `<old>`. When `<old>` is already gone from disk and `<new>` exists, it SHALL stage the move without touching the disk. The stage SHALL remember the move until it is committed or either path is restored. Recorded edits of `<old>` SHALL follow the move to `<new>`. A `<new>` that the stage's index already holds SHALL exit with code 5 and change nothing.

#### Scenario: Move a file
- **WHEN** stage `a` runs `mv x.go y.go`
- **THEN** `y.go` is on disk in place of `x.go`, and stage `a` holds the move

#### Scenario: Move already made on disk
- **WHEN** an agent moved `x.go` to `y.go` with a shell command and stage `a` then runs `mv x.go y.go`
- **THEN** stage `a` holds the move and the disk is unchanged

#### Scenario: Recorded edits follow
- **WHEN** stage `a` has recorded edits of `x.go`, runs `mv x.go y.go`, then `add --edits`
- **THEN** the edits are staged in `y.go`

## MODIFIED Requirements

### Requirement: Stage whole files
`add <path>...` SHALL stage whole files as `git add` does, with ignore rules, attributes, filters, and binary files, but SHALL take each path literally, without pathspec globs or magic. An ignored file SHALL need `-f`; without it the command SHALL exit with code 1 and stage nothing. `add` SHALL NOT stage a deletion or a rename: a path that is missing from disk SHALL exit with code 1, stage nothing, and hint at `rm` and `mv`. Only the stage's own index SHALL change.

#### Scenario: Stage a modified file
- **WHEN** the user runs `git multistage --stage a add src/login.ts`
- **THEN** `src/login.ts` is staged in stage `a`, and the default index and other stages are unchanged

#### Scenario: Ignored file
- **WHEN** the user runs `add` on a file that `.gitignore` ignores, without `-f`
- **THEN** the command exits with code 1 and the file is not staged

#### Scenario: Glob is literal
- **WHEN** the user runs `git multistage --stage a add '*.ts'` and no file is named `*.ts`
- **THEN** the command exits with code 1 and stages nothing

#### Scenario: Missing file
- **WHEN** another agent deleted `old.go` and stage `a` runs `add old.go`
- **THEN** the command exits with code 1, stages nothing, and hints at `rm` and `mv`
