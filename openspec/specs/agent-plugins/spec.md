# agent-plugins Specification

## Purpose
The repository is a Claude Code plugin and a Codex plugin. Each installs the editor hooks and the agent guide skill, so an agent can stage its own edits without manual setup.

## Requirements

### Requirement: Plugin manifests
The repository SHALL provide a Claude Code plugin and marketplace (`.claude-plugin/`) and a Codex plugin and marketplace (`.codex-plugin/`, `.agents/plugins/`), so each tool can install the plugin from `chrisfls/git-multistage`. The plugin install steps in the README SHALL say that the `git-multistage` binary must be on the `PATH` first, and SHALL link to the install from a release.

#### Scenario: Claude Code install
- **WHEN** the user runs `/plugin marketplace add chrisfls/git-multistage` and `/plugin install git-multistage@git-multistage`
- **THEN** Claude Code installs the hooks and the `git-multistage` skill

#### Scenario: Install steps name the binary
- **WHEN** a user reads the plugin install steps in the README
- **THEN** they say that the binary is needed first, and link to how to get it without Go

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

### Requirement: Agent guide skill
The plugin SHALL provide a `git-multistage` skill, with `name` and `description` in its front matter, that teaches an agent its stage ID, the staging and commit commands, and what to do when a command fails.

#### Scenario: Skill front matter
- **WHEN** the test suite reads `skills/git-multistage/SKILL.md`
- **THEN** its front matter names `git-multistage` and has a description

#### Scenario: Skill in a plugin session
- **WHEN** an agent with the plugin installed is about to commit
- **THEN** the `git-multistage` skill is available to it

### Requirement: Plugin version
The Claude Code plugin, its marketplace entry, and the Codex plugin SHALL carry one version. A release that changes a file of the plugin SHALL raise that version to the release version, so that an installed plugin updates to the new files.

#### Scenario: One version
- **WHEN** the test suite reads `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`, and `.codex-plugin/plugin.json`
- **THEN** each names the same version

#### Scenario: Release that changes the skill
- **WHEN** a maintainer releases `v0.2.0` with a changed `skills/git-multistage/SKILL.md`
- **THEN** the plugin manifests of that release name version `0.2.0`

### Requirement: Stage ID in the skill
The `git-multistage` skill SHALL tell the agent to take its stage ID from the first source that has one: a git-multistage message that names the agent's stage, then the agent ID from its task, then a new stage from `init`. Because the message follows the agent's first edit with a file-editing tool, the skill SHALL tell the agent to make that edit before it chooses an ID.

#### Scenario: Hook stage and task ID
- **WHEN** a git-multistage message names stage `claude-1a2b`, and the agent's task names agent ID `agent-a`
- **THEN** the skill tells the agent to stage and commit with `--stage claude-1a2b`

#### Scenario: Before the first edit
- **WHEN** an agent with the plugin installed has not edited a file yet, and its task names no agent ID
- **THEN** the skill tells it to make its first edit and take the stage that the message names, not to run `init`

#### Scenario: No ID
- **WHEN** the agent's first edit brought no message that names a stage, and the task names no agent ID
- **THEN** the skill tells the agent to run `git multistage init` once and use the printed ID

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

### Requirement: Skill description
The `git-multistage` skill's description SHALL say when to use the skill, in one sentence, and leave out what the skill holds.

#### Scenario: Description
- **WHEN** the test suite reads the front matter of `skills/git-multistage/SKILL.md`
- **THEN** the description starts with `Use ` and is one sentence
