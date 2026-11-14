# agent-stage-detection Specification

## Purpose
`--agentic` lets a coding agent with no stage ID of its own use the stage that its environment names, which matches the stage that its editor hook records into.

## Requirements

### Requirement: Opt-in detection
Agent environment variables SHALL select a stage only when `--agentic` is given and neither `--stage` nor `GIT_MULTISTAGE_ID` is set.

#### Scenario: Without the flag
- **WHEN** `CLAUDE_CODE_SESSION_ID` is set and the user runs `status` with no stage ID and no `--agentic`
- **THEN** the command exits with code 2

### Requirement: Agent tool variables
With `--agentic`, `CLAUDE_CODE_SESSION_ID` SHALL select `claude-<id>` and `CODEX_THREAD_ID` SHALL select `codex-<id>`. When both are set, or one is set with an orchestrator variable, a tool variable SHALL count only if its tool is an ancestor process, and the nearer ancestor SHALL win. If the process table cannot be read, a single tool variable SHALL win and two SHALL exit with code 2. Two tool variables with no ancestor tool and no orchestrator variable SHALL also exit with code 2.

#### Scenario: Claude Code agent
- **WHEN** `CLAUDE_CODE_SESSION_ID=abc` is the only agent variable set and the user runs `--agentic status`
- **THEN** the command uses stage `claude-abc`

#### Scenario: Codex nested in Claude Code
- **WHEN** both variables are set and `codex` is the nearer ancestor process
- **THEN** the command uses stage `codex-<CODEX_THREAD_ID>`

#### Scenario: Stale tool variable under an orchestrator
- **WHEN** `CLAUDE_CODE_SESSION_ID` and `PASEO_AGENT_ID=p` are set, Claude Code is not an ancestor process, and the user runs `--agentic status`
- **THEN** the command uses stage `paseo-p`

#### Scenario: No ancestor tool
- **WHEN** both tool variables are set, neither tool is an ancestor process, no orchestrator variable is set, and the user runs `--agentic status`
- **THEN** the command exits with code 2

### Requirement: Orchestrator variables
With `--agentic` and no tool variable that counts, `ORCA_AGENT_SESSION_ID` SHALL select `orca-<id>` and `PASEO_AGENT_ID` SHALL select `paseo-<id>`. When both are set, the command SHALL exit with code 2. When none of the four agent variables is set, the command SHALL exit with code 2.

#### Scenario: Paseo agent
- **WHEN** only `PASEO_AGENT_ID=p1` is set and the user runs `--agentic status`
- **THEN** the command uses stage `paseo-p1`

#### Scenario: Both orchestrators
- **WHEN** both `ORCA_AGENT_SESSION_ID` and `PASEO_AGENT_ID` are set, no tool variable counts, and the user runs `--agentic status`
- **THEN** the command exits with code 2

#### Scenario: No agent variable
- **WHEN** none of the four agent variables is set and the user runs `--agentic status`
- **THEN** the command exits with code 2

### Requirement: Agent IDs in stage IDs
A stage ID that the hook or `--agentic` builds from an agent ID SHALL be the editor name of the hook, or `claude-`, `codex-`, `orca-`, or `paseo-` for `--agentic`, followed by `-` and the agent ID, in lower case, with each character other than a letter, a digit, `_`, or `-` replaced by `-`, and trailing `-` and `_` removed. A stage ID longer than 64 characters SHALL be cut to at most 64 characters that end in `-` and 12 hex digits of a hash of the full ID.

#### Scenario: Agent ID with a dot
- **WHEN** `CLAUDE_CODE_SESSION_ID=ABC.def` is the only agent variable and the user runs `--agentic status`
- **THEN** the command uses stage `claude-abc-def`

#### Scenario: Editor name of another harness
- **WHEN** `hook kimi` records an edit of session `S.1`
- **THEN** the edit goes to stage `kimi-s-1`

### Requirement: Bounded process table read
The read of the process table for `--agentic` SHALL stop after at most 3 seconds. A read that stops SHALL count as a process table that cannot be read.

#### Scenario: The process table read hangs
- **WHEN** `CLAUDE_CODE_SESSION_ID=abc` and `PASEO_AGENT_ID=p` are set, reading the process table hangs, and the user runs `--agentic status`
- **THEN** the command uses stage `claude-abc` and finishes within a few seconds after the limit
