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
When the staged change is already on the branch, `commit` SHALL create no commit, SHALL reset the stage to the tip so that nothing is staged, and SHALL exit with code 0. Text output SHALL say `nothing new to commit: <branch> already contains every staged change; the stage is now empty`, and `--json` output SHALL report `"result": "already-applied"`.

#### Scenario: Same change committed elsewhere
- **WHEN** another stage committed the same change that stage `a` staged, and the user runs `commit --json -m "a"` on stage `a`
- **THEN** the command exits with code 0, creates no commit, reports `"result": "already-applied"`, and stage `a` has nothing staged

### Requirement: Target branch and HEAD state
`commit` SHALL exit with code 5 and commit nothing when HEAD is off the target branch, the branch is gone, or a rebase, `git am`, or multi-commit cherry-pick or revert is in progress. During a merge, cherry-pick, or revert it SHALL also exit with code 5 when `multistage.syncIndex` is `false` or the default index holds neither the parent's nor the commit's version of a path the commit changes. A detached-HEAD stage SHALL keep HEAD detached on its base or a descendant.

#### Scenario: Branch switched
- **WHEN** stage `a` was created on `main` and HEAD is now on `feature`
- **THEN** `commit` exits with code 5

#### Scenario: Rebase in progress
- **WHEN** a rebase is in progress
- **THEN** `commit` exits with code 5

#### Scenario: Merge on other paths
- **WHEN** `multistage.syncIndex` is `true` and a merge in progress changed only paths that stage `a` did not stage
- **THEN** stage `a`'s commit lands

#### Scenario: Merge on the same paths
- **WHEN** a merge in progress left a conflict at a path that stage `a` staged
- **THEN** `commit` exits with code 5 and lists the path

#### Scenario: Merge with sync disabled
- **WHEN** `multistage.syncIndex` is `false` and a merge in progress changed only paths that stage `a` did not stage
- **THEN** `commit` exits with code 5 and creates no commit

### Requirement: Commit message
`commit` SHALL take the message from `-m` (repeatable, parts joined by a blank line) or `-F <file>` (`-F -` reads stdin); with both, `-F` SHALL win. Without either it SHALL exit with code 2 and SHALL NOT open an editor. It SHALL clean the message with `git stripspace` before and after the `commit-msg` hook, SHALL exit with code 1 when the result is empty, and SHALL NOT apply `commit.cleanup` or `commit.template`.

#### Scenario: No message
- **WHEN** the user runs `commit` without `-m` or `-F`
- **THEN** the command exits with code 2 and no commit is created

#### Scenario: Both -m and -F
- **WHEN** the user runs `commit -m "a" -F msg.txt`
- **THEN** the commit message is the content of `msg.txt`

### Requirement: Git hooks
`commit` SHALL run `prepare-commit-msg` and `commit-msg` on the message, then `pre-commit` on a temporary index holding the tip with the staged change applied, then `post-commit`, all through `git hook run`. `--no-verify` (`-n`) SHALL skip `pre-commit` and `commit-msg`. A failing hook other than `post-commit` SHALL exit with code 1. If `pre-commit` changes the tree of its index, `commit` SHALL exit with code 1 and error kind `hook-modified`, and the stage SHALL keep its changes.

#### Scenario: Hook re-stages a reformatted file
- **WHEN** a `pre-commit` hook reformats a staged file and runs `git add` on it
- **THEN** the commit exits with code 1 with error kind `hook-modified`, nothing is committed, and the stage keeps its staged change

#### Scenario: Hook rewrites only the working tree
- **WHEN** a `pre-commit` hook reformats a staged file without `git add`
- **THEN** the commit lands with the staged content, and the working-tree file keeps the hook's change

#### Scenario: Skip hooks
- **WHEN** the user runs `commit --no-verify -m "x"`
- **THEN** `pre-commit` and `commit-msg` do not run

### Requirement: Identity, signing, and reflog
Author and committer SHALL come from Git's configuration and the `GIT_AUTHOR_*` and `GIT_COMMITTER_*` variables. When `commit.gpgSign` is `true`, commits SHALL be signed with Git's signing configuration. The branch reflog entry SHALL read `commit (multistage <stage-id>): <subject>`, or `commit (multistage <stage-id>, index not synced): <subject>` when `multistage.syncIndex` is `false`.

#### Scenario: Signed commit
- **WHEN** `commit.gpgSign` is `true`
- **THEN** the stage's commit is signed

#### Scenario: Reflog entry
- **WHEN** stage `a` commits "Fix login" with `multistage.syncIndex` at its default
- **THEN** the branch reflog shows `commit (multistage a): Fix login`

#### Scenario: Reflog entry without sync
- **WHEN** `multistage.syncIndex` is `false` and stage `a` commits "Fix login"
- **THEN** the branch reflog shows `commit (multistage a, index not synced): Fix login`

### Requirement: Unsupported commit options
`commit` SHALL accept only `-m`/`--message`, `-F`/`--file`, `-n`/`--no-verify`, and the global options. Any other option, such as `--amend`, `-a`, `--author`, `--date`, or `--fixup`, SHALL exit with code 2. It SHALL never open an editor and SHALL ignore `GIT_INDEX_FILE` in its environment.

#### Scenario: Unsupported option
- **WHEN** the user runs `commit --amend -m "x"`
- **THEN** the command exits with code 2 and no commit is created

#### Scenario: GIT_INDEX_FILE set
- **WHEN** `GIT_INDEX_FILE` points to another index and the user runs `commit`
- **THEN** the commit uses the stage's own index
