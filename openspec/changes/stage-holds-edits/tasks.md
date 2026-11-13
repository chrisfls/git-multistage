# Tasks

## 1. Stage records without the disk check

- [ ] 1.1 Remove `checkRecordsSafe`, `editOnDisk`, `overwrites`, and `addsBack` from `internal/ms/edits.go`, ignore `Uncertain`, and change the `edit-conflict` hint to name uncommitted lines of another stage. Verify: tests for "Edit overwritten on disk", "File deleted on disk", and an old record with `uncertain: true` pass; the tests that expected `edit-lost` are rewritten to expect the staged edit.
- [ ] 1.2 Add tests for "Edit of lines that another stage has not committed" (fails with `edit-conflict`, records kept) and "Edit of lines that another stage then committed" (stages after the other commit). Verify: both pass with `go test ./...`.

## 2. Record the tool call's own change without leases

- [ ] 2.1 Read `tool_response.originalFile`, `structuredPatch`, and `content` for Claude Code `Edit`, `MultiEdit`, and `Write`, and build the record from them, with the `PreToolUse` snapshot as fallback. Update the fixtures in `test/testdata/hooks/` with the transcript fields. Verify: a test changes another line of the file between the PreToolUse and PostToolUse events and the record holds only the tool's change.
- [ ] 2.2 Apply the Codex `apply_patch` hunks to the `PreToolUse` snapshot in memory, with the disk after the call as fallback. Verify: the "Another writer changes the file during a Codex patch" scenario passes, and a patch that does not match falls back.
- [ ] 2.3 Remove the leases, the wait, `TookOver`, and exit code 2 from `internal/ms/hook.go`, keep `PostToolUseFailure` deleting the pending record, and delete `internal/ms/lease_test.go` and the lease tests in `test/`. Verify: "Two agents edit one file at the same time" passes with neither hook waiting, and `go test -race ./...` passes.

## 3. Agent guide and README

- [ ] 3.1 Replace the `edit-lost` row in `skills/git-multistage/SKILL.md` with an `edit-conflict` row: the edit changes lines that another stage has not committed; commit after it, or use `add --hunk`. Say that `add --edits` stages what the agent wrote even when the file changed later. Verify: `grep -rn edit-lost skills README.md` prints nothing.
- [ ] 3.2 Rewrite the `add --edits` paragraph in the README "Coding agents" section to match. Verify: the paragraph names no lease and no `edit-lost`.

## 4. Checks

- [ ] 4.1 Run the checks of the `git-multistage-development` skill and `mise run ci`. Verify: both pass, and `openspec validate stage-holds-edits` passes.
