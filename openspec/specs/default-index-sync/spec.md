# default-index-sync Specification

## Purpose
Default-index sync keeps Git's ordinary index in step with the commits that stages make. This way `git status` and a person's `git commit` never undo those commits, and changes a person staged by hand are kept.

## Requirements

### Requirement: Sync after each stage commit
After each stage commit, every default-index entry at a path that the commit changed SHALL move from the parent's version to the commit's version. This SHALL happen while the tool holds `.git/index.lock`, and SHALL never touch the working tree.

#### Scenario: Clean status after a commit
- **WHEN** stage `a` commits a change to `x.go` and the working tree holds no other edits
- **THEN** `git status --short` prints nothing

### Requirement: Keep entries staged by hand
A default-index entry that equals neither the parent's nor the commit's version SHALL be kept, and the command SHALL print a warning. This includes conflict stages.

#### Scenario: Native staging
- **WHEN** a person ran `git add x.go` with other content, and stage `a` then commits a change to `x.go`
- **THEN** the default index keeps the person's entry and the commit prints a warning

### Requirement: Sync setting
`multistage.syncIndex=false` SHALL make stage commits leave the default index unchanged, and their reflog entries SHALL say `index not synced`. The default SHALL be `true`.

#### Scenario: Sync disabled
- **WHEN** `multistage.syncIndex` is `false` and stage `a` commits
- **THEN** the default index is unchanged and the reflog entry says `index not synced`

### Requirement: Sync command
`sync` SHALL move the default index from a start commit to the branch tip and keep entries that match neither version. Without `--from`, the start SHALL be the commit of the last sync if the index is unchanged since, else the newest of the last 200 first-parent commits whose tree equals the index, and only if each branch update since left the index alone (`update-ref` without `-m`, or a stage commit with sync off). Otherwise `sync` SHALL exit with code 1 and hint at `--from`.

#### Scenario: Index left behind by update-ref
- **WHEN** `git update-ref` without `-m` moved the branch and the index exactly matches the old commit
- **THEN** `sync` brings the index up to the tip without `--from`

#### Scenario: Entry staged by hand since the last sync
- **WHEN** a person ran `git add` after the last sync, so the index matches no commit
- **THEN** `sync` exits with code 1 and its hint names `--from <commit>`

#### Scenario: Branch moved by a logged update
- **WHEN** the index exactly matches an older commit, and an update with a reflog message moved the branch since
- **THEN** `sync` exits with code 1 and its hint names `--from <commit>`

#### Scenario: Explicit start commit
- **WHEN** a person ran `git add x.go` and the user runs `sync --from <commit>` with the commit that the rest of the index matches
- **THEN** `sync` exits with code 0, brings the other entries to the tip, and keeps the entry of `x.go`

#### Scenario: Start commit outside the history
- **WHEN** `sync --from <commit>` names a commit that is not an ancestor of the tip
- **THEN** `sync` exits with code 0, leaves the index unchanged, and prints a warning
