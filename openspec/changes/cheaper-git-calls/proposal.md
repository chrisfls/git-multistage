# Proposal

## Why

On macOS, the `git` on `PATH` is usually `/usr/bin/git`, the Xcode shim. Each run of the shim looks up the active developer directory and then runs the Git in it. `git rev-parse --git-dir` takes 6.8 ms through the shim and 3.4 ms through Git's own binary (median of 50 runs, one in flight). After `skip-known-git-calls`, `status` starts 9 `git` processes, a hooked edit 16, and `commit` 24, so the shim takes about half the time of every command on macOS.

Some of the remaining calls also repeat work. Every stage command reads the branch that HEAD points to and the tip of the stage's branch with two processes, and `commit` does it twice. `commit` cleans its message with a `git stripspace` process. `protect` runs `mktree` at the end of every command of a stage that has staged changes or recorded edits, even when the keep ref already holds that tree.

A Go Git library is still rejected. The design measures go-git against the reads that it would replace: it cannot read the keep refs or follow config includes, and it adds 49 modules.

## What Changes

- On macOS, when the `git` on `PATH` is the Xcode shim, git-multistage's own Git calls run the Git binary that `git --exec-path` names. Calls that run hooks or print Git's output to the user still run the `git` on `PATH`.
- One `git rev-parse` reads the tip of the stage's branch and the branch that HEAD points to, in every stage command and twice in `commit`.
- `commit` cleans the message in the process, as `git stripspace` does.
- The keep ref's commit records a digest of the input of its tree, so `protect` skips `mktree` when the staged tree and the recorded edits did not change.
- Outputs, exit codes, and the commits, refs, and indexes that commands write stay the same. The message of the keep ref's commit gains one line.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `commit`: the message requirement cleans the message as `git stripspace` does, without requiring a `git stripspace` process.

## Impact

- `internal/gitx/gitx.go`: `Open`, `run`, and a joint read of the tip and HEAD.
- `internal/ms/integrate.go`: `refresh`.
- `internal/ms/commit.go`: `prepareCommit`, `checkTarget`, `prepareMessage`, and the call of the index sync.
- `internal/ms/indexsync.go` and `internal/ms/commands.go`: `syncAfterCommit`, `syncIndexLocked`, and their callers.
- `internal/ms/stagearea.go`: `protect`.
- Every command and both edit hooks finish sooner on macOS, and start fewer `git` processes on every system. The measurements before and after go in `tasks.md`.
