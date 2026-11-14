# Tasks

## 1. Baseline

- [x] 1.1 Count the `git` processes of each command of the scenario (five other stages with one staged file each; in stage `claude-w`: two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, `commit`) from Git's trace2 `start` events, one command in flight. Verify: the counts, the unit, and the concurrency are recorded in this task.

  Baseline, 2026-11-14, master `9cf4d2b` (v0.1.5), macOS, 12 cores, Git 2.54 (Apple Git-157) through `/usr/bin/git`. Repository of 9 files, `core.hooksPath` naming a missing directory, one command in flight. The counts include the one `--exec-path` call of each process.

  | Command | git processes |
  | --- | --- |
  | `status` (creates the stage) | 15 |
  | `status` | 9 |
  | hook PreToolUse | 7 |
  | hook PostToolUse | 11 |
  | `add --edits` (2 files) | 13 |
  | `status`, two files staged | 14 |
  | `diff --cached` | 7 |
  | `commit -m x` | 22 |
  | `add f3.txt` | 19 |
  | `commit -m y` | 22 |

## 2. Remembered index trees

- [x] 2.1 Read the index checksum in `gitx`, read the ref format in `Open`, and make `WriteTree` remember trees in memory and in `<git-dir>/multistage/index-trees`. Make `indexTree` look up the default index before it copies it, and make `setIndexTree` remember the tree that it read. Verify: unit tests show that a second `WriteTree` of unchanged bytes starts no `git`, that changed bytes run `write-tree`, that a zero checksum (`index.skipHash`) remembers nothing, that a damaged cache file runs `write-tree`, and that `--show-ref-format` gives `files`; `status` of an unchanged stage starts no `write-tree`; the `gitx` tests and the end-to-end tests of commit, sync, hook, crash, and drop pass.

  Result: `status` of an unchanged stage starts 8 `git` processes and no `write-tree`, in the first and the second run (trace2 `start` events, one command in flight), and `index-trees` holds one line for each tree written. `indexTree` needs no extra remembering step: `WriteTree` of the copy remembers the tree under the copy's checksum, which is the original's checksum when the original did not change during the copy. The `gitx` unit tests pass (a second `WriteTree` of unchanged bytes, changed bytes, a rewritten index, a zero checksum, a failed `write-tree`, a damaged, truncated, and unreadable cache file, a new `Repo` served by the file, the 64-line limit, and `RefFormat` `files`), and so do all `internal` tests and the end-to-end tests whose names match Commit, Sync, Hook, Crash, Kill, Drop, Damaged, Garbage, and Status (49 s).

## 3. Keep record

- [x] 3.1 Write the keep record after each new keep commit, read the loose keep ref with the `files` ref storage, and fall back to `show -s --format=%H %P%n%B` otherwise. Verify: unit tests cover a missing ref file, a ref file that matches the record, a ref file that names another commit, a keep commit of an older version without a record, and the loose ref after `git pack-refs --all`; `status` and `diff --cached` of an unchanged stage start no `show` and no `write-tree`; the crash-recovery, `git gc`, damaged-stage, and drop tests pass.

  Result: `protect` writes `<stage-dir>/keep` after each keep commit and removes it with the ref; with the `files` storage it reads the loose ref file, which stayed a file after `git pack-refs --all`. Unit tests cover a missing ref file, a file that matches the record, a file that names another commit, an old keep commit without a record, a damaged record, ref deletion, and reftable. `status` and `diff --cached` of an unchanged stage start no `show`, `write-tree`, `mktree`, `commit-tree`, or `update-ref`. Counted with trace2: `status` 9 → 7, `status` with two files staged 14 → 9, `diff --cached` 7 → 5. The crash, kill, garbage, damaged, and drop end-to-end tests pass.

## 4. Staged paths of other stages

- [x] 4.1 Remember the staged paths of each other stage in `<git-dir>/multistage/staged-paths/<id>`, and delete the file in `drop`. Verify: a unit test shows that a second read of an unchanged stage starts no `git`, that a changed index or base runs `diff-index`, and that a file with the wrong path count runs `diff-index`; `status` with staged changes and five other stages starts no `diff-index` for them the second time; the shared-path, hunks, and concurrency tests pass.

  Result: the first `status` with staged changes starts one `diff-index --name-only`, for the one other stage that no earlier command read, where the baseline started 5; a second `status` starts none. Unit tests cover an unchanged stage, a changed index or base, a wrong path count and a file cut short, a zero checksum, `drop`, and a write for a stage dropped meanwhile. The shared-path, hunks, status, add, drop, and concurrency end-to-end tests pass.

## 5. Commit

- [x] 5.1 Remember diffs in `DiffTrees`, answer a diff without renames from one with renames, and list the staged changes of `prepareCommit` with `DiffTrees`. Verify: a unit test compares the derived diff with `git diff-tree --no-renames` for renames, a move into a directory, deletions, additions, and mode changes; `commit --json` prints the same `files` as before for a rename; the commit tests pass.

  Result: `DiffTrees` keeps each diff by its two trees and the rename setting, and answers a request without renames from a kept one with renames. The derived diff equals `git diff-tree --no-renames` for renames, a move into a directory, a rename with an edit or a mode change, deletions, additions, a mode change, a file that replaces a directory, and an empty diff, in SHA-1 and SHA-256 repositories. `commit` starts no `diff-index`, and `commit --json` prints the same `files` as `git show -M` for a mixed commit. The commit tests pass. This commit (`bb675e4`) also holds the commit-turn edits of `faster-waits-and-writes` to `verifyTree`, which that change meant to land.
