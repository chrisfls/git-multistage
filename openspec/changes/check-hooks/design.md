# Design

## Context

`hooks/hooks.json` runs `git multistage hook` for SessionStart, and around `Edit|MultiEdit|Write|apply_patch` for PreToolUse and PostToolUse. The hook decodes only `hook_event_name`, `tool_name`, `session_id`, `agent_id`, `tool_use_id`, `cwd`, and `tool_input.file_path` or `tool_input.command`. Without a kind argument it picks Codex for `apply_patch` and Claude Code otherwise.

## Goals / Non-Goals

**Goals:**
- Inputs with every documented field, replayed through the exact command of `hooks/hooks.json`.

**Non-Goals:**
- Recording inputs from a live agent session in CI; that needs an API key.

## Decisions

- **Install the CLI through mise as `npm:@anthropic-ai/claude-code`.** CI already installs tools with `jdx/mise-action` from `mise.toml`, so the version is pinned in one place. Running `claude plugin validate --strict .` with an empty `HOME` and no credentials passes locally, so the job needs no secret.
- **Fixtures with placeholders.** Paths differ per test and per OS, so a fixture holds `{{CWD}}` and `{{FILE}}`, which the test replaces with JSON-escaped values. All other fields are kept as documented.
- **Fields taken from documentation, not from a session.** Claude Code's documentation gives full SessionStart and Write inputs, and the Edit `tool_input` fields. The Edit `tool_response` uses the keys of the Edit result that Claude Code transcripts record. Codex's documentation lists its fields but gives no full example for `apply_patch`, so the `tool_response` text and the field values are assumptions. Codex documents `agent_id` only for subagent and model-request events, so no Codex fixture carries it.
- **One table-driven test.** Each case names its fixtures, the edit to make between PreToolUse and PostToolUse, and the stage that must record it.

## Risks / Trade-offs

- [A fixture drifts from what an agent sends] → The fixtures record which fields were not confirmed from a real session in the report of this change; a later change can replace them with captured inputs.
