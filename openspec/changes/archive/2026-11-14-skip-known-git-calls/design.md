# Design

## Context

A counting wrapper for `git` on `PATH` logged every call of one scenario: five other stages with one staged file each, then in stage `w` two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, and `commit`. On master `b316b7e`, `commit` started 35 processes. These calls give an answer that the process already has or can read on disk:

| Calls in one `commit` | Answer already known |
| --- | --- |
| `hook run` for `prepare-commit-msg`, `commit-msg`, `pre-commit`, `post-commit` | No hook file and no `hook.*` setting exists |
| `diff-tree`, `read-tree`, and `write-tree` around `pre-commit` | With no hook, the temporary index keeps its tree |
| The second `stripspace` | With no `commit-msg` hook, the message is already clean |
| `rev-parse <base>^{tree}` | The tree of the base is the same in every command |
| `rev-parse <new commit>^{tree}` and `merge-base --is-ancestor <parent> <new commit>` | `commit-tree` was given the tree and the parent |

In each hooked PostToolUse, `protect` starts `rev-parse <base>^{tree}` and two `mktree` calls, one for the edits' blobs and one for the keep tree that holds it.

## Goals / Non-Goals

**Goals:**
- Fewer `git` processes in `commit`, the PostToolUse hook, and every command that reads its base tree, with the same outputs and exit codes.

**Non-Goals:**
- A Go Git library. The reasons in the design of `fewer-git-processes` still hold. The calls removed here need none.
- Code that reads or writes Git objects, indexes, or refs.
- Caching an index's tree, a ref, or `HEAD`, which another process or an in-place write can change.

## Decisions

- **A hook is absent when Git would find nothing to run.** `HasHook(name)` in `gitx` reports true when any setting of the `hook` section exists, when the settings could not be read, or when the hooks directory holds an entry named `<name>` or `<name>.exe` of any kind, executable or not. The hooks directory is `core.hooksPath`, or `<common dir>/hooks` without it. A relative `core.hooksPath` resolves from the worktree top, where `git hook run` looks it up. A check with Git 2.54 confirmed this from a subdirectory. A hooks path that is a file, such as `/dev/null`, a common way to turn hooks off, holds no hook: the lookup fails with "not a directory", as Git's `access()` does. A value without `=`, an empty value, a value that starts with `~` or `%(`, and a rooted value without a drive report true, so `git hook run` decides. Any doubt runs the hook as before.
- **Skip the work around an absent hook.** `runHook` returns at once for an absent hook. `verifyTree` returns at once when `pre-commit` is absent, because `read-tree` and `write-tree` on a fresh index give back the tree they read. `prepareMessage` writes, runs, and cleans the message after `commit-msg` only when that hook exists. `stripspace` is idempotent, so the skipped second cleanup would not change the message.
- **The base tree in `meta.json`.** `Meta` gains `base_tree`. `save` writes the tree of `Base` when the tree cache of the process holds it, and leaves the field out otherwise. Loading a stage's metadata seeds the tree cache with the pair, and so does reading another stage's metadata in `stageStagedPaths`. A commit's tree never changes, so the pair stays true as long as `Base` does. A `meta.json` without the field, such as one written by an older version, costs one `rev-parse`, as now. An older version that rewrites `meta.json` drops the field, so a stale pair cannot remain.
- **Remember what `commit-tree` was given.** `commitTree` seeds the tree cache with the new commit's tree and records its parent. `IsAncestor(a, b)` returns true without `merge-base` when a recorded parent of `b` is `a`. Any other pair runs `merge-base` as now.
- **A flat keep tree.** The keep tree holds `staged`, the staged tree, beside one blob entry for each recorded edit's before and after content, named `<n>-before` and `<n>-after`. One `mktree` writes it. The crash-recovery requirement asks only that the tree hold the staged tree and the blobs. A keep ref of the old layout differs from the new tree, so the next `protect` writes it again once.
- **Rejected: one `update-index` for all records of `add --edits`.** `--index-info` sets `--replace`, so a recorded new file would silently replace a directory of the same name in the stage. Many `--cacheinfo` arguments in one call would report a failure without naming its record.
- **Rejected: skip the per-process `git version`.** It runs beside the setup `rev-parse`, so it adds no wait. A cached result would need a key for the Git binary, and on macOS `/usr/bin/git` is a stub whose file does not change when the active Git changes.
- **Rejected: compute the keep tree's ID in Go to compare with the keep ref.** It would save the `mktree` in a `protect` that changes nothing, but it implements Git's tree object format.

## Risks / Trade-offs

- [A hook source that `HasHook` does not know, in a later Git] → `HasHook` treats any `hook.*` setting as a hook, and it checks the same directory that Git 2.54 checks. The end-to-end tests of hooks run against the Git on `PATH`.
- [A wrong `base_tree` after a hand edit of `meta.json`] → Only `git-multistage` writes the file, and the pair is written together from one process's cache.
- [The keep ref of an older version] → The new layout replaces it at the next `protect`, and both layouts keep the same objects reachable.

## Migration Plan

None. Old `meta.json` files and old keep refs keep working and are updated by the next command that saves or protects the stage.
