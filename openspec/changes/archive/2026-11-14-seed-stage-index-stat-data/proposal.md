# Proposal

## Why

A stage builds a new index generation with `git read-tree <tree>` and then `git update-index -q --refresh`. An index from `read-tree` has no stat data, so the refresh reads and hashes every tracked file of the worktree. On the 11,540-file Core repository, the refresh takes 640 to 2,220 ms, while `read-tree` takes 26 ms. On a generated repository of 30,000 files, it takes 0.9 s with a warm file cache and 3.7 to 5.2 s with a cold one.

This cost lands where agents wait for it:

- The first hooked edit of each agent session creates the agent's stage inside the PostToolUse hook.
- Every commit in a shared checkout moves the branch, and the next command of each other stage moves its base to the new tip. With N agents, one commit causes N - 1 full refreshes.

An index that already has stat data for the same worktree needs only a stat check for each file. On Core, `read-tree --reset` on a copy of such an index and the refresh take 28 ms and 24 ms.

## What Changes

- A new index generation of a stage starts from a copy of a stage index that has stat data: the stage's current index, or, for a new stage, the current index of the other stage of the worktree that changed last. `git read-tree --reset <tree>` on the copy keeps the stat data of each entry whose mode and object ID match the tree, so the refresh hashes only the files of the other entries.
- Only a version 2 index serves as the copy. Git writes version 2 only when no entry has a skip-worktree or intent-to-add bit, and git-multistage never marks a stage index entry assume-unchanged, so the new index has no bit that an index read from the tree would lack. The default index never serves as the copy, because it can hold a user's assume-unchanged and skip-worktree bits.
- Without such an index, as for the first stage of a worktree or a stage in a sparse checkout, the stage reads the tree into an empty index, as now.
- The number of `git` processes stays the same.
- No behavior changes. Outputs, exit codes, commits, refs, and the entries and bits of every index stay the same.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. No requirement changes, so the change sets `skip_specs: true`.

## Impact

- `internal/ms/stagearea.go`: `setIndexTree` builds the new index generation through `fillIndex`.
- New `internal/ms/fillindex.go`: `fillIndex`, `statSeed`, `newestStageIndex`, `resetCopy`, and `indexV2` choose the copy, check its version, and fill it.
- New unit tests in `internal/ms/fillindex_test.go` and end-to-end tests in `test/seed_test.go`.
- Stage creation and every base move hash only the files whose entries differ from the copy. The measurements before and after go in `tasks.md`.
