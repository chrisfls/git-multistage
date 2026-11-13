# Proposal

## Why

The plugin's `SessionStart` hook tells every agent that others work in the checkout, so agents do not review, revert, or report other agents' work. It is verified in Claude Code only. In Codex, only the first-edit message has been observed. If Codex does not hand `SessionStart` `additionalContext` to the model, Codex agents start without the notice until their first edit.

## What Changes

- Verify in a real Codex session with the plugin installed whether the model receives the `SessionStart` notice.
- If it does, record that in the spec as a Codex scenario and add a regression test of the hook's `SessionStart` output for the Codex input.
- If it does not, make the notice reach Codex agents another way that Codex supports, and spec that way. The candidates are the plugin's skill description and a notice in the first `PreToolUse`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `editor-hooks`: the session start notice states which editors deliver it, and the fallback where one does not.

## Impact

- `internal/ms/hook.go` and its tests, only if Codex needs a different output.
- A Codex install and one hook trust in a scratch folder during verification. The Codex config is restored afterwards.
- Out of scope: other editors.
