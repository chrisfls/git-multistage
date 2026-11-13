# Proposal

## Why

The ID of an identical hunk copy in a staged listing (`hunks --cached`) hashes the blob of the stage index. `restore --staged --hunk` changes that blob, so after one copy is restored, the IDs that the listing gave the other copies are unknown, and the user must list the hunks again. An unstaged listing keeps the IDs of the other copies when one copy is staged; a staged listing must do the same when one copy is restored.

## What Changes

- In a staged listing, the ID of an identical hunk copy hashes the stage's base commit, the blob of the file in the base, and the copy's start line in the base. Restoring one copy changes none of them, so the other copies keep their IDs. A new base still makes the IDs unknown.
- The unstaged listing keeps its rule: the stage's base commit, the blob of the working-tree snapshot, and the copy's start line in that snapshot.
- The requirement "Identical hunks get distinct IDs" in the staging spec states both rules, in place of "cover their positions".

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `staging`: the requirement "Identical hunks get distinct IDs" states what the ID of a copy hashes in each listing, and that restoring one staged copy keeps the IDs of the others.

## Impact

- `internal/hunks/hunks.go`: `Parse` can place a copy by its line on the old side of the diff.
- `internal/ms/stage.go`: `stagedHunks` passes the base versions and the old side.
- IDs of identical copies in `hunks --cached` output change once. IDs of hunks that have no copy do not change.
