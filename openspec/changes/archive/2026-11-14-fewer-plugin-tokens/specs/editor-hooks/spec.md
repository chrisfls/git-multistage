# editor-hooks Specification

## MODIFIED Requirements

### Requirement: Session start notice
On a `SessionStart` event whose `cwd` is in a Git worktree, the hook SHALL print `additionalContext` that says other agents may edit and commit in the checkout, that their changes are theirs to leave alone, and that the agent stages and commits with `git multistage`. Outside a Git worktree it SHALL print nothing. The first-edit message of a subagent's stage SHALL carry the same note about other agents; a main session's SHALL leave it out. The hook SHALL exit with code 0.

#### Scenario: Session starts in a worktree
- **WHEN** the hook receives `SessionStart` with `cwd` in a Git worktree
- **THEN** it prints additional context about the other agents that names the `git-multistage` skill, and exits with code 0

#### Scenario: Session starts outside a worktree
- **WHEN** the hook receives `SessionStart` with `cwd` outside any Git worktree
- **THEN** it prints nothing and exits with code 0

#### Scenario: First edit of a subagent
- **WHEN** a Claude Code subagent with `agent_id` `x1` makes the first recorded edit of stage `claude-x1`
- **THEN** the first-edit message carries the note about other agents

#### Scenario: First edit of a main session
- **WHEN** a main session makes the first recorded edit of its stage
- **THEN** the first-edit message names the stage and the commands, and leaves out the note about other agents
