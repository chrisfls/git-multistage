# Design

## Context

- `editOnDisk` (`internal/ms/edits.go`) diffs the record's after-content with the disk. For a deletion it fails when a later hunk that adds lines touches the deletion point: an insertion right there, or a replacement whose old lines include the line before or after the point. 24052b5 added the replacement case so that a hunk that restores the deleted lines and changes a neighbour is caught; it also catches a hunk that only changes the neighbour.
- `markStagedBy` (`internal/ms/stage.go`) compares hunk IDs. A copy's ID hashes the stage's base commit and a blob that differs between an unstaged listing and another stage's staged listing, so copies never match.
- `gitx.Run` reruns a command whose stderr matches an English error text of Git for Windows. `internalEnv` keeps the caller's `LANG` and `LC_ALL`.

## Goals / Non-Goals

**Goals:**
- Each finding fixed with a test that fails without the fix.

**Non-Goals:**
- Changing hunk IDs.
- Translating Git output that streams to the user.

## Decisions

- **Deletion lost only when deleted lines come back.** At the deletion point, the check also requires that the later hunk adds at least one line equal to a deleted line. A hunk that only changes a neighbour adds none, so it passes; the restored-deletion cases of review3 add the deleted lines, so they stay refused.
- **Staged-by key: changed lines plus old start line.** `markStagedBy` keeps the ID match, and also matches a hunk with the same removed and added lines and the same `OldStart`. When both stages start from the same base and the listing stage has nothing staged before the hunk, the old sides are the same file, so `OldStart` names the same place. Otherwise a copy can miss its mark, which is the previous behavior. Hashing per-listing versions into this key is what broke it, so the key leaves them out.
- **`LC_ALL=C` for captured calls.** `run` appends `LC_ALL=C` when the call is internal (`!User`) and captures both outputs. Git then prints untranslated messages, so `lostObjectWrite`, `gitErr`'s prefix trims, and the `Binary files` and `git version` matches see the text they expect. Interactive and streamed calls, such as `add -p` and `diff`, keep the user's locale. A locale-free signal of the lost write does not exist: Git exits with 128 for every fatal error.
- **Windows check as a package variable.** `renamesObjects` defaults to `runtime.GOOS == "windows"`; the test sets it to true and puts a shell script named `git` first on `PATH`. The script counts its runs in a file, consumes stdin, and fails with the lost-write text until a given run. The tests run on Unix only, because the fake is a shell script.
- **Lease test without the testhooks tag.** A test built only without `testhooks` sets `GIT_MULTISTAGE_LEASE_TIMEOUT_MS=0`, holds a lease as another stage, and checks that `takeLease` still waits. Before 97b8e4c it returned at once.

## Risks / Trade-offs

- [Git's error messages that reach the user are in English] → They were already prefixed and reworded by git-multistage; the codes and kinds do not change.
