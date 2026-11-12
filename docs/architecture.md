# Architecture

git-multistage is a Go program that runs Git subprocesses. It stores no objects of its own and implements no diff, merge, or index format. Every repository change goes through Git plumbing.

## Concepts

| Term | Meaning |
| --- | --- |
| Stage | One staging area, selected by its stage ID: a private index file plus metadata. |
| Base | The commit that a stage's index was built from. Staged changes are the difference between the base tree and the stage's index. |
| Target branch | The branch that HEAD pointed to when the stage was created. The stage commits only to it. |
| Tip | The commit that the target branch points to now. |
| Default index | The worktree's ordinary index, `.git/index` (or the worktree's own index in a linked worktree). |

## Storage

```text
<per-worktree git dir>/multistage/
  stages/<stage-id>/
    lock            OS lock file; held while a command uses the stage
    meta.json       base, target branch, current index file name, pending commit, commit list
    index-<n>       the stage's index (generation n)
    COMMIT_EDITMSG  the message of the last commit, for hooks
  index-sync        the branch, commit, and index tree of the last default-index sync
  tmp/              temporary index files

refs/worktree/multistage/<stage-id>   per-worktree ref to a commit of the staged tree
```

Staged blobs and trees are referenced only by the stage's index file, which `git gc` does not read. While a stage has staged changes, the per-worktree ref points to a commit of the staged tree (parent: the base), so garbage collection keeps those objects. The ref is deleted when nothing is staged. `git log --all` lists it while it exists.

The per-worktree Git directory comes from `git rev-parse --path-format=absolute --git-dir`: `.git` for the main worktree, `.git/worktrees/<name>` for a linked one. Each worktree therefore has its own stages, and the same stage ID in two worktrees names two separate stages. A repository without linked worktrees has a single set of stages in `.git/multistage`. Git never tracks files inside the Git directory.

Stage IDs are lowercase, so two IDs cannot collide on a case-insensitive file system. IDs that Windows reserves (`con`, `nul`, `com1`, ...) are refused.

### Atomic updates

- `meta.json` is replaced through a temporary file, `fsync`, and a rename.
- When a stage needs a new index, such as after a rebase onto a newer tip, the tool writes `index-<n+1>` first, then renames the new `meta.json` into place. The metadata rename is the commit point. A crash before it leaves the old generation current; a crash after it leaves the new one current. Unreferenced generations are deleted on the next command.
- Staging commands (`git add`, `git update-index`, `git reset`, `git apply --cached`) change the current index file in place through Git's own lock-and-rename.

### Locks

| Lock | Holder | Scope and duration |
| --- | --- | --- |
| `stages/<id>/lock` | git-multistage | One stage, for one command. An OS lock (`flock`, `LockFileEx`), so the operating system releases it when the process dies. |
| `.git/index.lock` | Git and git-multistage | The default index. git-multistage holds it only around the branch update and the default-index sync, which take milliseconds. |
| `<ref>.lock` | Git | One branch, inside `git update-ref`. |

No lock covers the commits of all stages. Commits build their trees and commit objects without any shared lock, and they race only at the compare-and-swap of the branch.

## Staging

`add <path>` runs `git add` on a copy of the stage's index (the next generation), so ignore rules, attributes, filters, deletions, and binary files behave as in Git. Before the copy becomes current, the command checks other stages of the worktree: when another stage has staged changes in the same file, the whole file may contain that stage's edits, so the command discards the copy and refuses (exit code 5). `--allow-shared` overrides the check. A crash before the metadata rename leaves the previous index current.

`add <path> --hunk <id>`:

1. Hash the working-tree file once with `git hash-object -w --path=<path>`, which applies the clean conversions (line endings). Every later step uses this blob, so a concurrent write to the file cannot mix two versions.
2. Diff the stage's blob against the snapshot with `git diff -U0 --diff-algorithm=myers <old-blob> <new-blob>`.
3. Give each hunk an ID: the first 12 hex digits of SHA-256 over the path and the hunk's removed and added lines. When several hunks of one file have the same lines, their IDs also hash the number of copies, their positions, and the nearest unchanged old lines above and below them. When lines above the copies are added or deleted, each copy gets another position or other neighbors, so an old ID becomes unknown instead of moving to another copy.
4. Check that every requested ID matches exactly one hunk. Otherwise fail with exit code 4 and change nothing. An untracked file that `.gitignore` ignores needs `-f`, as with `git add`.
5. Apply the chosen hunks to the old blob content, store the result with `git hash-object -w --stdin`, and set the index entry with `git update-index --cacheinfo`.

