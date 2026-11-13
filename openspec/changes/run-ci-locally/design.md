# Design

## mise tasks

`mise.toml` gains `[tasks.ci]` and `[tasks."ci:stress"]`. `ci` runs `golangci-lint fmt --diff ./...`, `golangci-lint run ./...`, `go vet ./...`, `go vet -tags testhooks ./...`, `go test -count=1 ./...`, and `openspec validate --all --strict` with `OPENSPEC_NO_UPDATE_CHECK=1`, stopping at the first failure. The commands are the ones that `ci.yml` ran, so the two cannot drift: the workflow's steps call `mise run ci`.

## Workflow

`ci.yml` replaces its `push` and `pull_request` triggers with `workflow_dispatch`. Its job keeps the three-system matrix and calls `mise run ci` on each, so a manual run costs what the old run cost, and nothing more. The stress step stays out of it: stress runs locally.

## Cost

A manual three-system run is about 15 minutes on macOS (10x), 10 on Windows (2x), and 5 on Linux: roughly $1.50. The old setup started that on every push.

## Risk

Windows and Linux regressions are found late. Mitigation: the release workflow cross-compiles all six targets, so a build break is caught on a tag, and the manual workflow is a command away.
