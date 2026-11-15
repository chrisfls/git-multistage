# Proposal

## Why

`rulesync.jsonc` names seven agent targets. Adding or removing an agent on a machine means editing a tracked file. RuleSync has no autodetect option, so the repository has to pick the targets for each contributor. The tracked config should name no agent. Contributors also run setup steps by hand from the README, so a setup script should run them in one step.

## What Changes

- Move the agent targets out of the tracked `rulesync.jsonc`. It keeps the sources and the feature sets only.
- Add `scripts/setup.sh`, the devenv setup script that contributors run once after a clone. It:
  1. Runs `mise install`.
  2. Writes `rulesync.local.jsonc` with the targets whose agent has a config directory in the user's home. `codexcli` is always included with `["rules", "skills"]`, because the repository rules go to `AGENTS.md`. Every other target gets `["skills"]`, as today.
  3. Runs `rulesync install && rulesync generate`.
- Gitignore `rulesync.local.jsonc`, which is per machine.
- Replace the README's manual RuleSync and mise steps with `scripts/setup.sh`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change affects the repository's tooling only, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `rulesync.jsonc`: remove the `targets` block. Keep `sources`, `delete`, and `global`.
- `scripts/setup.sh`: new.
- `.gitignore`: add `/rulesync.local.jsonc`.
- `README.md`: the contributing steps run `scripts/setup.sh`.
- A contributor with no agent installed still gets `codexcli` output.
- Release: a patch version, because the change alters no CLI behavior.
