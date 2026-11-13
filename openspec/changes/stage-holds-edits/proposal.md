# Proposal

## Why

A stage keeps one agent's work apart from the others' work. Today it gives that work up when the shared working tree changes: `add --edits` fails with `edit-lost` when another writer overwrote the agent's lines or deleted the file, and refuses a record that is marked uncertain. To lower the chance of an uncertain record, the editor hook locks each file with a lease and makes other agents wait for up to 20 seconds.

A recorded edit already holds the agent's change. The stage can keep it whatever later happens to the file on disk, so it can serve as a backup of the agent's work. The hook can also read the agent's own change from the editor's tool input and response, so it needs no lock to keep another writer's change out of the record.

## What Changes

- **BREAKING** `add --edits` stages each recorded edit from the record alone. It no longer compares the record with the working tree. The kind `edit-lost` goes away: an edit stays stageable after another writer overwrites its lines or deletes the file.
- **BREAKING** The editor hook takes no file leases. Two agents' edits of one file never wait for each other, and the hook never blocks an edit with exit code 2.
- The hook records only the change that the tool call made. For Claude Code `Edit`, `MultiEdit`, and `Write` it takes the change from the original content and the patch in the tool response. For Codex `apply_patch` it applies the patch text to the snapshot taken before the call. A change that another writer makes to the file at the same time stays out of the record.
- Records are no longer uncertain. Existing records marked uncertain are staged like any other record.
- `edit-conflict` stays for a record that does not fit the stage's own index. An example is an edit of lines that another stage added and has not committed. The error tells the agent to commit after the other stage, or to stage the file with `add --hunk`.
- The agent guide and the README say that `add --edits` stages the recorded edits even when the file changed later on disk.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `editor-hooks`: remove the file leases, uncertain edits, and the `edit-lost` failure; record each edit from the tool's own change; stage records without reading the working tree; replace the requirement on a deletion next to a later change.

## Impact

- `internal/ms/hook.go`: remove the leases and their wait; build the record from the tool input and response.
- `internal/ms/edits.go`: remove `checkRecordsSafe`, `editOnDisk`, `overwrites`, and `addsBack`; keep the replay into the stage index.
- `internal/ms/lease_test.go` and the tests in `test/` that cover leases, uncertain records, and `edit-lost`.
- `skills/git-multistage/SKILL.md` and `README.md`: the `edit-lost` text.
- Agents no longer wait on each other's edits. A commit can now hold an edit that is not on disk anymore.
