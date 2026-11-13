# Tasks

## 1. Report orphaned stage index locks

- [x] 1.1 Add an end-to-end test that leaves `index-<gen>.lock` in an idle stage, runs `doctor`, and checks that the output names the stage and the path and that the file still exists; verify that it fails before the change
- [x] 1.2 Make `Doctor` warn about each `index-*.lock` and `hook-index-*.lock` file of each stage that is not busy; verify that the test of 1.1 and `go test -count=1 ./...` pass

## Workflow follow-up

- Archive the change with `openspec-archive-change` after every task is done.
