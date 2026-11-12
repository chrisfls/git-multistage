# Spec Delta

## Purpose

The commit turns one stage's staged changes into a normal Git commit on the newest tip of its branch. It carries those changes and nothing else, even when other stages commit at the same time.

## ADDED Requirements

### Requirement: Commit onto the current tip
`commit` SHALL create a commit whose parent is the target branch's current tip and whose tree is the staged change applied to that tip. If nothing is staged, it SHALL exit with code 1.

#### Scenario: Nothing staged
- **WHEN** stage `a` has nothing staged and the user runs `commit -m "x"`
- **THEN** the command exits with code 1 and no commit is created

#### Scenario: Tip moved since staging
- **WHEN** stage `b` committed after stage `a` staged a change to another file
- **THEN** stage `a`'s commit has stage `b`'s commit as its parent and keeps stage `b`'s change

### Requirement: Concurrent commits never lose a commit
Concurrent commits from several stages SHALL each land on the branch, and none SHALL be lost or applied twice. A commit that loses the race for the branch SHALL replay its staged change onto the new tip and retry.

#### Scenario: Two stages commit at once
- **WHEN** stages `a` and `b` run `commit` at the same time on different files
- **THEN** the branch holds both commits, one on top of the other

### Requirement: Commit carries only the staged change
The commit's tree SHALL differ from its parent only at the staged paths and at the new names of staged paths that the tip renamed. Each staged path that the tip did not change SHALL hold exactly the staged mode and content. A tree that fails this check SHALL be refused with exit code 3.

#### Scenario: Unstaged edits stay out
- **WHEN** the working tree has unstaged edits in files that stage `a` did not stage
- **THEN** stage `a`'s commit does not contain them

### Requirement: Conflicts keep the staged change
When the staged change cannot be replayed onto the tip, `commit` SHALL exit with code 3, list the conflicting paths, and keep the stage's staged changes.

#### Scenario: Same lines changed on the branch
- **WHEN** a newer commit on the branch changed the lines that stage `a` staged
- **THEN** the command exits with code 3 and stage `a` still holds its staged change

### Requirement: Line replay for adjacent changes
When a merge onto the tip reports only content conflicts, each conflicted file SHALL be replayed by placing each staged hunk at the tip's position of its lines. The replay SHALL succeed only when the tip left those lines unchanged and no insertion competes for the same point.

#### Scenario: Adjacent lines
- **WHEN** stage `a` staged a change to line 5 and the tip changed line 6
- **THEN** the commit succeeds and holds both changes

#### Scenario: Competing insertions
- **WHEN** stage `a` and the tip both insert lines at the same point
- **THEN** the commit exits with code 3

### Requirement: Already applied change
When the staged change is already on the branch, `commit` SHALL create no commit, SHALL clear the stage, and SHALL report `already-applied`.

#### Scenario: Same change committed elsewhere
- **WHEN** another stage committed the same change that stage `a` staged
- **THEN** stage `a`'s commit creates nothing and reports `already-applied`

### Requirement: Target branch and HEAD state
`commit` SHALL exit with code 5 when HEAD is not on the stage's target branch, when a rebase or `git am` is in progress, when a multi-commit cherry-pick or revert is in progress, or when a merge, cherry-pick, or revert in progress changed the same paths. A stage created on a detached HEAD SHALL commit to HEAD while HEAD stays detached on a descendant of its base.

#### Scenario: Branch switched
- **WHEN** stage `a` was created on `main` and HEAD is now on `feature`
- **THEN** `commit` exits with code 5

#### Scenario: Rebase in progress
- **WHEN** a rebase is in progress
- **THEN** `commit` exits with code 5

#### Scenario: Merge on other paths
- **WHEN** a merge in progress changed only paths that stage `a` did not stage
- **THEN** stage `a`'s commit lands

### Requirement: Commit message
`commit` SHALL take the message from `-m` (repeatable) or `-F <file>`, and SHALL fail without either. It SHALL clean the message with `git stripspace` and SHALL NOT apply `commit.cleanup` or `commit.template`.

#### Scenario: No message
- **WHEN** the user runs `commit` without `-m` or `-F`
- **THEN** the command fails and no commit is created

### Requirement: Git hooks
`commit` SHALL run `prepare-commit-msg`, `commit-msg`, `pre-commit`, and `post-commit` through Git's hook configuration. `--no-verify` SHALL skip `pre-commit` and `commit-msg`. If `pre-commit` changes the files it is given, the commit SHALL fail with `hook-modified` and the stage SHALL keep its changes.

#### Scenario: Hook rewrites a file
- **WHEN** a `pre-commit` hook reformats a staged file
- **THEN** the commit fails with `hook-modified` and nothing is committed

#### Scenario: Skip hooks
- **WHEN** the user runs `commit --no-verify -m "x"`
- **THEN** `pre-commit` and `commit-msg` do not run

### Requirement: Identity, signing, and reflog
Author and committer SHALL come from Git's configuration and the `GIT_AUTHOR_*` and `GIT_COMMITTER_*` variables. Commits SHALL be signed as `git commit` signs them. The reflog entry SHALL read `commit (multistage <stage-id>): <subject>`.

#### Scenario: Signed commit
- **WHEN** `commit.gpgSign` is `true`
- **THEN** the stage's commit is signed

#### Scenario: Reflog entry
- **WHEN** stage `a` commits "Fix login"
- **THEN** the branch reflog shows `commit (multistage a): Fix login`

### Requirement: Unsupported commit options
`commit` SHALL NOT support `--amend`, `-a`, `--author`, `--date`, `--fixup`, path arguments, or an editor. It SHALL ignore `GIT_INDEX_FILE` in its environment.

#### Scenario: GIT_INDEX_FILE set
- **WHEN** `GIT_INDEX_FILE` points to another index and the user runs `commit`
- **THEN** the commit uses the stage's own index
