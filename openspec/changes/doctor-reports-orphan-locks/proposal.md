# Proposal

## Why

A Git child of a killed git-multistage can leave the lock file of a stage's index, such as `index-3.lock`, in the stage directory. Since e4cba35, the stage's next command moves to a new index generation, and the file is deleted only when it is an hour old. While the file exists, `doctor` reports every check as ok, so a user who investigates a slow or failed command does not see it.

## What Changes

- `doctor` warns about each index lock file in the directory of a stage that no git-multistage process holds, and names the stage and the file's path.
- A stage that a command holds is skipped: its Git child may hold the lock file now.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli`: a new requirement, "Doctor reports orphaned stage index locks".

## Impact

- `internal/ms/commands.go`: `Doctor` lists the lock files of each idle stage.
- `test/`: one end-to-end test.
