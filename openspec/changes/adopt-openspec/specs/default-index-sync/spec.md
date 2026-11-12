# Spec Delta

## Purpose

Default-index sync keeps Git's ordinary index in step with the commits that stages make. This way `git status` and a person's `git commit` never undo those commits, and changes a person staged by hand are kept.

## ADDED Requirements

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
`sync` SHALL bring the default index up to the branch tip and keep entries staged by hand. It SHALL work out the commit that the index matches when it can tell. Otherwise it SHALL need `--from <commit>`.

#### Scenario: Index left behind by update-ref
- **WHEN** a plain `git update-ref` moved the branch and the index exactly matches the old commit
- **THEN** `sync` brings the index up to the tip without `--from`

#### Scenario: Unknown starting point
- **WHEN** the index matches no known commit
- **THEN** `sync` fails and asks for `--from <commit>`
