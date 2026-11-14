# Tasks

## 1. Baseline

- [x] 1.1 Count the `git` processes and time each command of the scenario script (five other stages with one staged file each; in stage `claude-w`: two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, `commit`), one command in flight, two runs. Verify: the numbers, their units, and the concurrency are recorded in this task.

  Baseline, 2026-11-14, master `3da9762` (v0.1.4), macOS, 12 cores, Git 2.54 (Apple Git-157) through `/usr/bin/git`. Repository of 9 files, `core.hooksPath` naming a missing directory, one command in flight, wall time of each command:

  | Command | git processes | Run 1 | Run 2 |
  | --- | --- | --- | --- |
  | `status` (creates the stage) | 15 | 284 ms | 233 ms |
  | `status` | 9 | 95 ms | 93 ms |
  | hook PreToolUse | 6 | 102 ms | 89 ms |
  | hook PostToolUse | 10 | 169 ms | 164 ms |
  | hook PreToolUse (second edit) | 6 | 102 ms | 90 ms |
  | hook PostToolUse (second edit) | 10 | 166 ms | 167 ms |
  | `add --edits` (2 files) | 13 | 234 ms | 223 ms |
  | `status`, two files staged | 15 | 143 ms | 144 ms |
  | `diff --cached` | 8 | 113 ms | 107 ms |
  | `commit -m x` | 24 | 439 ms | 405 ms |
  | `add f3.txt` | 19 | 266 ms | 254 ms |
  | `commit -m y` | 24 | 410 ms | 400 ms |

## 2. Git's own binary

- [x] 2.1 Make `Open` find Git's own binary when the `git` on `PATH` is the macOS Xcode shim, and run every internal call with it; calls with `Opt.User` keep the `git` on `PATH`. Verify: on macOS, a unit test shows that the binary found reports the same `git version` as `/usr/bin/git`, and that a `PATH` without `/usr/bin/git` or a set `GIT_EXEC_PATH` keeps the `git` on `PATH`; the fake-`git` tests of `gitx` pass; the scenario's calls run the binary in the exec path.

  Result: the scenario's calls run `/Library/Developer/CommandLineTools/usr/libexec/git-core/git`, after one `/usr/bin/git --exec-path` in each process, so each command starts one process more. The counting wrapper of 1.1 sits first on `PATH` and hides the shim, so the counts here come from Git's trace2 `start` events, with `GIT_TRACE2_EVENT` set and no wrapper. One traced run of each build, one command in flight: `status` 78 → 49 ms, a hooked edit 209 → 150 ms, `commit -m x` 332 → 169 ms. Both builds wrote the same commit trees. The `gitx` tests and the version and hook end-to-end tests pass.

## 3. Joint read of the tip and HEAD

- [x] 3.1 Add the joint read to `gitx` and use it in `refresh`, `prepareCommit`, and the index sync after a commit. Verify: a unit test compares the joint read with `SymbolicHead` and `ResolveRef` for a branch, a detached HEAD, a missing branch, an unborn branch, and a symbolic ref that points to a branch; `status` starts one process fewer and `commit` two fewer; the tests of commits, sync, detached HEADs, and branch mismatches pass.

  Result: counted with trace2 as in 2.1, `status` 10 → 9, `add --edits` 14 → 13, `diff --cached` 9 → 8, `commit` 25 → 23, `add f3.txt` 20 → 19. The unit tests pass, and so do the end-to-end tests whose names match commit, sync, detached HEAD, branch, recovery, crash, kill, doctor, refresh, stale, and index (38 s).

  A review then found two defects. An unborn HEAD beside a file named `HEAD` at the worktree top read as a detached HEAD, so the joint read now ends with `--`, and `TestTipAndHead` covers the case. A failed tip read in `doctorIndex` ended `doctor` with an error instead of a warning, as before this change, and the warning is back. The `gitx` tests and the doctor, commit, sync, detached, unborn, refresh, and stale end-to-end tests pass.

