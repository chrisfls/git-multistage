# Design

## Context

The module has no Go benchmarks. Earlier performance changes measured a scenario of `git multistage` commands by its `git` processes, from Git's trace2 `start` events, and by the wall time of each command. On master `77b02b0` that scenario starts 79 processes: five other stages with one staged file each, then in stage `claude-w` two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, and `commit`. `test/gitcalls_test.go` checks the Git calls of several commands.

`go.mod` declares Go 1.22, so `strings.Lines`, `strings.SplitSeq`, `maps.Keys` with `slices.Sorted`, and range-over-func are not available.

## Goals / Non-Goals

**Goals:**
- Less code to read for the same behavior: one name for one concept, no dead code or dead branches, and no step that each caller must repeat.
- No more allocations, copies, Git processes, or file operations on any path, and fewer where a cleanup removes them.
- Remove a partial index copy that a failed write leaves behind.

**Non-Goals:**
- A change of output, error text, hint, exit code, JSON field, commit, ref, index, or Git call.
- A raise of the `go` directive.
- Changes to locking, the crash windows, or the order of Git calls. `filelock.Acquire`, `Drop`'s Windows release, and the stage-creation paths stay as they are.
- `indexTrees.put`. Its map check skips the scan of the order list for a new checksum, which `slices.Index` would not.
- A helper for every repeated shape. The three `tmp` directory setups, the "exit 1 means empty" shape of `ResolveRef` and `SymbolicHead`, and the hook's stderr prefix stay inline, because a helper would save a few lines and add a name to learn.

## Decisions

- **One index entry type.** `indexEntry` (stage.go) and `stagedEntry` (indexsync.go) are both `struct{ mode, oid string }`. Keep `indexEntry`, because the move, snapshot, and edit code use it, and delete `stagedEntry`. `readIndexEntries` then returns `map[string]indexEntry`, so `lsIndexMany` returns that map instead of copying it, and `lsIndex` returns the entry without a conversion. This saves one map allocation and one pass for each `lsIndexMany` call.
- **`otherStagesStaged` returns no error.** Its only error return is `nil`, with a `//nolint:nilerr`, and each read ignores its own error. Drop the error result and the error branches of its three callers: `refuseShared`, `Hunks`, and `Status`.
- **`emit` owns the warning reset.** Every emit that takes a `jsonWarnings` entry is followed by `a.warnings = nil`, so that `Main` does not print them again on a later error. `emit` sets the entry and then clears `a.warnings`, in the same branch. The emit of `init`, which takes no `jsonWarnings` entry, keeps its warnings, as now. `Main` calls `flushWarnings` once, before the JSON and text branches of its error path; both branches called it first.
- **`randomHex` for random names.** `hookIndexPath`, `writeRecord`, `renameCaseOnly`, and `NewStageID` each read 4 random bytes and hex-encode them, and ignore the read error. `randomHex()` in stagearea.go does that once. `commit.go` no longer imports `crypto/rand` as `crand` beside `math/rand`.
- **One table of Codex patch headers.** `codexHeader` ranges over a map literal, which Go builds on each call, once per patch line; `patchFiles` builds a three-element slice per line. A package-level array of prefix and kind serves `codexHeader`, and `patchFiles` calls `codexHeader`. The three prefixes cannot match one line together, so the order of the checks does not change the result. `patchFiles` keeps its rule that a header with an empty path names no file.
- **`copyFile` removes a partial destination.** After it creates the destination, a failed copy, close, or `Chtimes` removes the file before it returns the error. Every caller passes a new or temporary name, which it deletes or never adopts on an error, so the removal changes no adopted index. Without it, a failed copy left an `index-N` until `removeLeftovers`.
- **`hunks.Apply` writes into a `bytes.Buffer` grown to the old content's length,** and returns its bytes. The `strings.Builder` grew in steps and `[]byte(b.String())` copied the result. For an empty result the function now returns a nil slice instead of an empty one; every caller hands the bytes to an `io.Reader` or compares line counts, which treat both alike.
- **`Repo.run` copies the base environment only for a non-`User` call.** A `User` call replaced the copy with `userEnv()` at once.
- **Small cleanups with no change of flow.** Delete `gitx.Repo.Env`, which has no caller. `saveIndexTrees` ends with `writeCacheFile`, whose body it repeated. `TipAndHead` stops shadowing its named result `head` and stops giving `ok` two meanings. `snapshotBlob` checks `os.Lstat` once and uses a `switch`, and uses `diskPath`. `RemovePaths` and `MovePath` check the error and the pending index separately. `runEvent` uses `eventPreTool`, `eventPostTool`, and `eventPostFailure`. `runHook` builds its arguments once. `quotePath` drops `&& c != 0`, because `cEscapes` holds no NUL. `moveBase` declares `var e *Error`. `land` calls `syncMoved` directly with the comment that `syncLanded` held. `markStagedBy` names its inner set `stagedIDs`. The doc comment of `protect` moves from above `sparseState` to `protect`. `addPaths` sizes its JSON slice once.
- **Benchmarks.** `BenchmarkApply` applies three hunks to a 2,000-line file. `BenchmarkPatchFiles` and `BenchmarkCodexSection` parse a Codex patch of 20 files. Both run against a clean export of master `77b02b0` and the new tree, with `-benchmem -count=10`, compared with `benchstat` when it is installed.

## Risks / Trade-offs

- [The nil result of `hunks.Apply` for an empty file] → Every caller was read: `replayLines` checks line counts, and `setBlob`, `replayPath`, and `hash-object` read through `bytes.NewReader`. A unit test covers an empty result.
- [`hunks.Apply` grows its buffer to the old content's length, so a result that deletes every line allocates once where the old code allocated nothing] → Such a result is rare, and every other call saves 19 of 26 allocations in `BenchmarkApply`.
- [`emit` now clears warnings as a side effect] → The behavior is the same at every call site, and the doc comment of `emit` says so. The JSON tests with warnings cover it.
- [`copyFile` removes a file that a caller wanted] → Every one of its eleven callers passes a fresh or temporary name; none reads the destination after an error.
- [A refactor changes a Git call] → The scenario's trace2 counts must match the baseline for every command, and `test/gitcalls_test.go` must pass.
