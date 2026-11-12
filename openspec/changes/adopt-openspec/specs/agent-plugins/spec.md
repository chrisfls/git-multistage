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
The plugin's hooks file SHALL run `git multistage hook` before and after `Edit`, `MultiEdit`, `Write`, and `apply_patch`.

#### Scenario: Edit in a plugin session
- **WHEN** an agent with the plugin installed edits a file with `Edit`
- **THEN** the edit is recorded in the agent's stage

### Requirement: Agent guide skill
The plugin's `git-multistage` skill SHALL hold the same text as `docs/agent-guide.md`.

#### Scenario: Guide edited
- **WHEN** `docs/agent-guide.md` changes and the skill does not
- **THEN** the test suite fails
