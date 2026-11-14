# Design

## Context

`setIndexTree` in `internal/ms/stagearea.go` makes each new index generation of a stage. Three paths call it: `create` for a new stage, `moveBase` when the stage's base moves to a new branch tip, and `finishCommit` and the `already-applied` result of `commit`. When the stage's current index holds the new tree already, as after a commit of the staged tree, `setIndexTree` copies that index and refreshes the copy. Otherwise it runs `git read-tree <tree>` into an empty file and then `git update-index -q --refresh`.

The refresh compares the stat data of each entry with the file. An entry from `read-tree` has zero stat data, so Git reads and hashes the file. Measured with Git 2.54 (Apple Git-157) on macOS, one command in flight:

| Repository | `read-tree <tree>` + refresh | `read-tree --reset <tree>` on a copy of an index with stat data + refresh |
| --- | --- | --- |
| Core, 11,540 files | 26 ms + 640 to 2,220 ms | 28 ms + 24 ms |
| Generated, 30,000 files | 23 ms + 880 to 5,210 ms | 37 ms (refresh only, after a commit) |

The larger refresh times come with a cold file cache.

Git facts that the design relies on, checked with Git 2.54:

- `read-tree --reset <tree>` is `read-tree -m` that discards unmerged entries. With one tree, Git keeps the entry of the old index, with its stat data and its bits, when its mode and object ID match the tree. Other entries come from the tree with zero stat data. An intent-to-add entry has no match in the tree and is dropped.
- Git writes index version 3 only when an entry has an extended bit (skip-worktree or intent-to-add). It writes version 2 otherwise, also when it read version 3. An assume-unchanged bit fits in version 2.
- An index file's modification time is Git's reference for racily clean entries. `copyFile` keeps the original's modification time. With a copy that keeps it, `read-tree --reset` and the refresh report a racily clean entry whose file changed; with a copy that has a later time, they miss it.

## Goals / Non-Goals

**Goals:**
- A new index generation of a stage refreshes with a stat check for each unchanged file, at stage creation and at every base move.
- The same entries and the same bits as an index read from the tree, and the same number of `git` processes.

**Non-Goals:**
- Stat data for the first stage of a worktree, for a stage in a sparse checkout, and for an index of version 3 or 4. They read the tree into an empty index, as now.
- A refresh of a stage index during other commands. Git's diffs of a stage index already hash only the files whose stat data changed.
- Writing index files in Go.

## Decisions

- **Fill the new generation from a copy, with `read-tree --reset`.** `fillIndex` copies the seed index to the new generation's file, runs `git read-tree --reset <tree>` on it, and refreshes it. The copy keeps the original's modification time, so Git's racy-entry checks see the copy as the original. When the remembered tree of the seed is the new tree, the copy needs no `read-tree`.
  - Rejected: `read-tree -m <tree>`. It fails on an index with unmerged entries, and `--reset` discards them as an index read from the tree would.
  - Rejected: `read-tree --reset` on the current index in place. The metadata rename is the commit point of a new generation; until it happens, the current index must stay as it is.
  - Rejected: copying stat data into an index file from Go. It writes index files outside Git.
- **The seed is a stage index, never the default index.** `statSeed` returns the stage's current index. For a new stage, it returns the current index of the other stage whose directory changed last: every metadata write renames `meta.json` into the stage directory, so the directory's modification time follows the stage's last write, and that stage's index has the newest stat data. `statSeed` reads the other stage's `meta.json` without its lock, as `status` reads other stages.
  - Rejected: the default index. A user's assume-unchanged or skip-worktree bit on an entry that matches the tree would stay in the stage index, and the stage would then ignore changes to the file. An index read from the tree has no such bit.
  - Rejected: any other stage. The stage that changed last costs one `stat` of each stage directory, which `ReadDir` returns, and its stat data are the freshest.
- **Only a version 2 seed, and none with `core.ignoreStat`.** `statSeed` reads the 8-byte header of the seed and returns "" unless its version is 2. A version 2 index holds no skip-worktree and no intent-to-add bit. git-multistage never marks an entry of a stage index assume-unchanged, but with `core.ignoreStat` Git marks the entries that it updates, so `statSeed` returns "" when `core.ignoreStat` is set. Every bit that a fresh index would get, such as the skip-worktree bits of a sparse checkout, `copySkipWorktree` still sets after the fill, as now.
  - Rejected: clearing the extra bits after the fill. It needs an `ls-files -t` of the new index and an `update-index` for the bits, in every fill, for a seed that only a sparse checkout or `index.version=4` produces.
- **A failed copy falls back to the empty index.** When the copy or `read-tree --reset` fails, as when the other stage replaced its index generation and deleted the file between the read of its metadata and the copy, `fillIndex` deletes the new file and reads the tree into an empty index, as now. An error of that path, such as a missing tree, is the command's error.
- **The known-tree copy stays.** When the stage's current index holds the new tree, `fillIndex` copies it and refreshes, for any index version, as `reuse-git-results` made it.

## Risks / Trade-offs

- [A copy of another stage's index on Windows holds the file open for about 1 ms. Go opens files without `FILE_SHARE_DELETE`, so the other stage's rename onto that file or its deletion of an old generation can fail in that time.] → Git for Windows retries a failed rename, git-multistage's `rename` retries on Windows, and `removeLeftovers` deletes an old generation that a failed delete left. `status` already has Git read the indexes of other stages.
- [Stat data in the seed that do not match the file, such as a file changed since the seed's last refresh.] → The refresh hashes those files, as it hashes every file now.
- [A seed whose remembered tree is the new tree but whose stat data are old.] → The refresh compares every entry with its file, so old stat data cost a hash, never a wrong entry.
