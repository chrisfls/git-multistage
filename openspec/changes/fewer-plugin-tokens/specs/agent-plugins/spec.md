# agent-plugins Specification

## ADDED Requirements

### Requirement: Plugin version
The Claude Code plugin, its marketplace entry, and the Codex plugin SHALL carry one version. A release that changes a file of the plugin SHALL raise that version to the release version, so that an installed plugin updates to the new files.

#### Scenario: One version
- **WHEN** the test suite reads `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`, and `.codex-plugin/plugin.json`
- **THEN** each names the same version

#### Scenario: Release that changes the skill
- **WHEN** a maintainer releases `v0.2.0` with a changed `skills/git-multistage/SKILL.md`
- **THEN** the plugin manifests of that release name version `0.2.0`

### Requirement: Stage ID in the skill
The `git-multistage` skill SHALL tell the agent to take its stage ID from the first source that has one: a git-multistage message that names the agent's stage, then the agent ID from its task, then a new stage from `init`.

#### Scenario: Hook stage and task ID
- **WHEN** a git-multistage message names stage `claude-1a2b`, and the agent's task names agent ID `agent-a`
- **THEN** the skill tells the agent to stage and commit with `--stage claude-1a2b`

#### Scenario: No ID
- **WHEN** no message names a stage and the task names no agent ID
- **THEN** the skill tells the agent to run `git multistage init` once and use the printed ID

### Requirement: Failure steps in the skill
For a failed command, the `git-multistage` skill SHALL send the agent to the error's hint. It SHALL give its own steps where a hint does not fit an agent in a shared checkout: a file that another stage staged, a branch or operation state, a held lock, a pre-commit hook that rewrote files, a commit with nothing new, and the Codex sandbox.

#### Scenario: Stale hunk
- **WHEN** an `add --hunk` fails with exit code 4 and a hint to list the hunks again
- **THEN** the skill tells the agent to follow the hint

#### Scenario: Another branch
- **WHEN** a commit fails with exit code 5 because HEAD is on another branch
- **THEN** the skill tells the agent to stop and report it, and not to change branches

#### Scenario: Held lock
- **WHEN** a command fails with exit code 6 and a hint to delete a lock file
- **THEN** the skill tells the agent to retry after a few seconds, then run `git multistage doctor` and report its output
