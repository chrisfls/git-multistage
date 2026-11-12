# Architecture

git-multistage is a Go program that runs Git subprocesses. It stores no objects of its own and implements no diff, merge, or index format. Every repository change goes through Git plumbing.

## Concepts

| Term | Meaning |
| --- | --- |
| Stage | One agent's staging area: a private index file plus metadata. |
| Base | The commit that a stage's index was built from. Staged changes are the difference between the base tree and the stage's index. |
| Target branch | The branch that HEAD pointed to when the stage was created. The stage commits only to it. |
| Tip | The commit that the target branch points to now. |
| Default index | The worktree's ordinary index, `.git/index` (or the worktree's own index in a linked worktree). |

## Storage

```text
<per-worktree git dir>/multistage/
  stages/<agent-id>/
    lock            OS lock file; held while a command uses the stage
    meta.json       base, target branch, current index file name, pending commit, commit list
    index-<n>       the stage's index (generation n)
    COMMIT_EDITMSG  the message of the last commit, for hooks
  index-sync        the branch and commit that the default index was last synced to
  tmp/              temporary index files
```

The per-worktree Git directory comes from `git rev-parse --path-format=absolute --git-dir`: `.git` for the main worktree, `.git/worktrees/<name>` for a linked one. Each worktree therefore has its own stages, and the same agent ID in two worktrees names two separate stages. Git never tracks files inside the Git directory.

Agent IDs are lowercase, so two IDs cannot collide on a case-insensitive file system. IDs that Windows reserves (`con`, `nul`, `com1`, ...) are refused.

### Atomic updates

- `meta.json` is replaced through a temporary file, `fsync`, and a rename.
- When a stage needs a new index, such as after a rebase onto a newer tip, the tool writes `index-<n+1>` first, then renames the new `meta.json` into place. The metadata rename is the commit point. A crash before it leaves the old generation current; a crash after it leaves the new one current. Unreferenced generations are deleted on the next command.
- Staging commands (`git add`, `git update-index`, `git reset`, `git apply --cached`) change the current index file in place through Git's own lock-and-rename.

### Locks

| Lock | Holder | Scope and duration |
| --- | --- | --- |
| `stages/<id>/lock` | git-multistage | One agent's stage, for one command. An OS lock (`flock`, `LockFileEx`), so the operating system releases it when the process dies. |
| `.git/index.lock` | Git and git-multistage | The default index. git-multistage holds it only around the branch update and the default-index sync, which take milliseconds. |
| `<ref>.lock` | Git | One branch, inside `git update-ref`. |

No lock covers all agents' commits. Commits build their trees and commit objects without any shared lock, and they race only at the compare-and-swap of the branch.

## Staging

`add <path>` runs `git add` with `GIT_INDEX_FILE` set to the stage's index, so ignore rules, attributes, filters, deletions, and binary files behave as in Git. Before it keeps the result, it checks other stages of the worktree: when another agent has staged changes in the same file, the whole file may contain that agent's edits, so the command restores the previous index and refuses (exit code 5). `--allow-shared` overrides the check.

`add <path> --hunk <id>`:

1. Hash the working-tree file once with `git hash-object -w --path=<path>`, which applies the clean conversions (line endings). Every later step uses this blob, so a concurrent write to the file cannot mix two versions.
2. Diff the stage's blob against the snapshot with `git diff -U0 --diff-algorithm=myers <old-blob> <new-blob>`.
3. Give each hunk an ID: the first 12 hex digits of SHA-256 over the path and the hunk's removed and added lines. Identical hunks in one file get `-2`, `-3`, in order.
4. Check that every requested ID matches exactly one hunk. Otherwise fail with exit code 4 and change nothing.
5. Apply the chosen hunks to the old blob content, store the result with `git hash-object -w --stdin`, and set the index entry with `git update-index --cacheinfo`.

`restore --staged --hunk` uses the same machinery between the base blob and the staged blob: it keeps every staged hunk except the chosen ones.

Files with a `filter` attribute (such as Git LFS), symbolic links, submodules, binary files, and deletions have no hunks; they are staged whole.

## Keeping a stage current

