# Proposal

## Why

After `reuse-git-results`, the scenario of the earlier performance changes starts 115 `git` processes for 12 commands on master `33fbf02`. A trace of every process shows that about a third of what remains reads state that git-multistage can read itself, from files whose format is simple and stable:

- Every command starts `git version` beside its setup `rev-parse`, although the Git binary rarely changes between two commands: 12 processes.
- Every stage command, each commit, and each PreToolUse hook read HEAD and the tip of a branch with `rev-parse` or `symbolic-ref`: 18 processes, one to three in each command.
- Each hook on a case-insensitive file system (the default on macOS), `add`, `add --edits`, and the index sync after a commit read index entries with `ls-files`: 8 processes.

The user asked whether a Git library could take over these reads. go-git was measured against them today in the design of `cheaper-git-calls` and rejected: it cannot read per-worktree refs from a linked worktree, fails on split and sparse indexes and on reftable, does not follow config includes, and adds 49 modules to a program that has none. The three reads above need far less than a library: the loose and packed ref files, and the entry table of an index file.

## What Changes

- git-multistage remembers that the Git binary passed the version check, in `<git-dir>/multistage/git-version`, keyed by the binary's resolved path, size, and modification time. A command whose binary matches starts no `git version`. Any other case, and any failed setup `rev-parse`, runs the check as now.
- With the `files` ref storage, HEAD and the branches under `refs/heads/` are read from their loose files and from `packed-refs`. A symbolic link, a malformed file, a ref outside `refs/heads/`, and the `reftable` storage run `git` as now.
- The stage-0 entries and conflicted paths of chosen paths, and the index spelling of a path on a case-insensitive file system, are read from the index file. A split index, a sparse index, an unknown index version, an unknown required extension, and a malformed file run `ls-files` as now.
- Outputs, exit codes, and the commits, refs, and indexes that commands write stay the same. One new cache file goes in `<git-dir>/multistage/`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. No requirement changes, so the change sets `skip_specs: true`. The `cli` requirement that the tool fails at startup with a Git older than 2.45 still holds: the version check runs for every binary that has not passed it.

## Impact

- `internal/gitx/gitx.go`: `Open`, the version check, `ResolveRef`, `SymbolicHead`, and `TipAndHead`.
- `internal/gitx/refs.go` and `internal/gitx/index.go`: new readers of the ref files and of index entries, with their tests.
- `internal/ms/stage.go`, `internal/ms/indexsync.go`, and `internal/ms/hook.go`: `lsIndex`, `readIndexEntries`, and the index spellings in `canonicalUnheld` and `newEditHook` call the index reader.
- Every command starts fewer `git` processes. The measurements before and after go in `tasks.md`.
