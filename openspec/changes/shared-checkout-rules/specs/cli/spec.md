# cli Specification

## ADDED Requirements

### Requirement: Hints for agents
When `CLAUDE_CODE_SESSION_ID`, `CODEX_THREAD_ID`, `ORCA_AGENT_SESSION_ID`, or `PASEO_AGENT_ID` is set, an error's hint SHALL leave the state that other agents share alone: a branch or operation error SHALL hint to stop and report it, a held lock to retry and then run `doctor`, `shared-path` to stage hunks, and `hook-modified` to stage the changes again by hunk. With `CODEX_THREAD_ID`, an error from a read-only `.git` SHALL hint to run with escalated permissions.

#### Scenario: Branch mismatch for an agent
- **WHEN** `CLAUDE_CODE_SESSION_ID` is set and a commit fails because HEAD is on another branch than the stage's
- **THEN** the hint says to stop and report it, and does not say to check out a branch

#### Scenario: Branch mismatch for a person
- **WHEN** none of the agent variables is set and a commit fails because HEAD is on another branch than the stage's
- **THEN** the hint says to check out the stage's branch again, or drop the stage

#### Scenario: Held lock for an agent
- **WHEN** `CODEX_THREAD_ID` is set and the default index stays locked
- **THEN** the hint says to retry after a few seconds and then run `git multistage doctor`, and does not say to delete the lock file
