# Proposal

## Why

Each `git-multistage` command starts many `git` processes, one after another. On macOS, with the installed binary, one `status` in a small repository started 23 `git` processes and took 1.06 s, and `add` followed by `commit` started 65 and took 1.54 s (one command in flight, measured with a counting wrapper on `PATH`). The end-to-end suite runs its 177 tests one after another, so one full `mise run ci` takes 6 to 7 minutes, and most of it is the `test` package (308 s to 364 s in the last three runs). Agents run the suite several times in each round of work, and the same cost slows down every agent that uses the tool.

## What Changes

- Repository setup starts fewer `git` processes: one `rev-parse` call gives the top level, the Git directories, the prefix, and the object format; the version check, the empty tree ID, and the empty blob ID come from cheaper sources or are computed once.
- Commands read the index and trees with one `git` call where they now start one call for each path, such as `ls-files` per path in the hooks and `lsIndex` and `lsTree` per path in staging and integration.
- The end-to-end tests call `t.Parallel()`, except the tests that change the process environment or that need a quiet machine for timing.
- No behavior changes. Outputs, exit codes, and files on disk stay the same.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change alters no behavior, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `internal/gitx/gitx.go`: `Open`, and helpers that read many paths at once.
- `internal/ms/`: callers that loop over paths with one `git` call each.
- `test/`: `t.Parallel()` in the end-to-end tests, and helpers that read shared state.
- `mise run ci` and every command get faster. The measurements before and after go in `tasks.md`.
