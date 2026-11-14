# Tasks

## 1. Baseline

- [x] 1.1 Count the `git` processes and time each command of the scenario script (two `status`, two hooked edits, `add --edits`, `diff --cached`, `commit`, `add`, `commit`) in a small repository, one command in flight, two runs. Verify: the numbers, their units, and the concurrency are recorded in this task.

  Baseline, 2026-11-14, master `a1ddbf8`, macOS, 12 cores. Repository of 5 files, one command in flight; the stage `warm` exists before the first command. Two runs:

  | Command | git processes | Run 1 | Run 2 |
  | --- | --- | --- | --- |
  | `status` (creates the stage) | 20 | 280 ms | 282 ms |
  | `status` | 14 | 191 ms | 189 ms |
  | hook PreToolUse | 11 | 150 ms | 155 ms |
  | hook PostToolUse (creates the stage) | 22 | 329 ms | 318 ms |
  | hook PreToolUse | 10 | 139 ms | 139 ms |
  | hook PostToolUse | 16 | 237 ms | 232 ms |
  | `add --edits` | 14 | 197 ms | 199 ms |
  | `diff --cached` | 10 | 136 ms | 137 ms |
  | `commit` (nothing staged) | 10 | 130 ms | 134 ms |
  | `add f3.txt` | 22 | 324 ms | 336 ms |
  | `commit -m y` | 43 | 648 ms | 678 ms |

## 2. Fewer git processes

- [x] 2.1 Read the default index path in the setup `rev-parse` of `gitx.Open`, and drop the second `rev-parse` of `OpenRepo`. Verify: every command of the scenario starts one fewer `git` process, and `go test ./...` passes.
- [x] 2.2 Memoize `TreeOf` per `Repo`. Verify: a unit test shows a second `TreeOf` of one commit starts no `git` process; `status` and `commit` start fewer processes; `go test ./...` passes.
- [x] 2.3 Read Git settings with one `git config -z --list` per process, with Git's boolean rules. Verify: a unit test covers the boolean values, a key without a value, the last value of a repeated key, mixed-case keys, and an invalid value; `commit` starts fewer processes; `go test ./...` passes.
- [x] 2.4 Make `cleanFile` add the index entry and the file with one `update-index` call. Verify: each hooked edit starts fewer processes, and the CRLF and filter tests in `go test ./...` pass.

## 3. Results

- [ ] 3.1 Measure the scenario as in 1.1 and record the numbers next to the baseline. Run `mise run ci`. Verify: CI exits with code 0, and both sets of numbers are in this task.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
