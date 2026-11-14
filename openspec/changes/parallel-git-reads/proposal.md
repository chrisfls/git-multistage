# Proposal

## Why

After `cr-free-snapshots`, most `git` processes that remain in a command are needed, but many of them wait for each other without a reason. `status` runs its three reads of the stage one after another, then reads every other stage one after another, even when its own stage has nothing staged and so can share no path. With five other stages, `status` of an empty stage starts 15 processes and takes about 200 ms. Every stage command ends with `protect`, which runs `write-tree`, a tree lookup, and a read of the keep ref in sequence, and the PreToolUse hook reads the snapshot and the branch tip in sequence.

## What Changes

- `status` reads the other stages only when its own stage has staged changes.
- `status` runs its staged, unstaged, and untracked reads at the same time, and reads the other stages at the same time, at most 8 `git` processes at once.
- `protect` and the PreToolUse hook run their independent reads at the same time.
- Concurrent tree lookups of one commit share one `rev-parse`.
- No behavior changes. Outputs, exit codes, and files on disk stay the same.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change alters no behavior, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `internal/ms/parallel.go`: a helper that runs reads at the same time.
- `internal/ms/commands.go`, `internal/ms/stage.go`, `internal/ms/stagearea.go`, `internal/ms/hook.go`: the callers.
- `internal/gitx/gitx.go`: `TreeOf`.
- `status`, every stage command, and the PreToolUse hook finish sooner. The measurements before and after go in `tasks.md`.
