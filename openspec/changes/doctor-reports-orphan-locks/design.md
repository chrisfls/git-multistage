# Design

## Context

A stage directory holds its index generations (`index-<n>`), the hook indexes of commits (`hook-index-<hex>`), and the stage lock file `lock`. Git writes `<index>.lock` beside an index while it changes it. `removeLeftovers` deletes a `.lock` file only when it is an hour old (`leftoverLockAge`), because a Git child of a killed process may still rename it. `Stages()` already tries the stage lock with no wait and records `Busy`.

## Goals / Non-Goals

**Goals:**
- `doctor` names every index lock file of an idle stage.

**Non-Goals:**
- Deleting the file. `doctor` changes no stage; `removeLeftovers` keeps its one-hour rule.

## Decisions

- **Match `index-*.lock` and `hook-index-*.lock`.** These are the only lock files that Git writes in a stage directory. The stage lock `lock` has no `.lock` suffix.
- **Skip a busy stage.** While a git-multistage command holds the stage lock, its own Git child may hold an index lock file; a warning about it would be wrong. An idle stage has no running command, so its lock files come from a Git child that outlived its command.
- **Warn, do not fail.** The next command of the stage works around the file, so the level is `warning`, like the leftover `index.lock` of the default index.

## Risks / Trade-offs

- [A command takes the stage lock right after `doctor` checked it] → The warning can name a lock file of a live Git child. The message says that a Git process may still be running.
