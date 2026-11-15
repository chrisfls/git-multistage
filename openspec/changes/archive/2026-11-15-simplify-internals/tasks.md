# Tasks

## 1. Baseline

- [x] 1.1 Build master `77b02b0`, run `mise run ci`, and run the scenario of the design with the trace2 counts and the wall time of each command. Verify: CI exits with code 0, and the counts and times are recorded in this task.

  Baseline, 2026-11-15, master `77b02b0`, macOS, Apple M4 Pro (12 cores), Go 1.27.4, Git 2.54 (Apple Git-157). `mise run ci` exited with code 0 in 67 s wall: format, lint (0 issues), vet with and without `testhooks`, the tests, and `openspec validate --all --strict`. The scenario ran `git-multistage` directly in a 9-file repository with `multistage.shared` true and `core.hooksPath` naming a missing directory, one command in flight. Its `git` processes, counted from trace2 `start` events: `status` (creates the stage) 9, `status` 5, PreToolUse 4, PostToolUse 7, PreToolUse 4, PostToolUse 7, `add --edits` 7, `status` 7, `diff --cached` 3, `commit` 9, `add f3.txt` 8, `commit` 9; 79 in all, as in `fewer-git-spawns`.

## 2. Benchmarks

- [x] 2.1 Add `BenchmarkApply` in `internal/hunks/bench_test.go`, and `BenchmarkPatchFiles` and `BenchmarkCodexSection` in `internal/ms/bench_test.go`, and run them on a clean export of `77b02b0` with `-benchmem -count=10`. Verify: the baseline results are recorded in this task.

  Baseline on a clean export of `77b02b0`, `-benchmem -count=10`, medians: `BenchmarkApply` 44.7 µs, 334.0 KiB, 26 allocs per op; `BenchmarkPatchFiles` 2.76 µs, 3.33 KiB, 25 allocs; `BenchmarkCodexSection` 12.0 µs, 9.58 KiB, 181 allocs.

## 3. gitx and hunks

- [x] 3.1 Delete `Repo.Env`; copy the base environment in `run` only for a non-`User` call; end `saveIndexTrees` with `writeCacheFile`; remove the shadowing in `TipAndHead`. Verify: the `gitx` tests pass with `-race`.
- [x] 3.2 Make `hunks.Apply` write into a grown `bytes.Buffer` and return its bytes; add a test of an empty result. Verify: the `hunks` tests pass, and `BenchmarkApply` shows no more allocations or bytes than the baseline.

  Result: `BenchmarkApply` 30.3 µs (−32 %), 128.5 KiB (−62 %), 7 allocs (−73 %), p ≤ 0.002 by `benchstat`, n = 10. `TestApplyEmptiesFile` covers a result with no content.

## 4. ms

- [x] 4.1 Delete `stagedEntry` and use `indexEntry`; return the map of `readIndexEntries` from `lsIndexMany` without a copy. Verify: the `ms` tests pass.
- [x] 4.2 Drop the error result of `otherStagesStaged` and its callers' error branches. Verify: the `ms` tests and the end-to-end tests whose names match Shared, Hunk, Status, and Add pass.
- [x] 4.3 Clear the warnings in `emit`, remove the thirteen `a.warnings = nil` lines, call `flushWarnings` once in the error path of `Main`, and size the JSON slice of `addPaths`. Verify: the end-to-end tests whose names match JSON, Warn, and Hint pass.
- [x] 4.4 Add `randomHex` and use it in `hookIndexPath`, `writeRecord`, `renameCaseOnly`, and `NewStageID`. Verify: the `ms` tests pass.
- [x] 4.5 Add the table of Codex patch headers and use it in `codexHeader` and `patchFiles`. Verify: the end-to-end tests whose names match Codex and Patch pass, and `BenchmarkPatchFiles` and `BenchmarkCodexSection` show no more allocations than the baseline.

  Result: `BenchmarkCodexSection` 5.21 µs (−57 %) and `BenchmarkPatchFiles` 2.62 µs (−5 %), with the same bytes and allocations; Go did not allocate the map literal, but ranging over it cost the time.
