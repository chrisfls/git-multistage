## ADDED Requirements

### Requirement: Concurrent commits take turns
Commits of stages in one worktree SHALL take turns from the read of the branch tip to the update of the branch, so that a commit that waits for its turn lands without a replay. A commit SHALL give up its turn while its `pre-commit` hook runs. A killed commit SHALL never keep its turn. A commit that waits 10 s for its turn SHALL go on without it.

#### Scenario: Eight stages commit at once
- **WHEN** eight stages run `commit --json` at the same time on different files, and no other process moves the branch
- **THEN** the branch holds all eight commits, and each commit reports `attempts` 1

#### Scenario: Turn held too long
- **WHEN** another process holds the commit turn of the worktree for longer than 10 s while stage `a` runs `commit`
- **THEN** stage `a`'s commit goes on without the turn and lands

#### Scenario: Killed while holding the turn
- **WHEN** a commit of stage `a` is killed while it holds its turn, and stage `b` then runs `commit`
- **THEN** stage `b`'s commit takes the turn at once and lands

#### Scenario: Slow pre-commit hook
- **WHEN** stage `a`'s `pre-commit` hook is still running and stage `b` runs `commit`
- **THEN** stage `b`'s commit lands before stage `a`'s hook ends, and stage `a`'s commit then lands on top of it without running the hook again
