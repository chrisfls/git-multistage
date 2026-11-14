# Tasks

## 1. Baseline

- [x] 1.1 Time each command of the scenario script of `cheaper-git-calls` (five other stages with one staged file each; in stage `claude-w`: two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, `commit`), one command in flight, two runs; and time batches of 8 and of 4 commits from as many stages, all in flight. Verify: the numbers, their units, and the concurrency are recorded in this task.

  Baseline, 2026-11-14, master `9cf4d2b`, macOS, 12 cores, Git 2.54 (Apple Git-157) through `/usr/bin/git`, repository of 9 files, wall time of each command:

  | Command | Run 1 | Run 2 |
  | --- | --- | --- |
  | `status` (creates the stage) | 87.4 ms | 80.1 ms |
  | `status` | 40.8 ms | 43.7 ms |
  | hook PreToolUse | 53.3 ms | 49.7 ms |
  | hook PostToolUse | 74.0 ms | 70.8 ms |
  | hook PreToolUse (second edit) | 51.8 ms | 49.2 ms |
  | hook PostToolUse (second edit) | 73.9 ms | 74.2 ms |
  | `add --edits` (2 files) | 100.8 ms | 90.7 ms |
  | `status`, two files staged | 53.3 ms | 52.9 ms |
  | `diff --cached` | 57.8 ms | 51.6 ms |
  | `commit -m x` | 148.0 ms | 143.4 ms |
  | `add f3.txt` | 100.8 ms | 97.9 ms |
  | `commit -m y` | 153.0 ms | 141.9 ms |

  Concurrent commits, each stage with one staged file of its own, wall time of the batch: batch of 8, 8 in flight: 1580, 1723, 1472, 1349, 1288 ms. Batch of 4, 4 in flight: 740, 729, 650 ms.

## 2. Writes

- [x] 2.1 Flush atomic writes with `fsync(2)` on macOS and with `os.File.Sync` elsewhere. Verify: a unit test writes a file with `writeFileAtomic` and reads it back; the scenario's writes take under 1 ms each on macOS; the crash, kill, hook, and sync end-to-end tests pass.

  Result: `TestWriteFileAtomic` passes, and `go vet` passes for Linux and Windows. In a build with timers, the scenario's writes took 0.17 to 0.23 ms each, against 4.4 to 7.0 ms before. The end-to-end tests whose names match crash, kill, hook, sync, and recover pass (14 s).

## 3. Lock waits

- [ ] 3.1 Make `filelock.Acquire` and `lockIndex` check again after 1 ms, doubling up to 10 ms. Verify: a `filelock` unit test takes a lock within 30 ms of its release by another holder that held it for 300 ms; the busy-stage and `index.lock` timeout tests still pass with their exit codes.

## 4. Commit turns

- [ ] 4.1 Take the commit turn in each commit attempt, as the design describes, and give it up after `update-ref`, before the `pre-commit` hook, and on an early return; go on without it after 10 s or on a failure to take it. Verify: end-to-end tests cover the four scenarios of the `commit` spec delta (8 commits at once each report `attempts` 1; a turn held from the test past a shortened wait; a commit killed at `before-update-ref`; a slow `pre-commit` hook), and the concurrency, race, crash, and hook tests pass.

## 5. Results

- [ ] 5.1 Measure the scenario and the concurrent batches as in 1.1, with the old and new builds interleaved, and record the numbers. Run `mise run ci` and `mise run ci:stress`. Verify: both exit with code 0, and the numbers are in this task.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
