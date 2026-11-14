# commit Specification

## Purpose
The commit turns one stage's staged changes into a normal Git commit on the newest tip of its branch. It carries those changes and nothing else, even when other stages commit at the same time.

## Requirements

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
`commit` SHALL take the message from `-m` (repeatable, parts joined by a blank line) or `-F <file>` (`-F -` reads stdin); with both, `-F` SHALL win. Without either it SHALL exit with code 2 and SHALL NOT open an editor. It SHALL clean the message as `git stripspace` does before the `commit-msg` hook, and again after that hook when it ran, SHALL exit with code 1 when the result is empty, and SHALL NOT apply `commit.cleanup` or `commit.template`.

#### Scenario: No message
- **WHEN** the user runs `commit` without `-m` or `-F`
- **THEN** the command exits with code 2 and no commit is created

#### Scenario: Both -m and -F
- **WHEN** the user runs `commit -m "a" -F msg.txt`
- **THEN** the commit message is the content of `msg.txt`

#### Scenario: Message cleanup
- **WHEN** the user runs `commit -m "  \n\nsubject  \n\n\n\nbody\t\n\n"`
- **THEN** the commit message is `subject\n\nbody\n`, the message that `git stripspace` gives for that text

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

### Requirement: Absent Git hooks
`commit` SHALL treat a Git hook as absent when the hooks directory (`core.hooksPath`, or `hooks` in the common Git directory) holds no entry of its name and no `hook.*` setting exists. It SHALL skip an absent hook without starting `git hook run`, and SHALL run `git hook run` whenever it cannot tell.

#### Scenario: Hook in core.hooksPath
- **WHEN** `core.hooksPath` names a relative directory that holds a `pre-commit` hook
- **THEN** `commit` runs that hook

#### Scenario: Hook from a setting
- **WHEN** no hook file exists and `hook.lint.event` is `pre-commit` with `hook.lint.command` set
- **THEN** `commit` runs that command as its `pre-commit` hook

#### Scenario: No hooks
- **WHEN** the hooks directory holds only Git's sample hooks and no `hook.*` setting exists
- **THEN** `commit` starts no `git hook run` and lands the commit

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
`commit` SHALL accept only `-m`/`--message`, `-F`/`--file`, `-n`/`--no-verify`, `--recreate`, and the global options. Any other option, such as `--amend`, `-a`, `--author`, `--date`, or `--fixup`, SHALL exit with code 2. It SHALL never open an editor and SHALL ignore `GIT_INDEX_FILE` in its environment.

#### Scenario: Unsupported option
- **WHEN** the user runs `commit --amend -m "x"`
- **THEN** the command exits with code 2 and no commit is created

#### Scenario: GIT_INDEX_FILE set
- **WHEN** `GIT_INDEX_FILE` points to another index and the user runs `commit`
- **THEN** the commit uses the stage's own index

### Requirement: Move trailers
A commit whose staged change holds moves made with `mv` SHALL end its message with one `Multistage-Moved: <old> -> <new>` trailer for each move. A path that holds `->`, a double quote, a backslash, or a control character, or that starts or ends with whitespace, SHALL be written in double quotes with C-style escapes, as `git diff` quotes paths. The moves of one commit's trailers SHALL take effect at the same time. A move whose new path the commit adds with `--recreate` SHALL get no trailer.

#### Scenario: Recreated move
- **WHEN** stage `a` staged `mv x.go y.go`, stage `b` committed `rm x.go`, and stage `a` runs `commit --recreate y.go`
- **THEN** the commit carries no `Multistage-Moved` trailer for `x.go`, and another stage's later commit of `x.go` fails with `path-deleted`

#### Scenario: Path with an arrow
- **WHEN** stage `a` runs `mv "p -> q.go" r.go` and commits
- **THEN** the trailer is `Multistage-Moved: "p -> q.go" -> r.go`, and another stage's later commit of `p -> q.go` lands in `r.go`

#### Scenario: Path with surrounding spaces
- **WHEN** stage `a` runs `mv " p.go" "r.go "` and commits
- **THEN** another stage's later commit of ` p.go` lands in `r.go `

#### Scenario: Commit a move
- **WHEN** stage `a` ran `mv x.go y.go` and commits
- **THEN** the commit message ends with `Multistage-Moved: x.go -> y.go`

### Requirement: Follow moves onto the tip
When the staged change is replayed onto a newer tip, each staged path that the base holds and that a `Multistage-Moved` trailer of a commit since the stage's base moved SHALL be replayed at the new path, following chains of moves across commits. A path that no trailer names SHALL follow a rename when one commit since the stage's base renamed it, by Git's rename detection on that commit alone; otherwise it SHALL count as deleted. A path that the stage added SHALL stay where the stage put it.

#### Scenario: Several moves in one commit
- **WHEN** stage `b` committed the trailers `a.go -> c.go` and `c.go -> d.go`, and stage `a` then commits its staged change to `a.go`
- **THEN** stage `a`'s commit changes `c.go`

#### Scenario: Edit after another stage's move
- **WHEN** stage `b` committed `mv x.go y.go` with heavy edits of the file, and stage `a` then commits its staged change to `x.go`
- **THEN** stage `a`'s commit changes `y.go` and does not create `x.go`

### Requirement: Staged change of a deleted file
When a commit since the stage's base deleted a path that the staged change modifies or moves, `commit` SHALL exit with code 3 and error kind `path-deleted`, commit nothing, and keep the stage. The error SHALL name the path and the deleting commit, and hint at `--recreate <path>` and `restore --staged <path>`. `commit --recreate <path>...` SHALL commit the stage's version of each named path as a new file.

#### Scenario: Another stage deleted the file
- **WHEN** stage `b` committed `rm x.go` and stage `a` then commits its staged change to `x.go`
- **THEN** the command exits with code 3, names `x.go` and stage `b`'s commit, and stage `a` keeps its staged change

#### Scenario: Recreate the file
- **WHEN** stage `a` then runs `commit --recreate x.go -m "Keep x"`
- **THEN** the commit adds `x.go` with stage `a`'s version

### Requirement: Two stages move one file
When a commit since the stage's base moved a path to a new path, and the stage moved the same path to another new path, `commit` SHALL exit with code 3 and error kind `conflict`, commit nothing, and keep the stage.

#### Scenario: Two names for one file
- **WHEN** stage `b` committed `mv x.go z.go`, and stage `a` staged `mv x.go y.go` and commits
- **THEN** the command exits with code 3, names `x.go`, and stage `a` keeps its staged move
