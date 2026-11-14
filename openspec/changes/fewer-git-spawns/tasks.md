# Tasks

## 1. Baseline

- [x] 1.1 Count the `git` processes of each command of the scenario (five other stages with one staged file each; in stage `claude-w`: two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, `commit`) from Git's trace2 `start` events, one command in flight. Verify: the counts, the unit, and the concurrency are recorded in this task.

  Baseline, 2026-11-14, master `644107e`, macOS, 12 cores, Git 2.54 (Apple Git-157) through `git multistage`. Repository of 9 files, `core.hooksPath` naming a missing directory, `core.ignoreCase` true (the default of `git init` on macOS), one command in flight. The counts come from trace2 `start` events and include the `git multistage` process of each command. The times are the wall time of each command, measured by the shell.

  | Command | git processes | Run 1 | Run 2 |
  | --- | --- | --- | --- |
  | `status` (creates the stage) | 13 | 68 ms | 67 ms |
  | `status` | 7 | 35 ms | 34 ms |
  | hook PreToolUse | 7 | 43 ms | 37 ms |
  | hook PostToolUse | 9 | 54 ms | 57 ms |
  | hook PreToolUse (second edit) | 7 | 41 ms | 35 ms |
  | hook PostToolUse (second edit) | 9 | 56 ms | 55 ms |
  | `add --edits` (2 files) | 10 | 71 ms | 63 ms |
  | `status`, two files staged | 9 | 47 ms | 48 ms |
  | `diff --cached` | 5 | 43 ms | 33 ms |
  | `commit -m x` | 14 | 90 ms | 86 ms |
  | `add f3.txt` | 11 | 83 ms | 71 ms |
  | `commit -m y` | 14 | 91 ms | 83 ms |

  The scenario starts 115 `git` processes.

## 2. Version check

- [x] 2.1 Make `Open` take a cache directory name, run the setup `rev-parse` first, and skip `git version` when `<git-dir>/multistage/git-version` holds the binary's resolved path, size, and modification time; run the check when `rev-parse` fails, and write the file after a passed check. Verify: unit tests show that a second `Open` starts no `git version`, that a changed size, modification time, or path runs it, that a damaged file runs it, that `""` writes no file, and that a Git older than 2.45 still fails `Open` with the version error; the `gitx` tests pass.

  Result: every command after the first one in a worktree starts no `git version`: the scenario starts 103 processes instead of 115, one fewer in each command. Unit tests cover a second check of the same binary, a new modification time, a new size, another path, a damaged cache file, no cache name, an old Git that is not remembered, the cache file that `Open` writes, and an old Git that fails `rev-parse` and gets the version error. The `gitx` tests pass with `-race`, and so do the `ms` unit tests.

## 3. Ref files

- [x] 3.1 Add a reader of `HEAD` and `refs/heads/<name>` for the `files` ref storage, and make `ResolveRef`, `SymbolicHead`, and `TipAndHead` use it, with `git` for every case that it does not handle. Verify: unit tests compare it with `git rev-parse` and `git symbolic-ref` for loose, packed, peeled, symbolic, chained, detached, and unborn refs, a branch name that is also a directory, a linked worktree, and SHA-256; they show a `git` fallback for a symbolic-link `HEAD`, malformed content, a malformed `packed-refs`, a ref outside `refs/heads/`, an invalid name, and reftable; `status` of an unchanged stage starts no `rev-parse` other than the setup and no `symbolic-ref`; the end-to-end tests of commit, detached, unborn, branch, and hook pass.

  Result: `status`, `add`, `diff --cached`, and `commit` start no `rev-parse` other than the setup call and no `symbolic-ref`, and the PreToolUse hook reads the branch tip without `rev-parse`: the scenario starts 87 processes instead of 103. The unit tests compare the reader with `git rev-parse --verify` and `git symbolic-ref` for loose, packed (with peeled lines), loose-over-packed, chained symbolic, detached, and unborn refs, a branch name that is a directory or lies below a file, and a linked worktree, in SHA-1 and SHA-256; they hand malformed content, the zero ID, an upper-case ID, a symbolic ref outside `refs/heads/`, a chain of six names, a tag, invalid names, a malformed `packed-refs`, a symbolic-link `HEAD`, and reftable to Git. Breaking the `packed-refs` lookup makes the comparison fail. A new end-to-end test checks the calls of `status` and `commit`. The `gitx` tests pass with `-race`, and the end-to-end tests whose names match Commit, Detached, Unborn, Branch, Hook, Status, Sync, Recover, Crash, Kill, Worktree, Stale, and Move pass (45 s).

## 4. Index entries

- [ ] 4.1 Add `ReadIndex` for index versions 2, 3, and 4, and make `lsIndex`, `readIndexEntries`, and the index spelling of `newEditHook` use it, with `ls-files` for every case that it does not handle. Verify: unit tests compare it with `git ls-files -s -z` for versions 2, 3, and 4, SHA-1 and SHA-256, unmerged and intent-to-add entries, a path longer than 4095 bytes, and a missing index; they show an `ls-files` fallback for a split index, a sparse index, an unknown required extension, and a file that ends early; the hooked edit, `add --edits`, `add`, and `commit` of the scenario start no `ls-files -s` and no `ls-files` with `GIT_ICASE_PATHSPECS`; the end-to-end tests of edits, hooks, case, sparse, split, add, and commit pass.

## 5. Results

- [ ] 5.1 Measure the scenario as in 1.1 with the old and new builds, interleaved, and time each command; check that both builds write the same commit trees. Run `mise run ci`. Verify: CI exits with code 0, and the counts and times are in this task.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
