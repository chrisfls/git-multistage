# Proposal

## Why

After `cheaper-git-calls`, each `git` process still costs about 4 to 5 ms of wall time on macOS, and most of that is the process start. A trace of every `git` process in one scenario on master `9cf4d2b` shows that many of them ask Git again for a result that an earlier process already gave:

- `status` starts 9 processes, a hooked edit 7 + 11, `add --edits` 13, and `commit` 22.
- `write-tree` runs on a stage index that did not change since the last command, in `status`, `diff --cached`, the hooked edit, `add`, and `commit`.
- `show` reads the keep commit at the end of every stage command, although only the stage itself writes that commit.
- `status` and `add <path>` start one `diff-index` for each other stage, so the count grows with the number of agents: 5 more processes with 5 other stages.
- `commit` reads the tip and HEAD again right after it moved the branch, diffs the commit that it just made, writes the trees of indexes whose trees it knows, and rebuilds a stage index that already holds the commit's tree.
- `add --edits` reads the same index entries twice and starts one `update-index` for each recorded edit.

A Go Git library does not remove these calls. The remaining calls write objects, read and write indexes, diff with filters, merge, run hooks, and move refs. The design of `cheaper-git-calls` measured go-git against the reads that it could take over, and rejected it.

## What Changes

- git-multistage remembers results by the checksum of what they came from, so a later command skips the `git` process when the input did not change:
  - the tree of an index, by the checksum at the end of the index file;
  - the staged paths of another stage, by the checksum of that stage's index and its base tree;
  - the parents and the input digest of a keep commit, by the commit ID. A stage reads its own keep ref from the loose ref file, in a repository that stores refs as files.
- `commit` reuses what it already knows: the new tip and HEAD after it moves the branch, the diff of its staged tree for the default index sync, the tree of the synced default index, and the stage index that already holds the committed tree.
- `add --edits` reads each index entry once, and writes all recorded edits to the index with one `update-index`.
- Outputs, exit codes, and the commits, refs, and indexes that commands write stay the same. New cache files go in `<git-dir>/multistage/`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. No requirement changes, so the change sets `skip_specs: true`.

## Impact

- `internal/gitx/gitx.go`: `Open` reads the ref format; the index checksum; the remembered index trees in `WriteTree`; the remembered diffs in `DiffTrees`.
- `internal/ms/stagearea.go`: `OpenRepo`, `protect`, `setIndexTree`, and a keep record in each stage directory.
- `internal/ms/stage.go`: `stageStagedPaths` and `lsIndexMany`.
- `internal/ms/commit.go` and `internal/ms/indexsync.go`: `prepareCommit`, `land`, `syncAfterCommit`, `indexTree`, and `applyIndexChanges`.
- `internal/ms/edits.go`: `gonePaths`, `indexView`, and `applyRecords`.
- Every stage command and the hooked edit start fewer `git` processes. The measurements before and after go in `tasks.md`.
