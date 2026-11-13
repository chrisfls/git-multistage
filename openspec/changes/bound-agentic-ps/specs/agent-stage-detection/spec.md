## ADDED Requirements

### Requirement: Bounded process table read
The read of the process table for `--agentic` SHALL stop after at most 3 seconds. A read that stops SHALL count as a process table that cannot be read.

#### Scenario: The process table read hangs
- **WHEN** `CLAUDE_CODE_SESSION_ID=abc` and `PASEO_AGENT_ID=p` are set, reading the process table hangs, and the user runs `--agentic status`
- **THEN** the command uses stage `claude-abc` and finishes within a few seconds after the limit
