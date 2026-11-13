# Spec Delta

## Purpose

Crash recovery makes sure that a killed or interrupted command never duplicates a commit, loses staged changes, or leaves a lock held.

## ADDED Requirements

### Requirement: Interrupted commit lands exactly once
A command that opens a stage (stage commands, `drop`, and the editor hook once it records an edit; not `list`, `doctor`, `sync`, or `log`) with an unfinished commit SHALL check whether the branch contains it. If so, the stage SHALL record it, clear its staged changes, and, unless `multistage.syncIndex` is `false`, sync the default index, warning after 3 s if `index.lock` stays held. If not, or if gc pruned it, it SHALL keep its staged changes. Either way it SHALL print a warning.

#### Scenario: Killed after the branch moved
- **WHEN** a commit was killed after the branch update and the user runs `status` on the stage
- **THEN** the stage records the commit once, shows nothing staged, and warns `finished an interrupted commit`

#### Scenario: Killed before the branch moved
- **WHEN** a commit was killed before the branch update
- **THEN** the next command keeps the staged changes, and a new `commit` creates one commit

#### Scenario: Command that does not open the stage
- **WHEN** a commit was killed after the branch update and the user runs `list`
- **THEN** `list` reports the pending commit, and the stage stays pending until a command opens it

#### Scenario: index.lock left by the killed commit
- **WHEN** the killed commit left `.git/index.lock` behind and the user runs `status` on the stage
- **THEN** the command records the commit, exits with code 0, and warns that the default index was not synchronized, naming `sync --from <parent>`

### Requirement: Killed staging command
A killed staging command SHALL leave either the old or the new index of the stage, never a mix. The next command on the stage SHALL delete leftover temporary files and index files that are not current. It SHALL delete a leftover lock file only when the file is older than one hour; when the current index has a lock file, the stage SHALL move to a copy of that index in a new index file.

#### Scenario: Killed during add
- **WHEN** `add` is killed while it writes the stage's index
- **THEN** the next command sees either the old or the new staged content

#### Scenario: Recent orphaned lock file
- **WHEN** a lock file younger than one hour sits beside the stage's current index
- **THEN** the next command keeps the lock file, moves the stage to a new index file, and stages and commits normally

### Requirement: Damaged stage
A stage whose current index file is missing SHALL be refused with exit code 1 and error kind `damaged-stage` by every command that opens the stage, except `drop`. `drop --force` SHALL delete the stage, `drop` without `--force` SHALL exit with code 5, and `drop --edits` SHALL run as usual. `log` SHALL still list the stage's commits.

#### Scenario: Missing index file
- **WHEN** a stage's index file was deleted and the user runs `commit`
- **THEN** the command exits with code 1, reports a damaged stage, and commits nothing

#### Scenario: Drop a damaged stage
- **WHEN** a stage's index file was deleted and the user runs `drop`
- **THEN** the command exits with code 5, and `drop --force` deletes the stage

### Requirement: Locks and timeouts
Each command that opens a stage SHALL hold an OS lock on it, so a dead process never leaves it held. When the stage lock stays held for 60 s, or `.git/index.lock` stays held for 15 s during `commit` or `sync`, the command SHALL exit with code 6 and commit nothing. While a command finishes an interrupted commit, a held `index.lock` SHALL only cause a warning after 3 s.

#### Scenario: Stage busy
- **WHEN** one process holds stage `a` and another command on stage `a` waits past the timeout
- **THEN** the second command exits with code 6

#### Scenario: Holder dies
- **WHEN** a process holding stage `a` is killed
- **THEN** the next command on stage `a` takes the lock at once

#### Scenario: index.lock held during commit
- **WHEN** `.git/index.lock` stays held for 15 s while stage `a` runs `commit` with sync on
- **THEN** `commit` exits with code 6, creates no commit, and stage `a` keeps its staged change

### Requirement: Staged objects survive garbage collection
After each command that opens a stage, and after the editor hook records an edit, `refs/worktree/multistage/<stage-id>` SHALL point to a commit, with the stage's base as parent, whose tree holds the staged tree and the blobs of recorded edits, so `git gc` keeps them. The ref SHALL be deleted when the stage has nothing staged and no recorded edits, and when `drop` deletes the stage. A damaged stage SHALL keep its ref.

#### Scenario: gc with staged changes
- **WHEN** stage `a` has staged a new file and the user runs `git gc --prune=now`
- **THEN** stage `a` can still commit the file

#### Scenario: Recorded edits only
- **WHEN** the editor hook recorded an edit for stage `a` and nothing is staged
- **THEN** `refs/worktree/multistage/a` exists and holds the edit's blobs

#### Scenario: Nothing left to protect
- **WHEN** stage `a` committed every staged change and has no recorded edits
- **THEN** `refs/worktree/multistage/a` does not exist
