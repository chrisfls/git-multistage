# Proposal

## Why

A gap review of the recent fixes found five problems:

1. `add --edits` refuses a deletion edit with `edit-lost` when another agent later changes the line just above or below it, although the deletion is still on disk (from 24052b5).
2. An unstaged listing no longer marks identical copies that another stage staged with `staged-by`, because the IDs of copies hash per-listing versions that differ between the two listings.
3. The rerun after a lost loose-object write on Windows matches Git's English error text, but internal Git calls inherit `LANG` and `LC_ALL`, so a localized Git never reruns.
4. The rerun loop of `gitx.Run` has no test.
5. No test fails if `GIT_MULTISTAGE_LEASE_TIMEOUT_MS` is read again in release builds (97b8e4c).

Finding 2 belongs with stable-staged-copy-ids, which was archived before the review arrived, so it is fixed here.

## What Changes

- A deletion edit counts as lost only when a later change at its point adds back one of the deleted lines. The existing cases where a later change restores the deleted lines stay refused.
- `staged-by` also marks an identical copy when another stage staged the same changed lines at the same start line of the old content.
- Internal Git calls whose output git-multistage reads run with `LC_ALL=C`. Git commands that stream to the user, and hooks, keep the user's locale.
- Tests for the rerun loop of `gitx.Run`, through a fake `git` on `PATH`, and for the lease timeout of release builds.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `editor-hooks`: a new requirement, "Deletion next to a later change".
- `staging`: a new requirement, "Staged-by for identical copies".

## Impact

- `internal/ms/edits.go`: `editOnDisk`.
- `internal/ms/stage.go`: `markStagedBy`.
- `internal/gitx/gitx.go`: the environment of internal calls, and the Windows check as a package variable for tests.
- Git's error messages that git-multistage passes on, such as in a failed `commit`, are in English.
- New tests in `internal/gitx`, `internal/ms`, and `test/`.
