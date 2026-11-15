# local-checks Specification

## ADDED Requirements

### Requirement: Live check of the flagship harnesses
`scripts/live-check.sh [claude] [codex] [opencode]` SHALL run one real session of each named agent with this checkout's plugin and binary, in a scratch shared checkout, and SHALL exit with a non-zero code unless each agent commits only its own change from a stage named after the agent. It SHALL leave the user's Codex and OpenCode configuration untouched.

#### Scenario: All three pass
- **WHEN** a maintainer with Claude Code and Codex logged in and `OPENROUTER_API_KEY` set runs `scripts/live-check.sh`
- **THEN** it prints `PASS claude`, `PASS codex`, and `PASS opencode`, and exits with code 0

#### Scenario: Another agent's changes enter the commit
- **WHEN** an agent's commit holds the other stage's change of `b.txt`
- **THEN** the script prints a failure for that agent and exits with a non-zero code
