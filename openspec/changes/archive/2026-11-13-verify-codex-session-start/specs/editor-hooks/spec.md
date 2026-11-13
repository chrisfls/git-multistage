# editor-hooks Specification

## MODIFIED Requirements

### Requirement: Session start notice
On a `SessionStart` event whose `cwd` is in a Git worktree, the hook SHALL print `additionalContext` that says other agents may edit and commit in the checkout, that their changes are theirs to leave alone, and that the agent stages and commits with `git multistage`. Outside a Git worktree it SHALL print nothing. The first-edit message SHALL carry the same note about other agents. The hook SHALL exit with code 0. In Claude Code and in Codex, the model SHALL receive the notice before its first tool call.

#### Scenario: Session starts in a worktree
- **WHEN** the hook receives `SessionStart` with `cwd` in a Git worktree
- **THEN** it prints additional context about the other agents that names the `git-multistage` skill, and exits with code 0

#### Scenario: Session starts outside a worktree
- **WHEN** the hook receives `SessionStart` with `cwd` outside any Git worktree
- **THEN** it prints nothing and exits with code 0

#### Scenario: Claude Code session start
- **WHEN** a Claude Code session starts in a Git worktree with the plugin installed
- **THEN** the model can quote the shared-checkout notice

#### Scenario: Codex session start
- **WHEN** a Codex session starts in a Git worktree with the plugin installed and its hooks trusted
- **THEN** the model can quote the shared-checkout notice before its first tool call