- [x] 5.2 Sync the default index after a commit with the commit and branch of `land`, derive the synced tree, and start the new stage index from a copy when the current index holds the committed tree. Verify: `commit` starts no `rev-parse` after `update-ref`, no `diff-tree`, no `read-tree`, and no `write-tree` in the scenario; an index entry staged by hand, an intent-to-add entry at a committed path, and a sparse checkout keep their current results; the sync, sparse, recovery, and commit tests pass.

  Result: `commit` starts no `rev-parse` after `update-ref`, no second diff, no `read-tree`, and no `write-tree`: 22 → 14 processes for both commits of the scenario, and `add` 19 → 11, with the same commit trees. An entry staged by hand, an intent-to-add entry for an empty file that the commit adds, an intent-to-add entry at another path, and a sparse checkout give the same results and skip-worktree flags as before. The sync, sparse, recovery, detached, unborn, branch, rename, and commit end-to-end tests pass.

## 6. add --edits

- [x] 6.1 Read the index entries of `add --edits` once, and write the records with one `update-index --add` of ordered `--cacheinfo` and `--force-remove` options. Verify: `add --edits` of two files starts one `ls-files` and one `update-index`; a new-file record at a path where the stage holds a directory, or the reverse, still fails; the tests of edits, moves, deletions, new files, ignored files, and edit conflicts pass.

  Result: `add --edits` of two files starts 10 processes where the baseline started 13: one `ls-files` and one `update-index`. Run against the old build, edits, deletions, new files, a path that starts with `-`, an ignored file with and without `-f`, a delete followed by a re-create, and a file replaced by a directory give the same exit codes and stage indexes. A new-file record where the stage holds a directory, and the reverse, still fail, and a failed Git write still outranks an edit conflict. After a queued deletion, `check-ignore` runs with `--no-index`, because the index copy still holds the path until the flush. The edit, move, shared-path, hunks, add, and concurrency end-to-end tests pass.

## 7. Results

- [x] 7.1 Measure the scenario as in 1.1 with the old and new builds, interleaved, and time each command; check that both builds write the same commit trees. Run `mise run ci`. Verify: CI exits with code 0, and the counts and times are in this task.

  After, 2026-11-14, master `6b16c5e` against `d121b22` (v0.1.6), macOS, 12 cores, Git 2.54 (Apple Git-157) through `/usr/bin/git`, one command in flight, wall time of each command, the builds interleaved (old, new, old, new). v0.1.6 holds the changes of `faster-waits-and-writes` and `fewer-plugin-tokens` that this scenario runs, and none of this change, so the times compare this change alone; its process counts equal those of 1.1. The counts come from Git's trace2 `start` events and include the `--exec-path` call of each process. The times include the shell and `perl` calls that time each command.

  | Command | git processes (before → after) | Before, runs 1 and 2 | After, runs 1 and 2 |
  | --- | --- | --- | --- |
  | `status` (creates the stage) | 15 → 13 | 77, 74 ms | 70, 96 ms |
  | `status` | 9 → 7 | 47, 45 ms | 40, 40 ms |
  | hook PreToolUse | 7 → 7 | 46, 42 ms | 42, 42 ms |
  | hook PostToolUse | 11 → 9 | 66, 67 ms | 64, 61 ms |
  | hook PreToolUse (second edit) | 7 → 7 | 46, 41 ms | 42, 42 ms |
  | hook PostToolUse (second edit) | 11 → 9 | 67, 67 ms | 61, 62 ms |
  | `add --edits` (2 files) | 13 → 10 | 98, 79 ms | 70, 72 ms |
  | `status`, two files staged | 14 → 9 | 54, 56 ms | 51, 50 ms |
  | `diff --cached` | 7 → 5 | 57, 54 ms | 46, 46 ms |
  | `commit -m x` | 22 → 14 | 131, 127 ms | 91, 90 ms |
  | `add f3.txt` | 19 → 11 | 104, 93 ms | 90, 75 ms |
  | `commit -m y` | 22 → 14 | 146, 125 ms | 96, 103 ms |

  - The scenario starts 157 `git` processes before and 115 after, 27 % fewer. By the mean of the runs, `commit` takes 28 % less time, `add --edits` 20 %, `diff --cached` 17 %, `add` 16 %, and `status` 13 %.
  - `status` with staged changes starts 5 processes fewer but gains little time: the reads of the other stages ran at the same time. With more stages, it saves one process for each other stage whose index did not change since an earlier command read it.
  - Both builds wrote the same commit trees in every run.
  - `mise run ci` in the shared checkout: format, lint (0 issues), vet with and without `testhooks`, and every test passed, 75 s wall; the `test` package took 70.2 s. Its last step, `openspec validate --all --strict`, failed only on `plugin-for-every-harness`, an unfinished change of another session that is not committed; the same validation passes in a clean export of `6b16c5e`.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
