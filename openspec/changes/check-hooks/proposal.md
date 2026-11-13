# Proposal

## Why

The hooks are the only path by which Claude Code and Codex record edits, but CI checks neither the plugin manifests against Claude Code's validator nor the hook command against inputs in the shape that the agents send. The end-to-end hook tests build minimal inputs by hand, with only the fields that the hook reads, so a field or nesting that a real input carries is never exercised.

## What Changes

- A CI job installs the Claude Code CLI at the version that `mise.toml` pins and runs `claude plugin validate --strict .`. The command reads only local files and needs no API key.
- Saved hook inputs under `test/testdata/hooks/`, built from the documented input formats of Claude Code (SessionStart, PreToolUse, and PostToolUse for Edit and Write, in a session and in a subagent) and Codex (SessionStart, PreToolUse, and PostToolUse for `apply_patch`).
- An end-to-end test replays each input through `git multistage hook`, the command that `hooks/hooks.json` runs, around a real file edit, and checks the stage that records the edit and the commit that `add --edits` makes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change adds a CI job and tests only, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `.github/workflows/ci.yml`: a `plugin` job.
- `mise.toml`: the Claude Code CLI version.
- `test/testdata/hooks/`, `test/hook_replay_test.go`.