## 4. Message cleanup

- [x] 4.1 Clean the commit message in the process. Verify: a unit test compares the result with `git stripspace` for empty text, blank lines only, CRLF, tabs, `\v`, `\f`, NUL, text without a final LF, and runs of blank lines; an end-to-end test covers the new scenario of the commit spec; `commit` starts no `git stripspace`.

  Result: `TestCleanMessage` compares 14 chosen texts and 200 random ones with `git stripspace`, and all match. `TestCommitMessageCleanup` covers the spec scenario and a message of white space only. `commit` 23 → 22 processes, with no `stripspace`. The hook and message end-to-end tests pass.

## 5. Keep input digest

- [x] 5.1 Record the digest of the keep tree's input in the keep commit, and make `protect` skip `mktree` when it matches. Verify: `status` and `diff --cached` of a stage with staged changes start no `mktree`; a keep ref without the digest is rewritten once with the same tree and parent; the crash-recovery, `git gc`, and hook tests pass.

  Result: `TestProtectSkipsAnUnchangedKeepTree` covers both checks. In the scenario, `status` with two files staged goes 15 → 14 and `diff --cached` 8 → 7; `mktree` runs only in the hooked edits, `add --edits`, and `add`, which change the stage. The end-to-end tests whose names match garbage, damaged, drop, hook, crash, and kill pass.

## 6. Results

- [x] 6.1 Measure the scenario as in 1.1, with the old and new builds interleaved, and record the numbers. Run `mise run ci`. Verify: CI exits with code 0, and the numbers are in this task.

  After, 2026-11-14, master `ddf6d35` against `3da9762`, macOS, 12 cores, Git 2.54 (Apple Git-157) through `/usr/bin/git`, one command in flight. The times of 1.1 include the counting wrapper's shell on each call, so both builds were timed again here without it, interleaved (old, new, old, new). The process counts come from Git's trace2 `start` events, as in 2.1.

  | Command | git processes (before → after) | Before, runs 1 and 2 | After, runs 1 and 2 |
  | --- | --- | --- | --- |
  | `status` (creates the stage) | 15 → 15 | 172, 151 ms | 76, 78 ms |
  | `status` | 9 → 9 | 78, 75 ms | 42, 44 ms |
  | hook PreToolUse | 6 → 7 | 79, 70 ms | 51, 52 ms |
  | hook PostToolUse | 10 → 11 | 122, 124 ms | 75, 71 ms |
  | hook PreToolUse (second edit) | 6 → 7 | 75, 69 ms | 50, 48 ms |
  | hook PostToolUse (second edit) | 10 → 11 | 125, 125 ms | 74, 74 ms |
  | `add --edits` (2 files) | 13 → 13 | 176, 160 ms | 84, 84 ms |
  | `status`, two files staged | 15 → 14 | 109, 111 ms | 52, 52 ms |
  | `diff --cached` | 8 → 7 | 84, 82 ms | 55, 48 ms |
  | `commit -m x` | 24 → 22 | 314, 302 ms | 141, 135 ms |
  | `add f3.txt` | 19 → 19 | 215, 195 ms | 93, 97 ms |
  | `commit -m y` | 24 → 22 | 325, 325 ms | 134, 136 ms |

  - Every command takes 35 to 58 % less time. `commit` goes from about 310 ms to about 140 ms, and one hooked edit from about 195 ms to about 125 ms.
  - The after counts include the one `--exec-path` call that each process makes when `git` is the Xcode shim. With Homebrew's Git 2.53 first on `PATH`, which is no shim, each command starts one process fewer: `status` 8, a hooked edit 6 + 10, `commit` 21. Both builds wrote the same commit trees.
  - `mise run ci`: every step passed, 73 s wall; the `test` package took 68.9 s. `mise run ci:stress`: exit code 0, 57 s.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
