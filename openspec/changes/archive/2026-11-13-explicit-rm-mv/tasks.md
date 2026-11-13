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
- [x] 5.4 Apply each commit's trailers as one simultaneous step, move only entries that the base holds, and quote unusual trailer paths. Verify: "Several moves in one commit" and "Path with an arrow" pass, and a stage's new file at a trailer's old path stays in place.
- [x] 5.5 Report kind `conflict` when a staged move's old path was moved on the tip, and `path-deleted` for a staged move of a path the tip deleted. Verify: "Two names for one file" passes, and a staged move of a deleted file names the deleting commit.
- [x] 5.6 Make `mv` refuse an existing target at the rename itself, and rename between two spellings of one path on a case-insensitive file system. Verify: "Target exists" and "Case-only rename" pass on macOS.
- [x] 5.7 Run the checks of the `git-multistage-development` skill and `mise run ci`. Verify: both pass, and `openspec validate explicit-rm-mv` passes.

## 6. Second review fixes

- [x] 6.1 Journal the raw bytes of each path (`hash-object --no-filters`) and compare them at the finish. Verify: "File with CRLF line endings" passes for `rm` and `mv` under `core.autocrlf=true` and under `* text=auto`.
- [x] 6.2 Keep one journal entry per command, honour an entry only while the stage's index still shows its change, and make the first `mv` roll the stage back when the target appeared after the check. Verify: "Rerun after a restore" passes for `mv` and `rm`, a crashed `rm a.go` finishes after an `rm b.go`, and a target that appears after the check leaves the stage unchanged.
- [x] 6.3 Map followed record paths through the stage's own moves and deletions, and follow tip-less records through the commits dated after them. Verify: "Own move after a followed move" passes, `rm -f` of a followed path does not block other records, and a record without a tip follows a pure rename.
- [x] 6.4 Rename case-only paths in one call on macOS, and journal the temporary name elsewhere. Verify: "Case-only rename" passes, and a rerun after a stop between the two renames finishes the move.
- [x] 6.5 Quote trailer paths that hold `->` or start or end with whitespace. Verify: "Path with surrounding spaces" passes, and paths ending in ` ->` round-trip.
- [x] 6.7 Write no trailer for a move whose new path the commit recreates. Verify: "Recreated move" passes.
- [x] 6.6 Run the checks of the `git-multistage-development` skill and `mise run ci`. Verify: both pass, and `openspec validate --all --strict` passes.

## 7. Final review fixes

- [x] 7.1 Check only the renames of paths that the staged change modifies or moves, follow a path when any commit in base..tip moved it, and name the commit that removed it. Verify: with one staged file and a tip commit that renames 300 others, one refresh (1 in flight) takes under 1 s; a delete, re-add, then `git mv` of a staged path lands at the new path; the `path-deleted` message names the removing commit.
- [x] 7.2 Make `rm` drop the records that follow a move to a deleted path, and report `edit-conflict` for a record of a path that no commit removed, also on an unborn branch. Verify: "Edit of another stage's new file" passes, and records of a followed path deleted with `rm` and then committed do not block `add --edits`.
- [x] 7.3 Match journal entries case-insensitively when `core.ignoreCase` is set, compare a symbolic link by its link text, and never drop or roll back a journal entry with a temporary name before the file left that name. Verify: a crash rerun of `rm readme.md` for `README.md` finishes; `rm` of an unchanged symbolic link needs no `-f`; a case-only `mv` stopped between its renames, then `restore --staged` and `mv` again, leaves no temporary file.
- [x] 7.4 Run the checks of the `git-multistage-development` skill and `mise run ci`. Verify: both pass, and `openspec validate --all --strict` passes.

## 8. Last review fixes

- [x] 8.1 Make a staged deletion or move follow a rename without a trailer, as "Follow moves onto the tip" requires. Verify: a staged `rm p.go`, then a branch commit `git mv p.go n.go` and a later rewrite of `n.go`, gives a modify/delete conflict instead of an empty stage; without the rewrite, the deletion of `n.go` is committed.
- [x] 8.2 Make `rm` forget records that follow another stage's move and then the stage's own `mv` to the deleted path. Verify: record `x.go`, another stage commits `mv x.go y.go`, the stage runs `mv y.go z.go`, `rm -f z.go`, and commits; a later `add --edits` stages the other records.
- [x] 8.3 Run `mise run ci` once. Verify: exit code 0.
