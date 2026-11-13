# Spec Delta

## Purpose

The repository is a Claude Code plugin and a Codex plugin. Each installs the editor hooks and the agent guide skill, so an agent can stage its own edits without manual setup.

## ADDED Requirements

### Requirement: Plugin manifests
The repository SHALL provide a Claude Code plugin and marketplace (`.claude-plugin/`) and a Codex plugin and marketplace (`.codex-plugin/`, `.agents/plugins/`), so each tool can install the plugin from `chrisfls/git-multistage`.

#### Scenario: Claude Code install
- **WHEN** the user runs `/plugin marketplace add chrisfls/git-multistage` and `/plugin install git-multistage@git-multistage`
- **THEN** Claude Code installs the hooks and the `git-multistage` skill

### Requirement: Plugin hooks
The plugin's hooks file SHALL run `git multistage hook` at `SessionStart`, and before and after `Edit`, `MultiEdit`, `Write`, and `apply_patch`.

#### Scenario: Edit in a plugin session
- **WHEN** an agent with the plugin installed edits a file with `Edit`
- **THEN** the edit is recorded in the agent's stage

#### Scenario: Session start in a plugin session
- **WHEN** a session with the plugin installed starts in a Git worktree
- **THEN** the agent receives the shared-checkout notice

### Requirement: Agent guide skill
The plugin SHALL provide a `git-multistage` skill, with `name` and `description` in its front matter, that teaches an agent its stage ID, the staging and commit commands, and what to do when a command fails.

#### Scenario: Skill front matter
- **WHEN** the test suite reads `skills/git-multistage/SKILL.md`
- **THEN** its front matter names `git-multistage` and has a description

#### Scenario: Skill in a plugin session
- **WHEN** an agent with the plugin installed is about to commit
- **THEN** the `git-multistage` skill is available to it
