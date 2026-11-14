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

- [ ] 3.1 Write the keep record after each new keep commit, read the loose keep ref with the `files` ref storage, and fall back to `show -s --format=%H %P%n%B` otherwise. Verify: unit tests cover a missing ref file, a ref file that matches the record, a ref file that names another commit, a keep commit of an older version without a record, and the loose ref after `git pack-refs --all`; `status` and `diff --cached` of an unchanged stage start no `show` and no `write-tree`; the crash-recovery, `git gc`, damaged-stage, and drop tests pass.

## 4. Staged paths of other stages

- [ ] 4.1 Remember the staged paths of each other stage in `<git-dir>/multistage/staged-paths/<id>`, and delete the file in `drop`. Verify: a unit test shows that a second read of an unchanged stage starts no `git`, and that a changed index or base runs `diff-index`; `status` with staged changes and five other stages starts no `diff-index` for them the second time; the shared-path, hunks, and concurrency tests pass.

## 5. Commit

- [ ] 5.1 Remember diffs in `DiffTrees`, answer a diff without renames from one with renames, and list the staged changes of `prepareCommit` with `DiffTrees`. Verify: a unit test compares the derived diff with `git diff-tree --no-renames` for renames, a move into a directory, deletions, additions, and mode changes; `commit --json` prints the same `files` as before for a rename; the commit tests pass.
- [ ] 5.2 Sync the default index after a commit with the commit and branch of `land`, derive the synced tree, and start the new stage index from a copy when the current index holds the committed tree. Verify: `commit` starts no `rev-parse` after `update-ref`, no `diff-tree`, no `read-tree`, and no `write-tree` in the scenario; an index entry staged by hand, an intent-to-add entry at a committed path, and a sparse checkout keep their current results; the sync, sparse, recovery, and commit tests pass.

## 6. add --edits

- [ ] 6.1 Read the index entries of `add --edits` once, and write the records with one `update-index -z --index-info`. Verify: `add --edits` of two files starts one `ls-files` and one `update-index`; the tests of edits, moves, deletions, new files, ignored files, and edit conflicts pass.

## 7. Results

- [ ] 7.1 Measure the scenario as in 1.1 with the old and new builds, interleaved, and time each command; check that both builds write the same commit trees. Run `mise run ci`. Verify: CI exits with code 0, and the counts and times are in this task.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