`restore --staged --hunk` uses the same machinery between the base blob and the staged blob: it keeps every staged hunk except the chosen ones.

Files with a `filter` attribute (such as Git LFS), symbolic links, submodules, binary files, and deletions have no hunks; they are staged whole.

## Recorded edits

The editor hook runs before and after each file edit: `git multistage hook claude` for Claude Code's `Edit`, `MultiEdit`, and `Write`, and `git multistage hook codex` for Codex's `apply_patch`, whose files come from the patch headers. A multi-file edit leases its files in a fixed order.

1. Before the edit, it takes a lease on the file: `leases/<hash>.json` in the multistage directory names the stage, a nonce, and an expiry 15 seconds ahead, written under a short OS lock. Another stage's hook waits until the lease is released or expires, up to 20 seconds, then blocks its edit with exit code 2. It then hashes the file into a blob and writes a pending record keyed by the tool call ID.
2. After the edit, it hashes the file again and writes an edit record with the before and after blobs into `stages/<id>/edits/`, one file per edit. If the lease changed hands meanwhile, the record is marked uncertain. It then releases the lease.

`add --edits` applies the records, oldest first, to a copy of the stage's index:

- Records whose last edit of a file is no longer on disk (a later change touched the lines that the edit added), and uncertain records, are refused, and nothing is staged.
- When the index entry equals the record's before blob, the after blob becomes the entry.
- Otherwise the edit's hunks (before to after) are placed on the index content by the line replay below. Other writers' changes in the same file stay out, even on the neighbouring lines.

The copy becomes current only when every record applies; then the records are deleted. The metadata save that makes the copy current also lists the applied records, so after a crash in between, the next command that opens the stage deletes them instead of applying them again.

## Line replay

Git's merge conflicts on changes to adjacent lines. When a merge onto the tip reports only content conflicts, each conflicted file is replayed instead: every staged hunk is placed at the tip's position of its lines, which succeeds only when the tip left those lines unchanged and no insertion competes for the same point. The replayed tree passes the same verification as a merge. Recorded edits use the same replay.

## Keeping a stage current

Before most commands, the stage is refreshed. When the tip has moved past the stage's base:

- With nothing staged, the stage's index becomes the tip's tree.
- With staged changes, the change from the base tree to the stage's tree is replayed onto the tip's tree with `git merge-tree --write-tree --merge-base=<base-tree> <tip-tree> <stage-tree>`, then verified as described below. On success the merged tree becomes the new index generation and the tip becomes the base. On a conflict the stage stays as it is, and `status` reports it as stale.

A refreshed stage shows work that other stages committed as part of its base, so `status` and `hunks` show only uncommitted work.

## The commit transaction

`commit` turns the staged change into a commit on the current tip.

1. Refuse when HEAD is not on the target branch, or when a merge, rebase, cherry-pick, or revert is in progress.
2. Read the staged tree `S` and the base tree `B`. Fail when `S == B` (nothing staged).
3. Prepare the message: write it to `COMMIT_EDITMSG`, run `prepare-commit-msg`, clean it with `git stripspace`, run `commit-msg` (unless `--no-verify`), and clean it again.
4. Loop, at most 100 attempts:
   1. Read the tip `T`.
   2. If `T` is the base, the result tree `R` is `S`. Otherwise `R` is the three-way merge of `B -> S` onto `T`'s tree. A conflict ends the commit with exit code 3, and the stage keeps its changes.
   3. Verify `R` (see below).
   4. If `R` equals `T`'s tree, every staged change is already on the branch: clear the stage and report `already-applied`.
   5. Run `pre-commit` (unless `--no-verify`) with `GIT_INDEX_FILE` pointing to a temporary index that holds `R`. If the hook changes that index, the commit fails and the stage keeps its changes: a hook that re-adds files reads them from the shared working tree, where unstaged edits that belong to other stages may be. A retry that introduces exactly the changes that the hook already accepted skips the hook.
   6. Create the commit with `git commit-tree R -p T` (plus `-S` when `commit.gpgSign` is set). Author and committer come from Git's configuration and environment.
   7. Write the pending commit `{commit, parent}` into `meta.json`.
   8. Take `.git/index.lock` (wait up to 15 s). Check again that no merge, rebase, cherry-pick, or revert started.
   9. Run `git update-ref -m <reflog message> <branch> <commit> <T>`. Git moves the branch only if it still points to `T`.
   10. On failure, release the lock and read the branch again. If it moved, clear the pending commit, wait a short random time, and go back to step 1. A retry replays the same staged change onto the new tip; it never reuses the old tree.
   11. On success, sync the default index (below) and release the lock.
   12. Record the commit in the stage, make the commit the new base, reset the stage's index to its tree, and clear the pending commit.
   13. Run `post-commit`.

