# Design

## Context

Today, `PreToolUse` takes a 15-second lease on each file and snapshots it. `PostToolUse` snapshots it again and records the pair (`internal/ms/hook.go`). The two snapshots read the shared working tree, so only the lease keeps another writer's change out of the pair. `add --edits` first runs `checkRecordsSafe`, which compares each path's newest record with the working tree (`editOnDisk`, `overwrites`, `addsBack` in `internal/ms/edits.go`). It then replays the records into a copy of the stage index (`applyRecord`, `replayRecord`). See proposal.md for the motivation.

## Goals / Non-Goals

**Goals:**
- A record's Before→After diff is exactly the tool call's change, with no lock.
- `add --edits` reads only the records and the stage index.

**Non-Goals:**
- Shell edits stay unrecorded.
- Staging the records that fit and skipping the rest. `add --edits` stays all-or-nothing, because later records of a path build on earlier ones.
- Pulling another stage's uncommitted lines into a stage. That case stays `edit-conflict`.

## Decisions

- **Claude Code: Before and After from the tool response.** `tool_response.originalFile` is the content that the tool read. After is `originalFile` with `structuredPatch` applied. For `Write`, After is `tool_input.content`. Both are hashed with `hash-object -w` and use the same line-ending cleaning as the current snapshot. Claude Code transcripts from 2026-11 hold `originalFile` and `structuredPatch` in every `Edit` and `Write` result, including `Write` with `type: create`. *Alternative:* reverse-apply the patch to the file on disk after the call. Rejected: another writer may have changed the patch's lines by then.
- **Fallback when `originalFile` is missing or null.** For a `Write` create, Before is empty. Otherwise Before is the `PreToolUse` snapshot. Without the lease, that snapshot can hold another writer's change from the short gap before the tool reads the file. The record still diffs from that snapshot to the tool's own After, so another writer's change during the write itself stays out.
- **Codex: After from the patch text.** Before is the `PreToolUse` snapshot. After is that snapshot with the `apply_patch` hunks applied in memory, matched by their context lines. If the hunks do not match the snapshot, the hook falls back to the file on disk after the call, as today. The hook keeps parsing `Add File`, `Delete File`, and `Move to` as it does now. *Alternative:* record only the `PostToolUse` disk snapshot. Rejected: it takes in concurrent changes.
- **Remove the leases completely.** Remove `lease`, `withLeaseFile`, the wait loop, `TookOver`, `leaseHeld`, `releaseLease`, `leaseTTL`, `leaseTimeout`, the `leases/` directory, and exit code 2. `PreToolUse` still writes the pending snapshot. *Alternative:* keep leases for Codex only. Rejected: the in-memory patch covers Codex, and one code path is simpler.
- **`add --edits` without the disk check.** Remove `checkRecordsSafe`, `editOnDisk`, `overwrites`, and `addsBack`. The `Uncertain` field stays readable in old records and is ignored. `applyRecord` and `replayRecord` stay unchanged. Their `edit-conflict` already covers a record that does not fit the stage index. The hint changes to say that the edit changes lines another stage has not committed yet, and to suggest committing after that stage or using `add --hunk`.
- **Once the other stage commits, the record fits.** "Stage follows the branch tip" refreshes the stage index to the new tip before `add`. The record's Before then matches the index on those lines, and the replay succeeds. This needs no new code. A test covers it.

## Risks / Trade-offs

- [A commit can hold content that is not on disk, so no one tested it] → This is the intended backup behavior. The agent guide says that `add --edits` stages what the agent wrote, not what the file holds now.
- [Two agents' tools read and write one file at the same moment, so the disk loses one write] → Without leases the editors can race on disk. Both records still hold their changes, so each stage keeps its edit. The agent guide tells agents to check `git multistage --stage <id> diff` against the file when they rely on disk content.
- [Claude Code changes or drops `originalFile`] → The fallback to the `PreToolUse` snapshot keeps recording working. The fixture test pins the field.
- [The in-memory `apply_patch` differs from Codex's matching, such as whitespace-tolerant matching] → The fallback to the disk after the call covers a mismatch.

## Migration Plan

Existing records keep their format. Records with `uncertain: true` become stageable. Leftover lease files from an earlier version are ignored. Rollback is the previous release, which reads the same records.
