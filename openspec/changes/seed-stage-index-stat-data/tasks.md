# Tasks

## 1. Baseline

- [x] 1.1 Time stage creation and a base move with the current build, on a generated repository of 30,000 tracked files and on a copy of the Core checkout, one command in flight: the first `status` of a new stage while another stage exists, and the next `status` of a stage after another stage committed. Count their `git` processes from Git's trace2 `start` events. Verify: the times, the counts, the unit, and the concurrency are recorded in this task.

  Baseline, 2026-11-14, master `b71acfb`, macOS 26.7, 12 cores, Git 2.54 (Apple Git-157) through `/usr/bin/git`, warm file cache. Each command runs as `git multistage`, one command in flight. Six rounds; each round creates a new stage with `status`, has stage `mover` stage and commit one file, then runs `status` in a stage with nothing staged and in a stage with one staged file. Wall time of one command: median (min to max). The process counts leave out the `git multistage` process itself.

  | Command | Core clone, 11,540 files | Generated, 30,000 files | git processes |
  | --- | --- | --- | --- |
  | First `status` of a new stage, another stage exists | 1,021 ms (909 to 1,289) | 1,028 ms (973 to 1,266) | 11 |
  | `status` after a base move, nothing staged | 981 ms (887 to 1,189) | 986 ms (946 to 1,443) | 9 |
  | `status` after a base move, one file staged | 1,028 ms (968 to 2,012) | 1,120 ms (1,019 to 1,694) | 20 |
  | `status` without a base move | 102 ms (92 to 122) | 70 ms (59 to 81) | 5 |

## 2. Index from a copy

- [x] 2.1 Add `fillIndex` and `statSeed`, and make `setIndexTree` in `internal/ms/stagearea.go` use them. Verify: end-to-end tests show that a base move and a new stage run `read-tree --reset` and give the stage index the entries of the tree with none of the seed's staged changes; that a racily clean entry of the seed whose file changed shows as unstaged in the new stage; that after `git sparse-checkout disable` a new stage and a moved base have no skip-worktree bit; that an assume-unchanged entry of the default index stays out of a new stage; that a stage whose index file is missing still gets a new index; and that `status` starts as many `git` processes as before. The stage, commit, sync, sparse, hook, crash, drop, and concurrency end-to-end tests pass.

  Result: `setIndexTree` builds each new generation through `fillIndex`, `statSeed`, and `resetCopy`, in the new file `internal/ms/fillindex.go`; `stagearea.go` only calls `fillIndex`. `resetCopy` checks the version of the copy, not of the seed, because another stage can replace its index between the check and the copy. A stage whose own index file is missing is reported as damaged before `setIndexTree` runs, so the fallback test removes the index of the other stage that a new stage starts from. Each new test fails when its rule is removed: the racy-entry test when `copyFile` drops the modification time, the sparse test without the version check, and the assume-unchanged test when the default index serves as the seed. The `internal` tests pass, and so do the end-to-end tests whose names match Stage, Commit, Sync, Sparse, Split, Hook, Crash, Kill, Drop, Many, Concurr, Damaged, Base, Refresh, Status, and Edit (52 s). The process counts are in 3.1.

## 3. Results

- [ ] 3.1 Measure as in 1.1 with the old and new builds, interleaved, and check that both builds give stage indexes with the same entries and bits. Run `mise run ci`. Verify: CI exits with code 0, and the counts and times are in this task.

## Workflow follow-up

- Archive the change after every task is done.
- Release a new patch version with the release binaries for Windows, macOS, and Linux.
