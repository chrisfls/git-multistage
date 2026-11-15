# Proposal

## Why

A review of `internal/gitx`, `internal/hunks`, and `internal/ms` found code that makes a reader do more work than the behavior needs. Two names describe one index entry, so `lsIndexMany` copies a whole map only to convert its values. One function returns an error that is always nil, so three callers handle an error that cannot occur. Thirteen JSON outputs repeat a line that resets the warnings by hand. Four places repeat the code for a random name, and two places repeat the headers of a Codex patch. Some code is dead or has dead conditions, a comment documents the wrong function, and a loop variable shadows a parameter. `copyFile` also leaves a partial index copy behind when a write fails.

## What Changes

- Merge the identical `indexEntry` and `stagedEntry` types. `lsIndexMany` and `lsIndex` return the entries without a copy.
- `otherStagesStaged` returns only its map. Its callers lose their dead error branches and the `//nolint:nilerr`.
- `emit` resets the warnings that it prints. The thirteen `a.warnings = nil` lines after an emit go away, and `Main` calls `flushWarnings` once on its error path.
- One `randomHex` helper names hook indexes, edit records, case-only move temporaries, and new stages.
- One table of Codex patch headers serves `codexHeader` and `patchFiles`. Neither builds a map or a slice for each patch line.
- `copyFile` removes its destination when a write after the create fails.
- Smaller cleanups: delete the unused `gitx.Repo.Env`; reuse `writeCacheFile` in `saveIndexTrees`; flatten `TipAndHead`, `snapshotBlob`, and the pending-disk checks of `rm` and `mv`; use the event constants in `runEvent`; build `runHook`'s arguments once; drop a dead condition in `quotePath`; drop the `syncLanded` wrapper; fix the shadowed `ids` in `markStagedBy`; move the doc comment of `protect` to its function.
- Fewer copies on two paths: `hunks.Apply` returns its buffer without a copy, `replayRecord` hands its bytes to Git without a string conversion, and a `User` Git call no longer copies the base environment that it replaces.
- Add benchmarks for `hunks.Apply` and the Codex patch parsers, so that the before and after builds can be compared.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change keeps every output, error, hint, exit code, JSON field, commit, ref, index, and Git call, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `internal/gitx/gitx.go`, `internal/gitx/indextree.go`, `internal/hunks/hunks.go`.
- `internal/ms`: `cli.go`, `commands.go`, `commit.go`, `edits.go`, `hook.go`, `hookinput.go`, `indexsync.go`, `integrate.go`, `move.go`, `ownchange.go`, `snapshot.go`, `stage.go`, `stagearea.go`.
- New benchmarks in `internal/hunks/bench_test.go` and `internal/ms/bench_test.go`.
- No exported behavior changes. `cmd/`, the plugins, the hooks, and the specs stay as they are.