- [x] 4.6 Remove the destination in `copyFile` when a write after the create fails; add a unit test. Verify: the test fails without the removal, and the `ms` tests pass.

  Result: `TestCopyFileRemovesPartialCopy` copies a directory, which opens but cannot be read; it fails with the removal deleted and passes with it.
- [x] 4.7 Apply the small cleanups of the design: `snapshotBlob`, the pending-disk checks of `RemovePaths` and `MovePath`, `runEvent`, `runHook`, `quotePath`, `moveBase`, `syncLanded`, `markStagedBy`, the comment of `protect`, and `replayRecord`'s reader. Verify: the `ms` tests pass.

## 5. Results

- [x] 5.1 Run `golangci-lint fmt --diff`, `golangci-lint run`, `go vet` with and without `testhooks`, `go vet` for Windows and Linux, the `gitx`, `hunks`, and `ms` tests with `-race`, `mise run ci`, and `mise run ci:stress`. Verify: every check exits with code 0.

  Result: `golangci-lint fmt --diff` and `golangci-lint run` (0 issues), `go vet` with and without `testhooks` for darwin, linux, and windows, the `filelock`, `gitx`, `hunks`, and `ms` tests with `-race`, `mise run ci` (exit 0, 78 s wall, 13 OpenSpec items valid), and `mise run ci:stress` (exit 0, 44 s) pass. Windows and Linux ran only `go vet`, not the tests.
- [x] 5.2 Run the scenario with the old and new builds, interleaved, five times each, and run the benchmarks of 2.1 on the new tree. Verify: every command starts the same number of `git` processes, both builds write the same commit trees, no median time is slower beyond the noise of the old build's own runs, the benchmarks show no more allocations, and the results are recorded in this task.

  Result, 2026-11-15, the old build of `77b02b0` against the new tree, the scenario of 1.1, one command in flight, 10 rounds of each build that alternate which build runs first; each time is the median of 10 wall times. A first pass of 5 rounds while other sessions loaded the machine (load average 7 to 9) gave a sum of medians 4.5 % higher for the new build, and a second pass gave 3.6 % lower, with single runs up to 1.3 s; the table is a third pass at load average 3 to 5.

  | Command | git processes (old → new) | Old | New |
  | --- | --- | --- | --- |
  | `status` (creates the stage) | 9 → 9 | 54.5 ms | 54.6 ms |
  | `status` | 5 → 5 | 30.1 ms | 30.1 ms |
  | hook PreToolUse | 4 → 4 | 33.8 ms | 35.0 ms |
  | hook PostToolUse | 7 → 7 | 57.2 ms | 57.5 ms |
  | hook PreToolUse (second edit) | 4 → 4 | 34.2 ms | 33.0 ms |
  | hook PostToolUse (second edit) | 7 → 7 | 56.5 ms | 55.9 ms |
  | `add --edits` (2 files) | 7 → 7 | 61.3 ms | 59.0 ms |
  | `status`, two files staged | 7 → 7 | 46.0 ms | 43.9 ms |
  | `diff --cached` | 3 → 3 | 39.1 ms | 38.2 ms |
  | `commit -m x` | 9 → 9 | 71.2 ms | 70.3 ms |
  | `add f3.txt` | 8 → 8 | 71.0 ms | 67.1 ms |
  | `commit -m y` | 9 → 9 | 71.8 ms | 70.6 ms |

  - Both builds start 79 `git` processes, the same number in every command, and wrote the same two commit trees in all 20 runs.
  - The sum of the medians is 627 ms for the old build and 615 ms for the new one. Every difference is smaller than the spread of the old build's own runs, so the commands take the same time; the process spawns dominate them.
  - The benchmarks are those of 3.2 and 4.5.
- [x] 5.3 Review the diff for changes of behavior, unused code, and changes outside the scope of the design. Verify: each finding is fixed or recorded here.

  Result: a review of the diff against `77b02b0` checked every `emit` and read of the warnings, the callers of `lsIndexMany`, `hunks.Apply`, and `otherStagesStaged`, the header lines of `patchFiles`, and the flow of `TipAndHead` and `snapshotBlob`, and found them equal to the old code. It noted two differences that the design names: `copyFile` also removes a complete copy when `Chtimes` fails, which no caller adopts after an error, and `hunks.Apply` allocates once for a result that deletes every line, recorded under Risks.
