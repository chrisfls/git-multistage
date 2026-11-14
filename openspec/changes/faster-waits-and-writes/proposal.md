# Proposal

## Why

Outside its `git` processes, git-multistage spends most of its own time in two places: flushing its metadata to the drive, and sleeping while it waits for a lock. On macOS, each atomic write of stage metadata, an edit record, or the sync record calls `F_FULLFSYNC` through `os.File.Sync`, which takes about 5 ms on an idle drive and more after Git wrote a large index. A hooked edit makes one such write, `add --edits` two, and `commit` three. When several stages commit at the same time, each commit sleeps up to 160 ms between two checks of `index.lock`, and each commit that loses the race for the branch replays its staged change, so a batch of 8 commits takes longer than the same 8 commits one after the other.

## What Changes

- On macOS, the atomic file writes flush with `fsync(2)`, as Git's default `core.fsyncMethod` (`writeout-only`) does on macOS, in place of `F_FULLFSYNC`. Linux and Windows keep `os.File.Sync`.
- A wait for a stage lock or for `index.lock` checks the lock again after at most 10 ms, in place of up to 160 ms. The timeouts stay the same.
- Commits of stages in one worktree take turns, from the read of the branch tip to the update of the branch, so a commit that waits for its turn lands without a replay. A commit gives up its turn while the `pre-commit` hook runs. A commit that waits 10 s for its turn goes on without it, as every commit does now. The compare-and-swap of the branch and the replay stay, for other Git processes and for commits that go on without a turn.
- Outputs, exit codes, and the commits, refs, and indexes that commands write stay the same. Under concurrent commits, `attempts` in `commit --json` is lower.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `commit`: concurrent commits of one worktree take turns, so a commit that waits for its turn does not replay.

## Impact

- `internal/ms/stagearea.go`: `writeFileAtomic`, with a new file-flush helper for macOS and one for the other systems.
- `internal/filelock/filelock.go` and `internal/ms/indexsync.go`: the wait loops of `Acquire` and `lockIndex`.
- `internal/ms/commit.go`: `commitAttempt`, `verifyTree`, and `land` take and give up the commit turn.
- New lock file `<git-dir>/multistage/commit.lock` in each worktree.
- Hooked edits, `add --edits`, `add`, and `commit` finish sooner on macOS; concurrent commits finish sooner on every system. The measurements before and after go in `tasks.md`.
