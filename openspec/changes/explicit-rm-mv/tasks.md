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

- [ ] 3.1 Make `add --edits` follow moves and report `path-deleted`, and add `--recreate`. Verify: the scenarios of "Recorded edits follow moves and deletions" and "Recreate a deleted file from its records" pass.
- [ ] 3.2 Record Codex `Delete File` and `Move to` as a deletion and a move, and stage them in `add --edits`. Verify: "Codex moves a file" passes with the `codex-*-apply-patch` fixtures extended by a `Move to` header.

## 4. Agent guide and checks

- [ ] 4.1 In `skills/git-multistage/SKILL.md`, tell agents to delete and move files with `rm` and `mv`, and add a `path-deleted` row with `--recreate` and `restore --staged`. Verify: the skill names `rm`, `mv`, and `path-deleted`.
- [ ] 4.2 Run the checks of the `git-multistage-development` skill and `mise run ci`. Verify: both pass, and `openspec validate explicit-rm-mv` passes.
