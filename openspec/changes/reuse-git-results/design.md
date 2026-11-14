# Design

## Context

Git's trace2 events (`GIT_TRACE2_EVENT`) listed every `git` process of one scenario on master `9cf4d2b`: five other stages with one staged file each, then in stage `claude-w` two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, and `commit`. Inside Git, each process ran for 0.5 to 3.6 ms; the starts of two processes that run one after the other are 4 to 6 ms apart. The calls that this change removes:

| Calls | Where | What the process already knows |
| --- | --- | --- |
| `write-tree` of the stage index | `protect` of every stage command, `prepareCommit`, `add`, `drop` | The previous command wrote the tree of the same index bytes |
| `show -s` of `refs/worktree/multistage/<id>` | `protect` of every stage command | The stage wrote that keep commit itself |
| `diff-index --cached --name-only` of each other stage | `status` with staged changes, `add <path>` | An earlier command read the same index of that stage |
| `rev-parse <branch>^{commit} --symbolic-full-name HEAD --` after `update-ref` | `syncAfterCommit` in `commit` | `land` checked HEAD and moved the branch to the commit under `index.lock` |
| `write-tree` of a copy of the default index | `validRecord` in `commit` | The last sync, or the creation of the sync record, wrote the tree of the same index bytes |
| `diff-tree` of the parent and the commit | `syncIndexLocked` in `commit` | The staged diff of `prepareCommit`, when the commit's tree is the staged tree |
| `write-tree` of the synced default index | `applyIndexChanges` in `commit` | The index matched the parent's tree, and every change of the commit applied |
| `read-tree` of the committed tree | `finishCommit` in `commit` | The stage index already holds that tree |
| The second `ls-files -s`, and one `update-index` for each record | `add --edits` | The copy of the index has the bytes that `gonePaths` read |

The keep refs live in `refs/worktree/`. With the `files` ref storage, Git stores a per-worktree ref as a loose file in the worktree's Git directory and never packs it (checked with Git 2.54: `git pack-refs --all` leaves `refs/worktree/multistage/<id>` loose, in the main worktree and in a linked one). `git rev-parse --show-ref-format` exists since Git 2.44, so the minimum Git 2.45 has it.

## Goals / Non-Goals

**Goals:**
- Fewer `git` processes in every stage command and in the hooked edit, with the same outputs, exit codes, commits, refs, and indexes.

**Non-Goals:**
- A Go Git library. See the decision below.
- Code that writes Git objects, indexes, or refs, or that computes an object ID.
- A change to when the hooked edit updates the keep ref. The crash-recovery spec requires the update after each recorded edit.
- The `config` read and the `--exec-path` call of each process, and the order of the reads. They do not repeat a result.
- The waits, the file flushes, and the commit turns. The change `faster-waits-and-writes` covers them.

## Decisions

- **Key each remembered result by a checksum of its input.** Each cache maps a content checksum to a fact that Git derives from that content alone, so an entry never goes stale: a changed input has another checksum and misses the cache. A lost, old, or damaged cache file only makes a command start the `git` process, as now. The cache files are written by a rename of a temporary file, without a flush, and a failed write is ignored.
  - Rejected: invalidating a cache when a command writes the index. Every writer of an index would have to remember it, and one missed writer would commit a wrong tree.
  - Rejected: the size and modification time of the index file. A file system with coarse timestamps can keep them for two different contents.
- **The index checksum.** An index file ends with the hash of its content: 20 bytes in a SHA-1 repository and 32 in a SHA-256 one. `gitx` reads these bytes and uses their hex as the checksum. With `index.skipHash` (also set by `feature.manyFiles`), the bytes are zero, and the index has no checksum, so nothing is remembered for it.
- **Remember the tree of each index.** `WriteTree` reads the index checksum first, and returns the remembered tree when there is one. Otherwise it runs `write-tree`, then remembers the tree under the checksum from before the call and under the one after it: `write-tree` rewrites the index when it updates the cache-tree extension (checked with Git 2.54, also with `GIT_OPTIONAL_LOCKS=0`).
  - The trees live in memory for the process and in `<git-dir>/multistage/index-trees`, one `<checksum> <tree>` line each, newest last, at most 64 lines. One file serves every index of the worktree, because the same bytes give the same tree.
  - `WriteTree` may remember only when no other process writes the index during the call. This holds for each call: a stage index under the stage lock, the private copies of the hooks, of `indexTree`, and of the sync, and the temporary indexes of the merges.
  - `indexTree` reads the default index's checksum before it copies the index, so an unchanged default index needs neither the copy nor `write-tree`.
  - When git-multistage knows the tree of an index that it wrote, it remembers the tree without `write-tree`: after `read-tree` of a tree and the refresh in `setIndexTree`, and after the sync derives the synced tree (below).
  - A remembered tree can name a tree object that `git gc` pruned. The staged tree of a stage with staged changes is reachable from its keep ref, and a stage without staged changes has its base tree. Elsewhere, the trees of the default index are only compared.