Before most commands, the stage is refreshed. When the tip has moved past the stage's base:

- With nothing staged, the stage's index becomes the tip's tree.
- With staged changes, the change from the base tree to the stage's tree is replayed onto the tip's tree with `git merge-tree --write-tree --merge-base=<base-tree> <tip-tree> <stage-tree>`, then verified as described below. On success the merged tree becomes the new index generation and the tip becomes the base. On a conflict the stage stays as it is, and `status` reports it as stale.

A refreshed stage shows other agents' committed work as part of its base, so `status` and `hunks` show only the agent's own uncommitted work and other agents' uncommitted work.

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
   5. Run `pre-commit` (unless `--no-verify`) with `GIT_INDEX_FILE` pointing to a temporary index that holds `R`. If the hook changes that index, the changed tree must still touch only staged paths.
   6. Create the commit with `git commit-tree R -p T` (plus `-S` when `commit.gpgSign` is set). Author and committer come from Git's configuration and environment.
   7. Write the pending commit `{commit, parent}` into `meta.json`.
   8. Take `.git/index.lock` (wait up to 15 s).
   9. Run `git update-ref -m <reflog message> <branch> <commit> <T>`. Git moves the branch only if it still points to `T`.
   10. On failure, release the lock and read the branch again. If it moved, clear the pending commit, wait a short random time, and go back to step 1. A retry replays the same staged change onto the new tip; it never reuses the old tree.
   11. On success, sync the default index (below) and release the lock.
   12. Record the commit in the stage, make the commit the new base, reset the stage's index to its tree, and clear the pending commit.
   13. Run `post-commit`.

### Verification of the merged tree

Before any tree becomes a commit or a refreshed stage, the tool checks it against the staged intent:

- The paths where `R` differs from the tip must be a subset of the staged paths, plus the new names of staged paths that the tip renamed. This blocks any tree that would revert or include changes that the agent did not stage.
- For each staged path that the tip did not change, `R` must hold exactly the staged mode and blob.

A tree that fails either check is refused with exit code 3. A clean merge does not prove semantic correctness; the check proves that the commit carries the staged edits and nothing else.

### Crash recovery

The pending commit in `meta.json` is the journal. At the start of every command, a stage with a pending commit checks whether that commit is reachable from the branch:

- Reachable: the branch update happened. The stage records the commit, clears its staged changes, and syncs the default index.
- Not reachable: the update did not happen. The stage clears the pending record and keeps its staged changes.

Either way the change ends up in exactly one commit. A killed staging command leaves either the old or the new index, because Git writes indexes through lock-and-rename; leftover lock files and temporary files are removed when the stage lock is next taken.

## The default index

`git update-ref` does not touch any index. Without a sync, `git status` after an agent's commit would show the commit's changes as staged reversals, and a person's next `git commit` would undo them.

After each successful branch update, while still holding `.git/index.lock`:

1. Read the sync record `S` (the commit that the default index was last brought to). If it is missing or not an ancestor of the tip, use the commit's parent.
2. List the paths that differ between `S` and the tip `H`.
3. For each path, read the default index entry:
   - It equals the version in `S`: replace it with the version in `H` (or remove it).
   - It already equals `H`: leave it.
   - Anything else, including conflict stages: a person staged something there. Keep it and print a warning.
4. Write the result to a copy of the index with `git update-index --index-info`, rename the copy over the index, and record `H`.

Holding `.git/index.lock` across the branch update and the sync matters: `git commit` holds the same lock while it reads the index and moves the branch, so a person's commit can never start between an agent's branch update and the sync.

The sync never touches the working tree. Files on disk already hold the agents' edits, so after a sync `git status` shows only uncommitted work.

## Invariants

1. A stage's index is written only by commands that hold that stage's lock.
2. A branch moves only through `git update-ref` with the expected old value.
3. A commit's tree differs from its parent only at the paths that its stage had staged (or their renamed paths), and holds the staged content wherever the parent did not change the path.
4. A failed, conflicting, or interrupted commit leaves the stage's staged changes in place, or, if the branch update happened, records the commit exactly once.
5. A default-index entry changes only when it holds the previously synced version.
6. The working tree is never written.
