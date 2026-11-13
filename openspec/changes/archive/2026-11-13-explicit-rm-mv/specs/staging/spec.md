## ADDED Requirements

### Requirement: Delete files
`rm <path>...` SHALL delete each file from the working tree and stage its deletion in the stage's own index. A file that is already gone from disk SHALL have its deletion staged. A path that the stage's index does not hold SHALL exit with code 1 and change nothing. An unstaged change on disk SHALL need `-f`, and an unchanged symbolic link SHALL not; without it the command SHALL exit with code 5 and change nothing.

#### Scenario: Unchanged symbolic link
- **WHEN** `link` is a committed symbolic link that is unchanged on disk, and stage `a` runs `rm link`
- **THEN** `link` is gone from disk, and stage `a` holds its deletion

#### Scenario: Path the stage does not hold
- **WHEN** stage `a` ran `mv x.go y.go`, another agent then created a new `x.go`, and stage `a` runs `rm -f x.go`
- **THEN** the command exits with code 1, and the new `x.go` stays on disk

### Requirement: rm forgets recorded edits
`rm` SHALL forget the stage's recorded edits of the paths it deletes, including records that follow a move to those paths.

#### Scenario: Recorded edits of a deleted file
- **WHEN** stage `a` recorded an edit of `x.go`, runs `rm -f x.go`, then `add --edits`
- **THEN** `add --edits` stages the other recorded edits and does not report `x.go`

#### Scenario: Delete a file
- **WHEN** stage `a` runs `rm old.go`
- **THEN** `old.go` is gone from disk, stage `a` holds its deletion, and other stages are unchanged

#### Scenario: File already deleted on disk
- **WHEN** an agent deleted `old.go` with a shell command and stage `a` then runs `rm old.go`
- **THEN** stage `a` holds the deletion of `old.go`

#### Scenario: File with CRLF line endings
- **WHEN** `w.txt` was committed with CRLF line endings, `core.autocrlf` is `true`, and stage `a` runs `rm w.txt`
- **THEN** `w.txt` is gone from disk, and stage `a` holds its deletion

#### Scenario: Unstaged change
- **WHEN** `old.go` on disk differs from stage `a`'s index and stage `a` runs `rm old.go` without `-f`
- **THEN** the command exits with code 5, and the file and the stage are unchanged

### Requirement: Finish an interrupted rm or mv
When `rm` or `mv` stops after it changed the stage and before it finished the disk change, running the same command again SHALL finish the disk change of that command and change nothing else, also after other `rm` or `mv` commands ran in between. It SHALL NOT delete or overwrite a file that another writer created at one of its paths in between.

#### Scenario: Rerun in another case
- **WHEN** `core.ignoreCase` is set, the index holds `README.md`, `rm readme.md` stopped before it deleted the file, and stage `a` runs `rm readme.md` again
- **THEN** `README.md` is gone from disk

#### Scenario: Crash during a chained move
- **WHEN** stage `a` ran `mv x.go y.go`, then `mv y.go z.go` stopped before it moved the file on disk, and stage `a` runs `mv y.go z.go` again
- **THEN** `z.go` is on disk in place of `y.go`, and stage `a` holds the move of `x.go` to `z.go`

### Requirement: Rerun after the change is gone
When the stage no longer holds the change of an interrupted `rm` or `mv`, such as after `restore --staged`, running the command again SHALL run it as new.

#### Scenario: Rerun after a restore
- **WHEN** `mv x.go y.go` stopped before it moved the file on disk, stage `a` runs `restore --staged x.go y.go`, then `mv x.go y.go`
- **THEN** stage `a` holds the move, and `y.go` is on disk in place of `x.go`

### Requirement: Move files
`mv <old> <new>` SHALL move the file in the working tree and stage the move in the stage's own index, with the content that the stage holds for `<old>`. When `<old>` is already gone from disk and `<new>` exists, it SHALL stage the move without touching the disk. The stage SHALL remember the move until it is committed or either path is restored.

#### Scenario: Move a file
- **WHEN** stage `a` runs `mv x.go y.go`
- **THEN** `y.go` is on disk in place of `x.go`, and stage `a` holds the move

#### Scenario: Move already made on disk
- **WHEN** an agent moved `x.go` to `y.go` with a shell command and stage `a` then runs `mv x.go y.go`
- **THEN** stage `a` holds the move and the disk is unchanged

### Requirement: Moves of moved files
Recorded edits of `<old>` SHALL follow a move to `<new>`. A move of a path that the stage already moved SHALL extend that move, and a move back to the original path SHALL cancel it.

#### Scenario: Chained moves
- **WHEN** stage `a` runs `mv x.go y.go`, then `mv y.go z.go`, and commits
- **THEN** the commit carries the one trailer `Multistage-Moved: x.go -> z.go`

#### Scenario: Recorded edits follow
- **WHEN** stage `a` has recorded edits of `x.go`, runs `mv x.go y.go`, then `add --edits`
- **THEN** the edits are staged in `y.go`

### Requirement: Move refusals
`mv` SHALL never overwrite a file on disk. A `<new>` that the stage's index already holds, or a `<new>` that exists on disk while `<old>` exists too, SHALL exit with code 5 with error kind `path-exists` and change nothing. When neither path exists on disk, `mv` SHALL exit with code 1. When a file appears at `<new>` after the check, `mv` SHALL still change nothing. On a case-insensitive file system, a move between two spellings of one path SHALL rename the file.

#### Scenario: Target exists
- **WHEN** `x.go` and `y.go` both exist on disk and stage `a` runs `mv x.go y.go`
- **THEN** the command exits with code 5 with `path-exists`, and both files are unchanged

#### Scenario: Case-only rename
- **WHEN** `core.ignoreCase` is set and stage `a` runs `mv Readme.md README.md`
- **THEN** the file on disk is named `README.md`, and stage `a` holds the move

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