- **Remember the staged paths of other stages.** `stageStagedPaths` reads the stage's index checksum, then looks in `<git-dir>/multistage/staged-paths/<id>` for that checksum and the stage's base tree. On a miss it runs `diff-index`, as now. When the checksum is the same after the call and the metadata still names the same index, it writes the file: `<checksum> <base-tree>` on the first line, then the paths, each ended by NUL. A reader writes the file of another stage, outside that stage's directory, so the stage's own cleanup never meets a reader's temporary file. `drop` deletes the file of its stage.
- **Remember the keep commit, and read the keep ref from its file.** After `protect` writes a keep commit, it writes `<stage-dir>/keep`: the commit ID, its parents, and its `keep-input` line. The next `protect` reads the ref from `<git-dir>/refs/worktree/multistage/<id>` when `Open` reported the `files` ref storage:
  - a missing file means no keep ref;
  - a file whose object ID equals the commit of the keep record gives the parents and the digest without `show`;
  - anything else, and the `reftable` storage, runs `show` as now. `show` now prints the commit ID too (`%H %P%n%B`), so `protect` writes the keep record also for a keep commit of an older version.
  - The keep record states facts about an immutable commit, and the ref file says which commit the ref names now, so a person's change of the ref is still seen.
  - `Open` asks for the ref format with `--show-ref-format` in its `rev-parse`, so this adds no process.
  - Rejected: trusting the keep record without the ref file. A deleted ref would stay deleted until the stage changes.
- **Remember the diffs of trees in the process.** `DiffTrees` resolves a commit to its tree when the process knows the tree (`RememberCommit`, the base tree of the metadata), and keeps each result by its two trees and the rename setting. A request without renames is answered from a kept result with renames: each rename becomes a deletion of the old path and an addition of the new one, and the entries are sorted by path, as `diff-tree --no-renames` prints them in a recursive diff.
  - `prepareCommit` lists the staged changes with `DiffTrees(baseTree, staged, renames)` in place of `diff-index --cached -M` on the stage index. A stage index holds no unmerged or intent-to-add entries, so both give the same changes.
  - The sync after the commit then diffs the parent and the commit from the kept result when the parent is the stage's base and the commit's tree is the staged tree.
- **The sync after a commit uses the commit and the branch that `land` used.** `land` holds `index.lock`, checked HEAD under it, and moved the branch to the commit by compare-and-swap. `syncLanded(branch, commit, parent)` starts the sync with them; `recover` still reads the tip and HEAD, through `syncAfterCommit`. The default-index-sync spec moves each entry from the parent's version to the commit's version, which this gives exactly.
- **Derive the tree of the synced default index.** When the remembered tree of the default index equals the parent's tree, no entry is kept, and every change of the commit was written, the synced index holds the commit's tree. `applyIndexChanges` then remembers that tree under the new index's checksum and skips `write-tree`.
  - An intent-to-add entry is left out of the index's tree. When the commit changes its path, the entry matches neither version, so the sync keeps it, and the derivation does not apply. Entries at other paths stay as they were, in both trees.
- **Start a new stage index from a copy when it holds the tree already.** `setIndexTree` copies the current index and runs only `update-index -q --refresh` when the remembered tree of the current index is the new tree. After a commit whose tree is the staged tree, this replaces `read-tree` and the refresh with the refresh alone. The refresh keeps the stat data that later diffs rely on.
- **`add --edits` reads once and writes once.** `gonePaths` reads the entries of every path of the chosen records with its one `ls-files`. `lsIndexMany` keeps the entries it read in the process by index checksum, so the view of the index copy, which has the same bytes, reads no path twice. The view keeps the records' writes and answers later reads from them. When every record applies, one `update-index -z --index-info` writes them in order; a deletion is a line with mode 0, as `--force-remove` does.
- **The commit's first attempt still reads the tip.** Taking the tip of `prepareCommit` would save one more call, but `faster-waits-and-writes` reads the tip inside the commit turn.
- **No Go Git library.** The design of `cheaper-git-calls` checked go-git v5.19.3 and v6.0.0-beta.1 against Git 2.54 repositories: it cannot read `refs/worktree/` refs from a linked worktree, does not follow config includes, fails on split and sparse indexes and on reftable, and hashes CRLF files unlike `git hash-object --path`. This change did not repeat the check. libgit2 needs cgo, which the release builds for six platforms without. The calls left after this change write objects, read and write indexes, diff with filters, merge, run hooks, and move refs.

## Risks / Trade-offs

- [A second process writes an index while `WriteTree` runs, and the remembered tree belongs to other bytes] → Each caller holds the stage lock or owns a private copy; the default index is copied first. The design lists the callers, and a new caller must keep the rule.
- [A cache file grows] → `index-trees` keeps 64 lines; `staged-paths/<id>` holds one stage's paths, and `drop` deletes it; `keep` is three lines.
- [A per-worktree ref in a later Git version gets packed, so a missing loose file no longer means a missing ref] → The keep ref would look missing; `protect` would write a new keep commit when the stage has staged changes, and it would skip deleting a ref that a stage without staged changes no longer needs. Only `--show-ref-format` `files` enables the read, and the unit test checks the loose file after `git pack-refs --all`.
- [Two processes write `index-trees` at once, and one entry is lost] → The next `write-tree` of that index remembers it again.
- [The kept diff without renames differs from `diff-tree --no-renames`] → A unit test compares them for renames, a move into a directory, deletions, additions, and mode changes.

## Migration Plan

None. Older versions ignore the new files. A keep commit of an older version gets a keep record at the next `protect`.
