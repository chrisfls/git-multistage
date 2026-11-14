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

- [ ] 2.1 Add `HasHook` to `gitx`, with the config read state, and make `commit` skip an absent hook, the temporary index of an absent `pre-commit`, and the second message cleanup of an absent `commit-msg`. Verify: unit tests cover a hook file, `<name>.exe`, a non-executable file, a sample file, an absolute and a relative `core.hooksPath`, a `~` value, a `hook.*` setting, and a failed config read; a new end-to-end test commits with a relative `core.hooksPath` hook and with a `hook.*` setting; the hook tests pass; a `commit` without hooks starts no `git hook run`.

## 3. Known trees and parents

- [ ] 3.1 Record the base tree in `meta.json` and seed the tree cache from it when a stage's metadata is read. Verify: a unit test shows that a seeded `TreeOf` starts no `git` process; `status` and the PostToolUse hook start one process fewer; `go test ./...` passes.
- [ ] 3.2 Make `commitTree` seed the tree cache and record the parent, and make `IsAncestor` answer a recorded parent without `merge-base`. Verify: a unit test covers a recorded parent and an unknown pair; `commit` starts no `merge-base` and no `rev-parse` of its new commit's tree; `go test ./...` passes.

## 4. Flat keep tree

- [ ] 4.1 Write the keep tree with one `mktree`. Verify: the PostToolUse hook starts one process fewer; the crash-recovery tests of recorded edits and `git gc` pass.

## 5. Results

- [ ] 5.1 Measure the scenario as in 1.1 and record the numbers. Run `mise run ci`. Verify: CI exits with code 0, and the numbers are in this task.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
