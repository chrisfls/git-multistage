# agent-plugins Specification

## MODIFIED Requirements

### Requirement: Plugin manifests
The repository SHALL provide a Claude Code plugin and marketplace (`.claude-plugin/`) and a Codex plugin and marketplace (`.codex-plugin/`, `.agents/plugins/`), so each tool can install the plugin from `chrisfls/git-multistage`. The plugin install steps in the README SHALL say that the `git-multistage` binary must be on the `PATH` first, and SHALL link to the install from a release.

#### Scenario: Claude Code install
- **WHEN** the user runs `/plugin marketplace add chrisfls/git-multistage` and `/plugin install git-multistage@git-multistage`
- **THEN** Claude Code installs the hooks and the `git-multistage` skill

#### Scenario: Install steps name the binary
- **WHEN** a user reads the plugin install steps in the README
- **THEN** they say that the binary is needed first, and link to how to get it without Go
