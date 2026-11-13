# Design

## Context

`hunks.Parse` gives each hunk an ID that hashes its path and changed lines. When several hunks of one file have the same lines, each copy also hashes a place: `@<NewStart>` and a versions string. `Stage.copyVersions` builds the versions from the stage's base commit and the blob of the new side. The unstaged listing diffs the stage index against a working-tree snapshot; the staged listing diffs the base blob against the stage index blob. Restoring a staged hunk rewrites the stage index blob, which is the new side of the staged diff, so both inputs of the copy ID change.

## Goals / Non-Goals

**Goals:**
- A staged copy's ID depends only on what restoring a copy leaves alone: the base commit, the base blob, and the copy's place in the base blob.

**Non-Goals:**
- Changing IDs of hunks that have no copy, or the unstaged rule.

## Decisions

- **Place a staged copy by `OldStart`.** In a staged diff, the base is the old side. Restoring a copy reverts lines of the index blob only, so every other copy keeps its `OldStart` in the base. `Parse` takes a parameter that selects the side; the unstaged listing keeps `NewStart`. Two hunks of one zero-context diff never share a start line on the old side with the same line count, because adjacent hunks merge, so `OldStart` separates copies as well as `NewStart` does.
- **Versions are the base commit and the base blob.** The base commit keeps the rule that a new base makes the IDs unknown, even when the file's base blob is the same. The base blob is implied by the base commit and the path, but naming it keeps the two listings symmetric and costs nothing.
- **Alternative rejected: hash the copy's ordinal among the copies.** Restoring the first copy would shift the ordinals of the others.

## Risks / Trade-offs

- [IDs of staged copies change once on upgrade] → An ID listed by the old binary is unknown to the new one; the command exits with code 4 and the user lists again, as for any stale ID.
- [`staged-by` compares staged IDs with unstaged IDs] → For copies these never matched before either, because the versions differ; no change.
