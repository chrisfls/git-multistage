# Tasks

## 1. Deletion next to a later change

- [x] 1.1 Port `TestGapDeletionNextToNeighbourEdit` to `test/`, and require at the deletion point that the later hunk adds back a deleted line; verify that the test fails before and passes after, and that the restored-deletion tests of `test/review3_test.go` still pass

## 2. Staged-by for identical copies

- [x] 2.1 Port `TestGapStagedByForCopies` to `test/`, and let `markStagedBy` also match the changed lines at the same old start line; verify that the test fails before and passes after

## 3. Untranslated internal Git output

- [x] 3.1 Run internal Git calls that capture their output with `LC_ALL=C`; add a `gitx` test with a fake `git` on `PATH` that prints its `LC_ALL`, under `LC_ALL=de_DE.UTF-8`, and check `C` for a captured call and the caller's value for `User` and streamed calls; verify that `go test -count=1 ./...` passes

## 4. Rerun loop tests

- [x] 4.1 Make the Windows check of `gitx.Run` a package variable, and add tests with a fake `git` on `PATH` for the stdin rewind, the cap of 5 reruns, and no rerun for `User`, streamed output, and stdin that cannot be rewound; verify with `go test -count=1 ./internal/gitx/`

## 5. Lease timeout of release builds

- [x] 5.1 Add an `internal/ms` test, built without `testhooks`, that sets `GIT_MULTISTAGE_LEASE_TIMEOUT_MS=0` and checks that `takeLease` still waits for a held lease; verify that it fails when the read of the variable is put back in `takeLease`

## Workflow follow-up

- Archive the change with `openspec-archive-change` after every task is done.
