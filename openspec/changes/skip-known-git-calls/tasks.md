# Tasks

## 1. Baseline

- [x] 1.1 Count the `git` processes and time each command of the scenario script (five other stages with one staged file each; in stage `w`: two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, `commit`), one command in flight, two runs. Verify: the numbers, their units, and the concurrency are recorded in this task.

  Baseline, 2026-11-14, master `b06e598` (v0.1.3 plus this proposal), macOS, 12 cores, Git 2.54 (Apple Git-157). Repository of 5 files without Git hooks, one command in flight, wall time of each command:

  | Command | git processes | Run 1 | Run 2 |
  | --- | --- | --- | --- |
  | `status` (creates the stage) | 15 | 183 ms | 182 ms |
  | `status` | 10 | 97 ms | 103 ms |
  | hook PreToolUse | 6 | 77 ms | 78 ms |
  | hook PostToolUse | 12 | 148 ms | 165 ms |
  | hook PreToolUse (second edit) | 6 | 80 ms | 88 ms |
  | hook PostToolUse (second edit) | 12 | 153 ms | 168 ms |
  | `add --edits` (2 files) | 14 | 199 ms | 199 ms |
  | `status`, two files staged | 16 | 137 ms | 146 ms |
  | `diff --cached` | 9 | 113 ms | 112 ms |
  | `commit -m x` | 35 | 552 ms | 561 ms |
  | `add f3.txt` | 21 | 254 ms | 248 ms |
  | `commit -m y` | 35 | 543 ms | 542 ms |

## 2. Absent hooks

- [x] 2.1 Add `HasHook` to `gitx`, with the config read state, and make `commit` skip an absent hook, the temporary index of an absent `pre-commit`, and the second message cleanup of an absent `commit-msg`. Verify: unit tests cover a hook file, `<name>.exe`, a non-executable file, a sample file, an absolute and a relative `core.hooksPath`, a `~` value, a `hook.*` setting, and a failed config read; a new end-to-end test commits with a relative `core.hooksPath` hook and with a `hook.*` setting; the hook tests pass; a `commit` without hooks starts no `git hook run`.

  Result: `commit` of the scenario starts 27 processes instead of 35, both with the global `core.hooksPath=/dev/null` of the test machine and with a `HOME` whose config has no hook setting (only Git's sample hooks). Removing the `core.hooksPath` or the `hook.*` handling from `HasHook` makes the new end-to-end test fail.

## 3. Known trees and parents

- [x] 3.1 Record the base tree in `meta.json` and seed the tree cache from it when a stage's metadata is read. Verify: a unit test shows that a seeded `TreeOf` starts no `git` process; `status` and the PostToolUse hook start one process fewer; `go test ./...` passes.

  Result: `status` 10 → 9, PostToolUse 12 → 11, `add --edits` 14 → 13, `status` with staged files 16 → 15, `diff --cached` 9 → 8, `commit` 27 → 26, `add f3.txt` 21 → 19. `go test ./...` passes.
- [x] 3.2 Make `commitTree` seed the tree cache and record the parent, and make `IsAncestor` answer a recorded parent without `merge-base`. Verify: a unit test covers a recorded parent and an unknown pair; `commit` starts no `merge-base` and no `rev-parse` of its new commit's tree; `go test ./...` passes.

  Result: `commit` 26 → 24, with no `merge-base` and no tree lookup of its new commit. The unit tests and the end-to-end tests of commits, sync, and crashes pass; the full suite runs in 5.1.

## 4. Flat keep tree

- [x] 4.1 Write the keep tree with one `mktree`. Verify: the PostToolUse hook starts one process fewer; the crash-recovery tests of recorded edits and `git gc` pass.

  Result: PostToolUse 11 → 10. The `git gc`, drop, damaged-stage, and hook tests pass. A keep ref that v0.1.3 wrote (`edits` and `staged` trees) became the flat layout (`0-after`, `0-before`, `staged`) at the next `status`, with the same staged tree and parent.

## 5. Results

- [ ] 5.1 Measure the scenario as in 1.1 and record the numbers. Run `mise run ci`. Verify: CI exits with code 0, and the numbers are in this task.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
