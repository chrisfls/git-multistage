# staging Specification

## Purpose
Staging puts chosen changes from the shared working tree into one stage's index, by whole file, hunk, or patch, and shows what a stage holds.

## Requirements

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

### Requirement: Refuse whole files that another stage staged
`add <path>` SHALL refuse with exit code 5, and stage nothing, when another stage of the worktree has staged changes in that file. `--allow-shared` SHALL override the check.

#### Scenario: File shared with another stage
- **WHEN** stage `b` has staged a hunk of `shared.ts` and stage `a` runs `add shared.ts`
- **THEN** the command exits with code 5 and stage `a` is unchanged

#### Scenario: Override
- **WHEN** stage `a` runs `add shared.ts --allow-shared`
- **THEN** the whole file is staged in stage `a`

### Requirement: List hunks with stable IDs
`hunks [--cached] [<path>...]` SHALL list zero-context hunks, each with a 12-hex-digit ID. Without paths it SHALL list the changed tracked files. The ID of a hunk that has no identical copy in its file SHALL derive from the path and the changed lines only: it SHALL stay the same while other hunks are staged or committed, and SHALL change when the hunk's content changes. In an unstaged listing, `staged-by` SHALL name other stages whose staged hunk has the same ID.

#### Scenario: IDs survive other staging
- **WHEN** a file has hunks `h1` and `h2` that make different changes, and `h1` is staged
- **THEN** a new `hunks` listing still shows `h2` with the same ID

#### Scenario: Hunk staged by another stage
- **WHEN** stage `b` has staged hunk `h1` and stage `a` runs `hunks`
- **THEN** `h1` is listed with `staged-by=b`

### Requirement: Identical hunks get distinct IDs
When several hunks of one file make the same change, the ID of each copy SHALL also hash the stage's base commit, a blob, and the copy's start line in that blob, so a selection never moves to another copy. An unstaged listing SHALL use the working-tree snapshot and the copy's line in it. A staged listing SHALL use the file's blob in the base and the copy's line in it. Staging or restoring one copy SHALL keep the IDs of the others; a new base SHALL make them unknown.

#### Scenario: Two identical edits
- **WHEN** a file has the same one-line change at line 3 and at line 30
- **THEN** `hunks` lists two different IDs

#### Scenario: Restore one staged copy
- **WHEN** a stage has staged three identical hunks of one file, the user lists them with `hunks --cached`, and restores the first with `restore --staged --hunk`
- **THEN** `restore --staged --hunk` with the listed ID of the second copy restores that copy

#### Scenario: Staged copy after a new base
- **WHEN** the user lists staged identical hunks with `hunks --cached`, and the stage then moves to a new base that changes the file
- **THEN** `restore --staged --hunk` with a listed copy ID exits with code 4 and changes nothing

### Requirement: Stage hunks by ID
`add <path>... --hunk <id>...` SHALL stage exactly the listed hunks, from one snapshot of the file. If any ID matches no hunk of the current diff, the command SHALL exit with code 4 and stage nothing. An untracked ignored file SHALL need `-f`.

#### Scenario: Stage one hunk
- **WHEN** `shared.ts` has hunks `d94c87b26e90` and `3c16c257727c` and the user runs `add shared.ts --hunk d94c87b26e90`
- **THEN** only the first hunk is staged

#### Scenario: Stale hunk ID
- **WHEN** the file changed after `hunks` ran and the user passes the old ID
- **THEN** the command exits with code 4 and nothing is staged

### Requirement: Files without hunks are staged whole
Files with a `filter` attribute, symbolic links, submodules, binary files, and deletions SHALL have no hunks and SHALL be staged whole.

#### Scenario: Binary file
- **WHEN** the user runs `hunks` on a changed binary file
- **THEN** the file is listed without hunks and can be staged only with `add <path>`

### Requirement: Interactive hunk selection
`add -p <path>...` SHALL let the user pick hunks interactively, as `git add -p` does. It SHALL be the only command that prompts for input.

#### Scenario: Pick a hunk
- **WHEN** the user runs `add -p shared.ts` and accepts the first hunk only
- **THEN** only that hunk is staged

### Requirement: Apply a patch to a stage
`apply [-R] <patch>|-` SHALL apply a patch to the stage's index, as `git apply --cached` does, and SHALL NOT touch the working tree.

#### Scenario: Patch from stdin
- **WHEN** the user pipes a patch into `git multistage --stage a apply -`
- **THEN** the patch's changes are staged in stage `a` and the files on disk are unchanged

### Requirement: Unstage
`restore --staged <path>...` SHALL unstage whole files. With `--hunk <id>...`, it SHALL unstage only the listed staged hunks and keep the rest.

#### Scenario: Unstage one hunk
- **WHEN** stage `a` has staged hunks `h1` and `h2` of a file and runs `restore --staged <path> --hunk h1`
- **THEN** only `h2` stays staged

### Requirement: Inspect a stage
`status` SHALL show the stage's staged, unstaged, and untracked files, its recorded edits, and which of its staged files other stages have also staged. `diff [--cached]` SHALL run `git diff` against the stage; `--cached` compares the staged changes with the base. `log` SHALL show the commits that the stage made.

#### Scenario: Another stage's file
- **WHEN** stage `fix-login` staged `src/login.ts` and stage `validation` runs `status`
- **THEN** `src/login.ts` is listed as not staged for `validation`

#### Scenario: Untracked files are shared
- **WHEN** the working tree holds an untracked file
- **THEN** the `status` of every stage lists it

#### Scenario: File staged in two stages
- **WHEN** stages `a` and `b` have both staged `shared.ts` and stage `a` runs `status`
- **THEN** `shared.ts` is listed as also staged in stage `b`

### Requirement: Staged-by for identical copies
In an unstaged listing, `staged-by` SHALL also name another stage that has staged a hunk with the same changed lines at the same start line of the old content, so identical copies that another stage staged are marked although their IDs differ.

#### Scenario: Copies staged by another stage
- **WHEN** a file has two identical hunks, stage `b` stages both, and stage `a` runs `hunks`
- **THEN** both copies are listed with `staged-by=b`

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
