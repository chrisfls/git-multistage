# agent-plugins Specification

## MODIFIED Requirements

### Requirement: Plugin hooks
The plugin's hooks file SHALL run `git multistage hook` at `SessionStart` and `SubagentStart`, before and after `Edit`, `MultiEdit`, `Write`, and `apply_patch`, and before `Bash`.

#### Scenario: Edit in a plugin session
- **WHEN** an agent with the plugin installed edits a file with `Edit` in an opted-in checkout
- **THEN** the edit is recorded in the agent's stage

#### Scenario: Session start in a plugin session
- **WHEN** a session with the plugin installed starts in an opted-in checkout
- **THEN** the agent receives the shared-checkout notice

#### Scenario: Shell command in a plugin session
- **WHEN** an agent with the plugin installed runs `git stash` in an opted-in checkout
- **THEN** the hook denies the command

### Requirement: Failure steps in the skill
For a failed command, the `git-multistage` skill SHALL send the agent to the error's hint, which the CLI words for an agent.

#### Scenario: Stale hunk
- **WHEN** an `add --hunk` fails with exit code 4 and a hint to list the hunks again
- **THEN** the skill tells the agent to follow the hint

#### Scenario: Another branch
- **WHEN** an agent's commit fails with exit code 5 because HEAD is on another branch
- **THEN** the hint that the skill sends the agent to says to stop and report it, and not to change branches

#### Scenario: Held lock
- **WHEN** an agent's command fails with exit code 6 because the default index stays locked
- **THEN** the hint that the skill sends the agent to says to retry after a few seconds, then run `git multistage doctor` and report its output

## ADDED Requirements

### Requirement: Skill description
The `git-multistage` skill's description SHALL say when to use the skill, in one sentence, and leave out what the skill holds.

#### Scenario: Description
- **WHEN** the test suite reads the front matter of `skills/git-multistage/SKILL.md`
- **THEN** the description starts with `Use ` and is one sentence