### Verification of the merged tree

Before any tree becomes a commit or a refreshed stage, the tool checks it against the staged intent:

- The paths where `R` differs from the tip must be a subset of the staged paths, plus the new names of staged paths that the tip renamed. This blocks any tree that would revert or include changes that were not staged.
- For each staged path that the tip did not change, `R` must hold exactly the staged mode and blob.

A tree that fails either check is refused with exit code 3. A clean merge does not prove semantic correctness; the check proves that the commit carries the staged edits and nothing else.

### Crash recovery

The pending commit in `meta.json` is the journal. At the start of every command, a stage with a pending commit checks whether that commit is reachable from the branch:

- Reachable: the branch update happened. The stage records the commit, clears its staged changes, and syncs the default index.
- Not reachable, or pruned by garbage collection: the update did not happen. The stage clears the pending record and keeps its staged changes.

A stage whose metadata names a missing index file is refused by every command except `drop`, because Git would read the missing index as an empty one and the next commit would delete every file. `drop` renames the stage directory before deleting it, so an interrupted drop cannot leave such a stage.

Either way the change ends up in exactly one commit. A killed staging command leaves either the old or the new index, because Git writes indexes through lock-and-rename; leftover lock files and temporary files are removed when the stage lock is next taken.

## The default index

`git update-ref` does not touch any index. Without a sync, `git status` after a stage's commit would show the commit's changes as staged reversals, and a person's next `git commit` would undo them.

After each successful branch update, while still holding `.git/index.lock`:

1. List the paths that differ between the commit's parent `P` and the new tip `H`.
2. For each path, read the default index entry:
   - It equals the version in `P`: replace it with the version in `H` (or remove it).
   - It already equals `H`: leave it.
   - Anything else, including conflict stages: a person staged something there. Keep it and print a warning.
3. Write the result to a copy of the index with `git update-index --index-info` and rename the copy over the index.

Every stage moves the branch only while holding `.git/index.lock`, and `git commit` holds the same lock while it reads the index and moves the branch. So the default index matches `P` when the sync starts, and a person's commit can never start between a stage's branch update and the sync. Only a tool that moves the branch without updating the index (a plain `git update-ref`, or a stage with `multistage.syncIndex=false`) leaves the index behind; the sync then updates only the committing stage's paths, and the rest stays behind until a manual sync.

`git multistage sync` needs to know which commit the index matches. The sync record stores the commit and the tree of the default index after each sync whose starting point was known. The record is valid only while the index still has that tree, so any native change to the index entries invalidates it. Without a valid record, `sync` needs `--from <commit>`.

The sync never touches the working tree. Files on disk already hold the committed edits, so after a sync `git status` shows only uncommitted work.

## Invariants

1. A stage's index is written only by commands that hold that stage's lock.
2. A branch moves only through `git update-ref` with the expected old value.
3. A commit's tree differs from its parent only at the paths that its stage had staged (or their renamed paths), and holds the staged content wherever the parent did not change the path. A hook cannot change it.
4. A failed, conflicting, or interrupted commit leaves the stage's staged changes in place, or, if the branch update happened, records the commit exactly once.
5. A default-index entry changes only when it holds the previously synced version.
6. git-multistage never writes the working tree. Hooks may.
