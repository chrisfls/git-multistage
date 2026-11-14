# Tasks

## 1. Baseline

- [x] 1.1 Count the `git` processes and time each command of the scenario script, as in `fewer-git-processes`, one command in flight, two runs. Verify: the numbers are recorded in this task.

  Baseline, 2026-11-14, master `dde5d8c` (v0.1.1): the "after" table of `fewer-git-processes`. One hooked edit (PreToolUse and PostToolUse, stage exists) starts 8 + 14 = 22 processes, 321 to 352 ms.

  Suite, same machine, full `go test -count=1 ./...` at the default `-parallel` (12), two rounds alternating the two commits; `test` package time:

  | Commit | Round 1 | Round 2 |
  | --- | --- | --- |
  | `a1ddbf8` (before `fewer-git-processes`) | 153.7 s | 169.4 s |
  | `dde5d8c` (after `fewer-git-processes`) | 146.6 s | 150.6 s |

## 2. Snapshots without an index

- [x] 2.1 Store a snapshot of a file or of content without CR with one `hash-object --path`, and keep the temporary index for content with CR. Verify: a new end-to-end test commits a hooked edit and an Edit's own change through a clean filter; it fails when the fast path drops `--path` or uses `--no-filters`; the CRLF tests pass.

## 3. Results

- [x] 3.1 Measure the scenario as in 1.1 and record the numbers. Run `mise run ci`. Verify: CI exits with code 0, and the numbers are in this task.

  After, 2026-11-14, master `f919027` with the same code built, one command in flight, two runs:

  | Command | git processes (before) | git processes (after) | Run 1 | Run 2 |
  | --- | --- | --- | --- | --- |
  | hook PreToolUse (stage exists) | 8 | 6 | 98 ms | 87 ms |
  | hook PostToolUse (stage exists) | 14 | 12 | 183 ms | 180 ms |
  | hook PreToolUse (no stage yet) | 9 | 7 | 119 ms | 105 ms |
  | hook PostToolUse (creates the stage) | 18 | 16 | 267 ms | 246 ms |

  - One hooked edit (stage exists) went from 22 to 18 processes, and from 321 to 352 ms to 267 to 281 ms. The other commands of the scenario start the same processes as before.
  - `mise run ci`: exit code 0, 144 s wall; the `test` package took 139.6 s.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
