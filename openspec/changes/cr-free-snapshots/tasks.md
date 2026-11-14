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

- [ ] 3.1 Measure the scenario as in 1.1 and record the numbers. Run `mise run ci`. Verify: CI exits with code 0, and the numbers are in this task.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
