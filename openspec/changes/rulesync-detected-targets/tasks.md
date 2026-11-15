# Tasks

## 1. Write the setup script

- [ ] 1.1 Write `scripts/setup.sh`. Map each agent's home config directory to its RuleSync target, for example `~/.claude` to `claudecode`, `~/.cursor` to `cursor`, `~/.config/opencode` to `opencode`. Write `rulesync.local.jsonc` with `codexcli` plus each detected target. Verify with a temporary `HOME` that holds only `~/.cursor`, that the file lists `codexcli` and `cursor` and nothing else.
- [ ] 1.2 Make the script run `mise install`, then `rulesync install && rulesync generate`. Verify with `scripts/setup.sh` in a clean checkout that `rulesync generate --check` passes.
- [ ] 1.3 Remove the `targets` block from `rulesync.jsonc`. Verify that `rulesync doctor` reports no error.

## 2. Ignore the local file

- [ ] 2.1 Add `/rulesync.local.jsonc` to `.gitignore`. Verify with `git check-ignore rulesync.local.jsonc`.

## 3. Document the setup

- [ ] 3.1 Replace the README's RuleSync and mise contributing steps with `scripts/setup.sh`. Verify that the README names no other setup command.
