# Proposal

## Why

`add` infers a deletion when a file is missing on disk, and a rename when it gets both paths. On a shared working tree, a file can be missing because another agent deleted or moved it, so `add` can stage another agent's work. Commits from several stages also land out of order. When one stage commits a move or a deletion, another stage's later commit of that file has no reliable way to follow it. Git's rename detection misses a move that came with heavy edits. A deletion fails as a generic conflict that names neither the deleting commit nor a way out.

This change follows `stage-holds-edits`, which keeps every recorded edit in its stage whatever happens on disk.

## What Changes

- New `rm <path>...` deletes the files from the working tree and records the deletions in the stage. It also accepts a file that is already gone from disk.
- New `mv <old> <new>` moves the file in the working tree and records the move in the stage. It also accepts a move that was already made on disk. Pending recorded edits of `<old>` follow the move.
- **BREAKING** `add <path>` no longer stages a deletion, and `add <old> <new>` no longer stages a rename. `add` of a missing file fails with a hint to use `rm`.
- A commit that contains staged moves gets one `Multistage-Moved: <old> -> <new>` trailer for each move.
- When a stage's change is replayed onto a newer tip, at `commit` and at the refresh before other commands, the replay follows the moves that `Multistage-Moved` trailers name in the commits since the stage's base. For other commits it uses Git's rename detection.
- A staged change, or a recorded edit, of a file that a commit since the base deleted fails with a new error kind, `path-deleted`. The error names the deleting commit and keeps the stage. `commit --recreate <path>` and `add --edits --recreate <path>` put the file back with the stage's version.
- Codex `apply_patch` `Delete File` and `Move to` are recorded as a deletion and a move, which `add --edits` stages like `rm` and `mv`.
- **BREAKING** git-multistage now writes the working tree, but only in `rm` and `mv`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `staging`: `add` stages no deletions and no renames; new `rm` and `mv` requirements.
- `commit`: the `Multistage-Moved` trailer; following moves when replaying onto the tip; `path-deleted` and `--recreate`; `--recreate` joins the accepted options.
- `stages`: the refresh onto the branch tip follows moves and reports `path-deleted`.
- `editor-hooks`: `add --edits` follows moves, reports `path-deleted`, and takes `--recreate`; Codex deletions and moves are recorded as operations.
- `cli`: `rm` and `mv` are the only commands that write the working tree; both support `--json`.

## Impact

- `internal/ms/cli.go`: the `rm` and `mv` commands, and `--recreate` on `commit` and `add --edits`.
- `internal/ms/stage.go`: `AddPaths` refuses deletions and renames.
- `internal/ms/stagearea.go`: the stage metadata keeps the staged moves.
- `internal/ms/integrate.go` and `internal/ms/commit.go`: trailers, path rewriting before the merge, and `path-deleted`.
- `internal/ms/edits.go` and `internal/ms/hook.go`: move and deletion records, and following moves in `add --edits`.
- `skills/git-multistage/SKILL.md`: agents delete and move files with `rm` and `mv`, and handle `path-deleted`.
