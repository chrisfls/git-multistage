# agent-stage-detection Specification

## MODIFIED Requirements

### Requirement: Agent IDs in stage IDs
A stage ID that the hook or `--agentic` builds from an agent ID SHALL be the editor name of the hook, or `claude-`, `codex-`, `orca-`, or `paseo-` for `--agentic`, followed by `-` and the agent ID, in lower case, with each character other than a letter, a digit, `_`, or `-` replaced by `-`, and trailing `-` and `_` removed. A stage ID longer than 64 characters SHALL be cut to at most 64 characters that end in `-` and 12 hex digits of a hash of the full ID.

#### Scenario: Agent ID with a dot
- **WHEN** `CLAUDE_CODE_SESSION_ID=ABC.def` is the only agent variable and the user runs `--agentic status`
- **THEN** the command uses stage `claude-abc-def`

#### Scenario: Editor name of another harness
- **WHEN** `hook kimi` records an edit of session `S.1`
- **THEN** the edit goes to stage `kimi-s-1`
