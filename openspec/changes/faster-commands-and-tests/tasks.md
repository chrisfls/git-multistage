# Tasks

## 1. Baseline

- [x] 1.1 On an idle machine, measure one full `go test -count=1 ./...` at the default `-parallel` value, and the `git` processes and time of one `status`, one `add` followed by `commit`, and one `add --edits` in a small repository, one command in flight. Verify: the numbers, their units, and the concurrency are recorded in this task.

  Baseline, 2026-11-13, master `c0c1a16`, macOS, 12 cores, idle machine:
  - `go test -count=1 ./...`, default `-parallel` (12), tests serial (no `t.Parallel`): 467.7 s wall; `test` package 467.3 s; CPU 174.6 s user + 229.6 s system.
  - One command in flight, small repository of 5 files, two runs (`/tmp/fc/count.sh`):

    | Command | git processes | Run 1 | Run 2 |
    | --- | --- | --- | --- |
    | `status` | 23 | 1330 ms | 880 ms |
    | `add f1.txt` | 19 | 411 ms | 405 ms |
    | `commit -m x` | 46 | 993 ms | 937 ms |
    | `add --edits` | 17 | 354 ms | 349 ms |

## 2. Fewer git processes

- [x] 2.1 Make `gitx.Open` start one `rev-parse` and no `hash-object`, and run the version check once per process. Verify: a `status` starts at least 3 fewer `git` processes, and `go test ./...` passes.
- [ ] 2.2 Replace per-path `git` calls with batch reads in the paths that `status`, `add`, `commit`, and `add --edits` use. Verify: the counting wrapper shows fewer calls for each of the four commands, and `go test ./...` passes.

## 3. Parallel tests

- [x] 3.1 Call `t.Parallel()` in every end-to-end test that does not change the process environment or measure time. Verify: `go test -count=1 -race ./test/` passes three times in a row, and `mise run ci:stress` passes.

## 4. Results

- [ ] 4.1 Measure the same numbers as 1.1, the same way, and record them next to the baseline. Run `mise run ci`. Verify: CI exits with code 0, and both sets of numbers are in this task.
