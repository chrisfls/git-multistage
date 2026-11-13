# Proposal

## Why

`--agentic` reads the process table when several agent variables are set, to find which agent tool is an ancestor. On Linux and macOS it runs `ps` and waits with no limit, so a `ps` that hangs, such as on a stuck NFS mount or under heavy load, hangs every `--agentic` command of the agent.

## What Changes

- The read of the process table stops after 3 seconds. A read that times out counts as a process table that cannot be read: a single tool variable wins, and two tool variables exit with code 2.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-stage-detection`: a new requirement, "Bounded process table read".

## Impact

- `internal/ms/procs_unix.go`: `ps` runs under a 3-second deadline.
- `test/`: one end-to-end test with a `ps` that hangs.
