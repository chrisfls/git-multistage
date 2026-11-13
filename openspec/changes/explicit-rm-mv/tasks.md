# Tasks

## 1. Explicit deletions and moves

- [x] 1.1 Make `add` refuse a missing path and a rename pair, with exit code 1 and a hint at `rm` and `mv`. Verify: the "Missing file" scenario passes, and existing tests that staged deletions through `add` move to `rm`.
- [x] 1.2 Add `rm <path>... [-f]`: stage the deletion, then delete the file. Accept a file already gone from disk. Need `-f` when the disk differs from the stage. Verify: the three "Delete files" scenarios pass with `--json` too.
- [x] 1.3 Add `Meta.Moves` and `mv <old> <new>`: stage the move with the staged blob, rewrite pending records of `<old>`, then move the file. Handle the "already made on disk" case. `restore --staged` drops the move. Verify: the three "Move files" scenarios and "Move writes only its paths" pass.

## 2. Commits follow moves and report deletions

- [x] 2.1 Write one `Multistage-Moved` trailer for each staged move, and clear `Moves` when the commit lands. Verify: the "Commit a move" scenario passes.
- [x] 2.2 In `integrate`, read the trailers of base..tip, rename the matching entries of the base and staged trees, then merge. Verify: "Edit after another stage's move" passes, with a heavy edit that Git's rename detection alone misses, and a chain of two moves passes too.
- [x] 2.3 Detect `path-deleted` before the merge, naming the deleting commit, and add `commit --recreate <path>...`. Verify: "Another stage deleted the file" and "Recreate the file" pass, and `commit --recreate` is accepted by the option check.
- [x] 2.4 Make the refresh keep the stage on `path-deleted` and name the deleted paths in its warning. Verify: "Refresh follows a move" and "Refresh meets a deletion" pass.

## 3. Recorded edits

- [x] 3.1 Make `add --edits` follow moves and report `path-deleted`, and add `--recreate`. Verify: the scenarios of "Recorded edits follow moves and deletions" and "Recreate a deleted file from its records" pass.
- [x] 3.2 Record Codex `Delete File` and `Move to` as a deletion and a move, and stage them in `add --edits`. Verify: "Codex moves a file" passes with the `codex-*-apply-patch` fixtures extended by a `Move to` header.

## 4. Agent guide and checks

- [x] 4.1 In `skills/git-multistage/SKILL.md`, tell agents to delete and move files with `rm` and `mv`, and add a `path-deleted` row with `--recreate` and `restore --staged`. Verify: the skill names `rm`, `mv`, and `path-deleted`.
- [x] 4.2 Run the checks of the `git-multistage-development` skill and `mise run ci`. Verify: both pass, and `openspec validate explicit-rm-mv` passes.

## 5. Review fixes

- [x] 5.1 Work out where each recorded path went once and apply it to every record of the path; `--recreate` stages the newest record and drops the older ones. Verify: "Several edits after another stage's move" passes, and a recreate test with two records passes.
- [x] 5.2 Store the branch tip in each record and follow `<record tip>..<branch>`, with trailers first and then Git's rename detection. Verify: "Edit after a plain Git rename" and "Move commit with an older date" pass.
- [x] 5.3 Add the `Meta.PendingDisk` journal for `rm` and `mv`, remove the base-tree fallback of `rm`, and make `rm` drop the records of the paths it deletes. Verify: "Path the stage does not hold", "Recorded edits of a deleted file", and "Crash during a chained move" pass, and the crash reruns of `rm` of an edited file and of a new file finish.
- [ ] 5.4 Apply each commit's trailers as one simultaneous step, move only entries that the base holds, and quote unusual trailer paths. Verify: "Several moves in one commit" and "Path with an arrow" pass, and a stage's new file at a trailer's old path stays in place.
- [ ] 5.5 Report kind `conflict` when a staged move's old path was moved on the tip, and `path-deleted` for a staged move of a path the tip deleted. Verify: "Two names for one file" passes, and a staged move of a deleted file names the deleting commit.
- [ ] 5.6 Make `mv` refuse an existing target at the rename itself, and rename between two spellings of one path on a case-insensitive file system. Verify: "Target exists" and "Case-only rename" pass on macOS.
- [ ] 5.7 Run the checks of the `git-multistage-development` skill and `mise run ci`. Verify: both pass, and `openspec validate explicit-rm-mv` passes.
