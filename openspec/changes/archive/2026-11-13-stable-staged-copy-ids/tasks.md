# Tasks

## 1. Tests first

- [x] 1.1 Add an end-to-end test that stages three identical hunks, lists them with `hunks --cached`, restores the first copy by ID, then restores the second copy by its listed ID; verify that it fails with exit code 4 before the fix
- [x] 1.2 Add an end-to-end test that lists staged copies, moves the stage to a new base that changes the file, and verifies that `restore --staged --hunk` with a listed copy ID exits with code 4

## 2. Hash the base for staged copies

- [x] 2.1 Let `hunks.Parse` place copies by the old side of the diff, and make `stagedHunks` pass the base commit, the base blob, and the old side; verify that the tests of group 1 and `go test -count=1 ./...` pass
- [x] 2.2 Correct the requirement "Identical hunks get distinct IDs" in `openspec/specs/staging/spec.md` through this change's delta, and verify that `openspec validate --all --strict` passes

## Workflow follow-up

- Archive the change with `openspec-archive-change` after every task is done.
