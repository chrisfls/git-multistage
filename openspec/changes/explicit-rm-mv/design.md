# Design

## Context

`integrate` (`internal/ms/integrate.go:13`) replays a stage onto a newer tip with a three-way `MergeTrees(baseTree, tipTree, staged)`, then checks the result with `verifyIntegration`. Git's rename detection inside that merge is the only way moves are followed today. A tip deletion of a staged path is a modify/delete conflict, reported as kind `conflict`. `AddPaths` (`internal/ms/stage.go:184`) stages deletions and renames like `git add`. Edit records (`internal/ms/edits.go`) carry a path, and `After == ""` means a deletion. This change assumes `stage-holds-edits` is applied: `add --edits` reads no disk state. See proposal.md for the motivation.

## Goals / Non-Goals

**Goals:**
- A deletion or a move enters a stage only through `rm`, `mv`, or a Codex patch header.
- A stage's change lands at the path where the branch moved the file, even when the move came with heavy edits.

**Non-Goals:**
- Directory moves (`mv dir/ other/`). `mv` takes one file. Moving a directory is several `mv` calls.
- Reading `Multistage-Moved` trailers from commits outside the base..tip range.

## Decisions

- **Moves live in the stage metadata.** `Meta` gets `Moves []Move` (old, new). `mv` updates the stage index (remove old, add new with the staged blob of old) and appends the move. `restore --staged` of either path drops the move. `commit` turns the moves into trailers and clears them when the commit lands. *Alternative:* infer moves from the staged D+A pairs. Rejected: that brings back the guessing the change removes.
- **Rewrite paths before the merge.** Before `MergeTrees`, `integrate` reads `git log --format=%(trailers:key=Multistage-Moved,valueonly) base..tip`, oldest first, and builds an old→new map that follows chains. It renames those entries in `baseTree` and `staged` so all three trees share the tip's paths, then merges. The rename detection of the merge still covers commits without trailers. *Alternative:* `merge-tree` rename hints. Rejected: Git has no option to supply renames.
- **`path-deleted` is detected before the merge.** For each path that the staged change modifies, after the move rewrite: if `baseTree` holds it and `tipTree` does not, the deleting commit is `git log -1 --diff-filter=D --format=%H base..tip -- <path>`. Any such path gives `path-deleted` with all the paths and commits. `--recreate <path>` removes the path from `baseTree` before the merge, so the staged version merges as an addition. If the tip has re-added the path, that addition conflicts as usual.
- **Refresh shares the logic.** `moveBase` calls `integrate`, so the refresh before other commands follows moves. On `path-deleted` it keeps the stage and adds the deleted paths to the "base is behind" warning.
- **Edit records follow the stage index.** `mv` rewrites the `Path` of pending records of `<old>`. In `add --edits`, a record whose path is missing from the refreshed index is checked against the trailer map since the record's time. A moved path is applied at the new path. A deleted path gives `path-deleted`, and `--recreate` stages the newest record's `After` as a new file and applies later records on top.
- **Codex headers become operations.** `Delete File` gives a record with `After == ""`, applied like `rm` without touching the disk. `Update File` plus `Move to` gives a move record and an edit record at the new path. `add --edits` appends the move to `Meta.Moves`.
- **`rm` and `mv` write the disk last.** The stage index and metadata are adopted first. The disk operation follows, so a crash between them leaves a stage that claims a move the disk does not show yet. Running the command again finishes it: the "already made on disk" path, or the "already gone" path. *Alternative:* disk first. Rejected: a crash would then leave the disk changed with no record of who changed it.
- **`rm -f` and unstaged changes.** `rm` compares the disk with the stage's index entry, as `git rm` does. A difference needs `-f`, because the disk may hold another agent's unstaged lines.

## Risks / Trade-offs

- [`rm` deletes a file that holds another agent's unstaged edits] → `rm` needs `-f` when the disk differs from the stage. The other agent's records keep its edits, and it gets `path-deleted` with `--recreate`.
- [A stage committed with the previous version used `add` for a deletion] → Existing staged deletions and renames stay staged and commit as before. Only new `add` calls refuse them.
- [Trailer parsing cost on long base..tip ranges] → One `git log` per replay, limited to the range.
- [A move of a file that the tip also moved somewhere else] → The merge reports a rename/rename conflict with kind `conflict`, as today.

## Migration Plan

Stages with staged deletions or renames from `add` keep them. `Meta.Moves` is optional, so old metadata reads unchanged. Rollback to the previous release ignores `Moves`: staged moves then commit as a deletion plus an addition, with no trailers.
