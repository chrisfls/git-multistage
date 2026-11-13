# Tasks

## 1. Refactor the flagged functions

- [x] 1.1 Refactor the flagged functions of `internal/gitx` and `internal/hunks`; verify that the full checks pass
- [x] 1.2 Refactor the flagged functions of `internal/ms/cli.go`; verify that the full checks pass
- [x] 1.3 Refactor the flagged functions of `internal/ms/commands.go`, `commit.go`, and `integrate.go`; verify that the full checks pass
- [ ] 1.4 Refactor the flagged functions of `internal/ms/edits.go`, `replay.go`, and `hook.go`; verify that the full checks pass
- [ ] 1.5 Refactor the flagged functions of `internal/ms/stage.go`, `stagearea.go`, `indexsync.go`, `snapshot.go`, and `procs.go`; verify that the full checks pass
- [ ] 1.6 Refactor the flagged tests; verify that the full checks pass

## 2. Enable the linters

- [ ] 2.1 Enable gocyclo, nestif, funlen, dupl, and goconst in `.golangci.yml` with the recommended thresholds, exclude goconst from `_test.go` files, and replace repeated strings with constants; verify that `golangci-lint run ./...` reports 0 issues and the full checks pass

## Workflow follow-up

- Archive the change with `openspec-archive-change` after every task is done.
