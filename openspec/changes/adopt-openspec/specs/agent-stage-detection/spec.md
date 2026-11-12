# Spec Delta

## Purpose

`--agentic` lets a coding agent with no stage ID of its own use the stage that its environment names, which matches the stage that its editor hook records into.

## ADDED Requirements

### Requirement: Opt-in detection
Agent environment variables SHALL select a stage only when `--agentic` is given and neither `--stage` nor `GIT_MULTISTAGE_ID` is set.

#### Scenario: Without the flag
- **WHEN** `CLAUDE_CODE_SESSION_ID` is set and the user runs `status` with no stage ID and no `--agentic`
- **THEN** the command exits with code 2

### Requirement: Agent tool variables
With `--agentic`, `CLAUDE_CODE_SESSION_ID` SHALL select `claude-<id>` and `CODEX_THREAD_ID` SHALL select `codex-<id>`. When both are set, the tool that is the nearer ancestor process SHALL win. If neither tool is an ancestor, the command SHALL exit with code 2.

#### Scenario: Claude Code agent
- **WHEN** `CLAUDE_CODE_SESSION_ID=abc` is set and the user runs `--agentic status`
- **THEN** the command uses stage `claude-abc`

#### Scenario: Codex nested in Claude Code
- **WHEN** both variables are set and `codex` is the nearer ancestor process
- **THEN** the command uses stage `codex-<CODEX_THREAD_ID>`

### Requirement: Orchestrator variables
With `--agentic` and no agent tool variable, `ORCA_AGENT_SESSION_ID` SHALL select `orca-<id>` and `PASEO_AGENT_ID` SHALL select `paseo-<id>`. When both are set, the command SHALL exit with code 2.

#### Scenario: Paseo agent
- **WHEN** only `PASEO_AGENT_ID=p1` is set and the user runs `--agentic status`
- **THEN** the command uses stage `paseo-p1`

#### Scenario: Both orchestrators
- **WHEN** both `ORCA_AGENT_SESSION_ID` and `PASEO_AGENT_ID` are set
- **THEN** the command exits with code 2
