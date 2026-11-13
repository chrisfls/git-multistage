# Tasks

## 1. Baseline

- [ ] 1.1 On an idle machine, measure one full `go test -count=1 ./...` at the default `-parallel` value, and the `git` processes and time of one `status`, one `add` followed by `commit`, and one `add --edits` in a small repository, one command in flight. Verify: the numbers, their units, and the concurrency are recorded in this task.

## 2. Fewer git processes

- [ ] 2.1 Make `gitx.Open` start one `rev-parse` and no `hash-object`, and run the version check once per process. Verify: a `status` starts at least 3 fewer `git` processes, and `go test ./...` passes.
- [ ] 2.2 Replace per-path `git` calls with batch reads in the paths that `status`, `add`, `commit`, and `add --edits` use. Verify: the counting wrapper shows fewer calls for each of the four commands, and `go test ./...` passes.

## 3. Parallel tests

- [ ] 3.1 Call `t.Parallel()` in every end-to-end test that does not change the process environment or measure time. Verify: `go test -count=1 -race ./test/` passes three times in a row, and `mise run ci:stress` passes.

## 4. Results

- [ ] 4.1 Measure the same numbers as 1.1, the same way, and record them next to the baseline. Run `mise run ci`. Verify: CI exits with code 0, and both sets of numbers are in this task.
