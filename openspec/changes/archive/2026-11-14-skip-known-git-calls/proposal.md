# Proposal

## Why

After `parallel-git-reads`, a `commit` still starts 35 `git` processes, and each hooked edit starts 6 + 12 (one command in flight, macOS, a repository without Git hooks). Many of those calls ask Git for an answer that `git-multistage` already has or can see on disk. A `commit` starts `git hook run` four times when no hook exists. It also builds a temporary index for a `pre-commit` hook that does not exist, and it asks Git for the tree and the parent of the commit that it just wrote. Every command asks for the tree of the stage's base, which never changes for one base. The keep ref's tree takes two `mktree` calls when the stage has recorded edits, and that happens on every hooked edit.

A Go Git library is still rejected, for the reasons in the design of `fewer-git-processes`. The calls that this change removes need no library: the answers are already in the process, in the stage's metadata, or in the hooks directory.

## What Changes

- `commit` starts no `git hook run` for a hook that has no file in the hooks directory and no `hook.*` setting. With no `pre-commit` hook, it builds no temporary index for it, and with no `commit-msg` hook, it cleans the message once.
- A stage's `meta.json` records the tree of the stage's base, so later commands read it without `git rev-parse`.
- `commit` remembers the tree and the parent of each commit that it writes, so the index sync and the stage reset ask Git for neither.
- The keep ref's tree holds the staged tree and the recorded edits' blobs side by side, written with one `mktree`.
- Outputs, exit codes, and the commits, refs, and indexes that commands write stay the same. The keep ref's tree changes its layout. `meta.json` gains one field.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `commit`: a new requirement defines an absent Git hook, which `commit` skips without starting `git hook run`; the message requirement cleans the message after `commit-msg` only when that hook ran.

## Impact

- `internal/gitx/gitx.go`: a hook check, remembered trees and parents, and the config read state.
- `internal/ms/commit.go`: the hook runs, the message cleanup, and `commitTree`.
- `internal/ms/stagearea.go`: `Meta`, `loadMeta`, `save`, and `keepTree`.
- `internal/ms/stage.go`: the reads of the other stages' metadata.
- `commit`, `add`, `status`, `diff --cached`, and the PostToolUse hook start fewer `git` processes. The measurements before and after go in `tasks.md`.
