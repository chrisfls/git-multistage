# Design

## Context

`gitx.Open` (`internal/gitx/gitx.go:106`) runs `git version`, `rev-parse` twice, and `hash-object` twice before any command does its work. Each `git` process costs tens of milliseconds on macOS. Many functions in `internal/ms` call `git` once for each path, through `lsIndex`, `lsTree`, `catBlob`, and `snapshotBlob`. The `test` package runs every test serially; `TestMain` builds one binary, and each test uses its own temporary repository.

## Goals / Non-Goals

**Goals:**
- Fewer `git` processes for each command, measured on `status`, `add`, `commit`, and `add --edits`.
- A shorter `go test ./...`, measured at the default `-parallel` value (GOMAXPROCS).

**Non-Goals:**
- Replacing `git` processes with a Go Git library. The module keeps no dependencies outside the standard library.
- Changing any output, exit code, or file layout.

## Decisions

- **Measure first, then cut the biggest costs.** A counting wrapper for `git` on `PATH` lists the calls of each command. The work starts with the calls that every command makes, then the per-path loops on the paths that the tests exercise most.
- **One `rev-parse` for setup.** `rev-parse --path-format=absolute --show-toplevel --git-dir --git-common-dir --show-prefix --show-object-format` gives everything that the two calls give now. The empty tree and empty blob IDs are fixed for each object format (SHA-1 and SHA-256), so they need no `hash-object`. The empty blob must still exist in the object database where the code writes it; the code writes it on first use, not at setup. The version check reads `git version` once per process.
- **Batch reads.** `ls-files -s -z -- <paths>` and `ls-tree -z <tree> -- <paths>` read many paths in one call, and `cat-file --batch` reads many blobs. Callers that loop over paths call the batch form.
- **Parallel tests.** Each test that does not call `t.Setenv`, `os.Setenv`, or `os.Chdir`, and does not measure time, calls `t.Parallel()` first. Shared package state in the helpers, such as the built binary path, is read-only after `TestMain`. The concurrency and crash tests keep their own timing, so they may stay serial.

## Risks / Trade-offs

- [Parallel tests expose shared state, such as a fixed path under the user's home or a global Git config] → `setTestEnv` already isolates `HOME` and the Git config for the process; tests that write outside their temporary directory stay serial.
- [A batched read changes error reporting for one bad path] → The batch callers keep the per-path errors and messages; tests that check those messages cover them.
- [Faster runs hide timing-dependent races] → `mise run ci:stress` keeps the repeated concurrency and crash runs.

## Migration Plan

None. No behavior or file changes.
