# Proposal

## Why

RuleSync generates every agent's configuration in this repository, but only Codex and Claude Code get it, and only part of the generated output is gitignored: `AGENTS.md` is still tracked. Agents that use Antigravity, OpenCode, Cursor, Devin (Windsurf), or Grok get no skills. A tracked generated file can also drift from its RuleSync source.

## What Changes

- Add five RuleSync targets that take the repository's skills: `antigravity-cli`, `opencode`, `cursor`, `devin` (Windsurf), and `grokcli`.
- Keep the rules generated into `AGENTS.md` only. RuleSync treats `AGENTS.md` as the root rule file of Codex, Antigravity, OpenCode, Devin, and Grok, and Claude Code and Cursor read it too.
- Gitignore the agent folders: `.agents/` (except `.agents/plugins/`), `.agent/`, `.claude/`, `.codex/`, `.cursor/`, `.devin/`, `.gemini/`, `.grok/`, `.opencode/`, and `.windsurf/`. The plugin folders `.claude-plugin/`, `.codex-plugin/`, and `.agents/plugins/` stay tracked.
- Gitignore the generated root file `AGENTS.md`, and remove it from the index. It stays on disk.
- Add `rulesync install && rulesync generate` to the README's contributing steps, because a fresh clone has no `AGENTS.md` or skills until it runs.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change affects the repository's tooling only, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `rulesync.jsonc`: five new targets.
- `.gitignore`: the agent folders and `AGENTS.md`.
- `AGENTS.md`: untracked. Readers on GitHub no longer see it.
- `README.md`: one contributing step.
- Contributors need the `rulesync` CLI, such as `brew install rulesync`.
