# Spec Delta

## Purpose

Crash recovery makes sure that a killed or interrupted command never duplicates a commit, loses staged changes, or leaves a lock held.

## ADDED Requirements

### Requirement: Interrupted commit lands exactly once
At the start of every command, a stage with an unfinished commit SHALL check whether that commit is on the branch. If it is, the stage SHALL record it, clear its staged changes, and sync the default index. If it is not, or garbage collection pruned it, the stage SHALL keep its staged changes. Either way it SHALL print a warning.

#### Scenario: Killed after the branch moved
- **WHEN** a commit was killed after the branch update and the user runs any command on the stage
- **THEN** the stage records the commit once and shows nothing staged

#### Scenario: Killed before the branch moved
- **WHEN** a commit was killed before the branch update
- **THEN** the next command keeps the staged changes, and a new `commit` creates one commit

### Requirement: Killed staging command
A killed staging command SHALL leave either the old or the new index of the stage, never a mix. Leftover lock and temporary files SHALL be removed the next time the stage is used.

#### Scenario: Killed during add
- **WHEN** `add` is killed while it writes the stage's index
- **THEN** the next command sees either the old or the new staged content

### Requirement: Damaged stage
A stage whose index file is missing SHALL be refused by every command except `drop`.

#### Scenario: Missing index file
- **WHEN** a stage's index file was deleted and the user runs `commit`
- **THEN** the command reports a damaged stage and commits nothing

### Requirement: Locks and timeouts
Each stage SHALL be locked by an OS lock for one command, so a dead process never leaves it held. When the stage lock or `.git/index.lock` stays held until its timeout, the command SHALL exit with code 6.

#### Scenario: Stage busy
- **WHEN** one process holds stage `a` and another command on stage `a` waits past the timeout
- **THEN** the second command exits with code 6

#### Scenario: Holder dies
- **WHEN** a process holding stage `a` is killed
- **THEN** the next command on stage `a` takes the lock at once

### Requirement: Staged objects survive garbage collection
While a stage has staged changes or recorded edits, `refs/worktree/multistage/<stage-id>` SHALL point to a commit that holds them, so `git gc` keeps their objects. The ref SHALL be deleted when nothing is staged.

#### Scenario: gc with staged changes
- **WHEN** stage `a` has staged a new file and the user runs `git gc --prune=now`
- **THEN** stage `a` can still commit the file
