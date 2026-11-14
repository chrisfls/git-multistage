# Tasks

## 1. Baseline

- [x] 1.1 Count the `git` processes and time the commands of the scenario script and of a scenario with five other stages, one command in flight, two runs. Verify: the numbers are recorded in this task.

  Baseline, 2026-11-14, master `9d74446` (v0.1.2), macOS, 12 cores.

  Five other stages, each with one staged file:

  | Command | git processes | Run 1 | Run 2 |
  | --- | --- | --- | --- |
  | `status`, nothing staged | 15 | 196 ms | 210 ms |
  | `add f1.txt` | 20 | 299 ms | 306 ms |
  | `status`, one file staged | 16 | 222 ms | 221 ms |
  | `diff --cached` | 9 | 126 ms | 122 ms |
  | `commit` | 35 | 595 ms | 578 ms |

  Scenario script, stage exists:

  | Command | git processes | Run 1 | Run 2 |
  | --- | --- | --- | --- |
  | `status` | 11 | 151 ms | 149 ms |
  | hook PreToolUse | 6 | 105 ms | 91 ms |
  | hook PostToolUse | 12 | 186 ms | 188 ms |
  | `add f3.txt` | 18 | 282 ms | 275 ms |
  | `commit -m y` | 35 | 579 ms | 573 ms |

## 2. Reads at the same time

- [x] 2.1 Make concurrent `TreeOf` calls for one commit share one `rev-parse`, and drop a failed lookup. Verify: unit tests with `-race` show one `git` call for 8 concurrent lookups and no kept failure.
- [ ] 2.2 Add `parallel`, skip the other stages for an empty stage in `status`, and run the reads of `status`, `otherStagesStaged`, `protect`, and the PreToolUse hook at the same time. Verify: unit tests of `parallel` with `-race` cover the error order and the bound; a `-race` build reports no race in `add`, `status` with shared paths, and the hook; no command of the two scenarios starts more `git` processes; the end-to-end tests of status, hooks, and stages pass.

## 3. Results

- [ ] 3.1 Measure as in 1.1 and record the numbers. Run `mise run ci`. Verify: CI exits with code 0, and the numbers are in this task.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
