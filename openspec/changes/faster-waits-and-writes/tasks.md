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

- [x] 3.1 Make `filelock.Acquire` and `lockIndex` check again after 1 ms, doubling up to 10 ms. Verify: a `filelock` unit test takes a lock within 30 ms of its release by another holder that held it for 300 ms; the busy-stage and `index.lock` timeout tests still pass with their exit codes.

  Result: `TestWaiterWakesSoon` passes three times in a row. `TestBusyStageExits6` and the orphaned `index.lock` tests pass, and so do the end-to-end tests whose names match crash, kill, concurrent, many, parallel, race, and sync (16 s). No test holds `index.lock` for the 15 s timeout of `commit`; the code that times out did not change.

## 4. Commit turns

- [x] 4.1 Take the commit turn in each commit attempt, as the design describes, and give it up after `update-ref`, before the `pre-commit` hook, and on an early return; go on without it after 10 s or on a failure to take it. Verify: end-to-end tests cover the four scenarios of the `commit` spec delta (8 commits at once each report `attempts` 1; a turn held from the test past a shortened wait; a commit killed at `before-update-ref`; a slow `pre-commit` hook), and the concurrency, race, crash, and hook tests pass.

  Result: `TestManyStagesCommitInTurns`, `TestCommitGoesOnWithoutAHeldTurn`, `TestKilledCommitGivesUpItsTurn`, and `TestSlowPreCommitHookGivesUpTheTurn` pass. With the turn disabled, the first two fail (an `attempts` of 3; a commit that did not wait), and with the turn kept through the hook, the last one fails (stage b's commit waited 10.2 s). The end-to-end tests whose names match concurrent, many, parallel, race, compare-and-swap, crash, kill, hook, commit, and turn pass (36 s).

  A review then found that a commit whose `pre-commit` hook ran moved the branch without its turn, so a commit that held the turn could still replay, against the spec. The attempt now takes the turn again after the hook and ends when the tip moved meanwhile. `TestHookedCommitTakesTheTurnAgain` covers it: without the fix, the commit that held the turn reports `attempts` 2. The review also found a test worker that could call `t.Fatal`, now fixed. The turn tests pass three times in a row.

## 5. Results

- [x] 5.1 Measure the scenario and the concurrent batches as in 1.1, with the old and new builds interleaved, and record the numbers. Run `mise run ci` and `mise run ci:stress`. Verify: both exit with code 0, and the numbers are in this task.

  After, 2026-11-14, master `d423f86` against `9cf4d2b`, macOS, 12 cores, Git 2.54 (Apple Git-157) through `/usr/bin/git`, with the old and new builds interleaved (old, new, old, new). Other sessions ran tests on the machine meanwhile, so single runs vary by about 10 ms. The new build also holds another change's edit of a hook message, which starts no process.

  | Command | Before, runs 1 and 2 | After, runs 1 and 2 |
  | --- | --- | --- |
  | `status` (creates the stage) | 90.8, 88.9 ms | 92.2, 81.2 ms |
  | `status` | 45.2, 54.1 ms | 46.1, 50.4 ms |
  | hook PreToolUse | 54.2, 60.3 ms | 44.5, 46.3 ms |
  | hook PostToolUse | 77.2, 96.8 ms | 66.2, 71.6 ms |
  | hook PreToolUse (second edit) | 52.2, 51.7 ms | 40.1, 45.2 ms |
  | hook PostToolUse (second edit) | 78.1, 83.7 ms | 66.8, 76.4 ms |
  | `add --edits` (2 files) | 93.9, 106.3 ms | 81.3, 79.9 ms |
  | `status`, two files staged | 56.0, 61.7 ms | 53.1, 57.8 ms |
  | `diff --cached` | 54.4, 56.7 ms | 52.1, 55.5 ms |
  | `commit -m x` | 155.9, 160.2 ms | 124.0, 133.3 ms |
  | `add f3.txt` | 113.2, 106.7 ms | 89.3, 96.1 ms |
  | `commit -m y` | 145.6, 152.7 ms | 122.4, 134.4 ms |

  Concurrent commits, wall time of the batch, interleaved:

  | Batch | Before | After |
  | --- | --- | --- |
  | 8 commits, 8 in flight | 1423, 1426, 1380, 1542, 1631 ms | 916, 815, 839, 819, 830 ms |
  | 4 commits, 4 in flight | 753, 725, 761 ms | 513, 403, 402 ms |

  - The commands that write metadata take 10 to 30 ms less: a hooked edit (both hooks) about 139 → 114 ms, `add --edits` about 100 → 81 ms, `commit` about 154 → 129 ms. `status` and `diff --cached`, which write nothing, stay the same within the noise.
  - A batch of 8 concurrent commits takes about 42 % less time (median 1426 → 830 ms), and a batch of 4 about 46 % less (median 753 → 403 ms). One commit alone takes about 129 ms, so 8 commits one after the other take about 1030 ms: 8 commits in turns now take less, where before they took about 40 % more.
  - `mise run ci` in the shared checkout failed its format check on another session's unfinished files in `internal/gitx/`, so its steps ran on a clean export of `d423f86`: every step passed, 79 s. The steps of `mise run ci:stress` (`MULTISTAGE_STRESS=3`), with the new tests' names added to its pattern, passed in 76 s.
  - The 8-commit test is named `TestManyStagesCommitInTurns`, so `ci:stress` repeats it.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
