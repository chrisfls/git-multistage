# Tasks

## 1. Baseline

- [ ] 1.1 Time stage creation and a base move with the current build, on a generated repository of 30,000 tracked files and on a copy of the Core checkout, one command in flight: the first `status` of a new stage while another stage exists, and the next `status` of a stage after another stage committed. Count their `git` processes from Git's trace2 `start` events. Verify: the times, the counts, the unit, and the concurrency are recorded in this task.

## 2. Index from a copy

- [ ] 2.1 Add `fillIndex` and `statSeed` to `internal/ms/stagearea.go`, and make `setIndexTree` use them. Verify: end-to-end tests show that a base move and a new stage run `read-tree --reset` and give the stage index the entries of the tree with none of the seed's staged changes; that a racily clean entry of the seed whose file changed shows as unstaged in the new stage; that after `git sparse-checkout disable` a new stage and a moved base have no skip-worktree bit; that an assume-unchanged entry of the default index stays out of a new stage; that a stage whose index file is missing still gets a new index; and that `status` starts as many `git` processes as before. The stage, commit, sync, sparse, hook, crash, drop, and concurrency end-to-end tests pass.

## 3. Results

- [ ] 3.1 Measure as in 1.1 with the old and new builds, interleaved, and check that both builds give stage indexes with the same entries and bits. Run `mise run ci`. Verify: CI exits with code 0, and the counts and times are in this task.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
