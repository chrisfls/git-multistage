# Proposal

## Why

`scripts/setup.sh` keeps its own list of six agents and the home directory that marks each one, and writes `rulesync.local.jsonc` from that list. The same detection now ships as [rulesync-deploy](https://github.com/chrisfls/rulesync-deploy), an npm CLI that covers every native RuleSync target, runs `rulesync install --frozen`, generates the files, and writes the matching `.gitignore` entries. Contributors who want the repository's rules and skills in their agents can run that CLI, so the repository can drop its own copy of the logic.

This concerns the contributor setup of this repository only: the rules and skills that agents read while they work on git-multistage. Users install the git-multistage plugins with each agent's standard procedure, as `AGENT_SETUP.md` describes, and this change leaves that unchanged.

## What Changes

- Delete `scripts/setup.sh`.
- Document `npx --yes rulesync-deploy` in the contributing section of `README.md` as the optional step that generates the agent configuration for contributors. `mise install` stays the required step.
- Bump `node` in `mise.toml` from `22` to `24.19.0`, the minimum that `rulesync-deploy` requires, so `npx` works inside the checkout where mise activates Node.
- Add `"features": ["rules", "skills", "subagents"]` to `rulesync.jsonc`. `rulesync-deploy` passes `--targets` and no `--features`, so the tracked config sets the features. **Behavior change for contributors:** every detected agent now gets the rules, the skills, and any subagents in `.rulesync/subagents/` (for example, `CLAUDE.md` for Claude Code), where `setup.sh` gave the rules only to Codex and generated no subagents.
- Deploy in project scope only: the documented command is `npx --yes rulesync-deploy` with no `--global`, and `rulesync.jsonc` keeps `"global": false`. Generated files go into the checkout, never into the contributor's home configuration.
- Replace the hand-written block of generated paths in `.gitignore` with the block that `rulesync gitignore` writes. Keep `/rulesync.local.jsonc` ignored and keep `.agents/plugins/` tracked.
- Regenerate `rulesync.lock` if RuleSync 43.10.0, which `rulesync-deploy` bundles, cannot read the current lock. The current lock was written by RuleSync 17.
- Replace `rulesync install && rulesync generate` with `npx --yes rulesync-deploy` in the `git-multistage-development` skill.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change affects the repository's contributor tooling only, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `scripts/setup.sh`: deleted.
- `mise.toml`: `node` 24.19.0. CI installs `node` through `mise-action`, so CI also moves to Node 24.
- `rulesync.jsonc`: new `features` key; `global` stays `false`.
- `rulesync.lock`: may be rewritten in the RuleSync 43 format.
- `.gitignore`: generated-path block replaced by the RuleSync block.
- `README.md`, `.rulesync/skills/git-multistage-development/SKILL.md`: contributing commands.
- Plugins, `AGENT_SETUP.md`, and the plugin manifests: unchanged.
- Release: no new version, because the change alters no CLI or plugin behavior.
