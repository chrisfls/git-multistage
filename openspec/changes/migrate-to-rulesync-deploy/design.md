# Design

## Context

`scripts/setup.sh` runs `mise install`, writes `rulesync.local.jsonc` with `codexcli: ["rules", "skills"]` plus `["skills"]` for each agent whose home config directory exists, then runs `rulesync install` and `rulesync generate` with the RuleSync on `PATH` (17.0.0 on the maintainer's machine).

`rulesync-deploy` 0.1.2 reads `rulesync.jsonc` and `rulesync.local.jsonc`, detects targets from executables and config directories, adds `agentsmd` when `.rulesync/rules/` has a rule and `agentsskills` when there are skills, and runs, with its bundled RuleSync 43.10.0:

1. `rulesync install --frozen` when a config declares sources,
2. `rulesync generate --targets <detected>`,
3. `rulesync gitignore --targets <detected> --features *`.

It requires Node 24.19.0 or newer.

## Goals / Non-Goals

**Goals:**

- Contributors generate the repository's rules and skills for their agents with one optional command.
- No agent list or target-detection code in this repository.

**Non-Goals:**

- Installing the git-multistage plugins. Each agent installs them with its standard procedure from `AGENT_SETUP.md`.
- Global (`--global`) deployment. The rules and skills of this repository apply to work in this checkout only, so they must not land in the contributor's user-level agent configuration.
- Changing the skill sources or their refs.

## Decisions

- **Run through `npx`, unpinned in `mise.toml`.** The step is optional, so the repository documents `npx --yes rulesync-deploy` and does not make mise install it for every contributor and every CI run. Alternative: pin `"npm:rulesync-deploy"` in `mise.toml` with a `setup` task; it fixes the version, but installs the tool for CI and for contributors who skip the step.
- **Node 24 in `mise.toml`.** Inside the checkout, mise puts its Node on `PATH`, so `npx` runs `rulesync-deploy` on that Node. Node 22 is below the package's `engines` minimum.
- **Features in `rulesync.jsonc`.** `rulesync-deploy` sets no `--features`, so the tracked config must. `["rules", "skills", "subagents"]` covers the two source folders in `.rulesync/` today and a `.rulesync/subagents/` folder if one is added. Every detected target gets all three. Alternative: `"*"`; it also turns on MCP, hooks, commands, and permissions, which this repository does not mean to deploy to contributors. Alternative: `["skills"]` only; then `agentsmd` gets no rules and `AGENTS.md` is not generated, which breaks Codex and the repository rules for every agent.
- **Keep `rulesync.local.jsonc` ignored.** `rulesync-deploy` no longer writes it, but RuleSync still reads it as a per-machine override, so a contributor may keep one.
- **Let RuleSync own the generated-path ignore block.** `rulesync gitignore` writes the paths for the detected targets. The hand-written block in `.gitignore` duplicates it and goes stale when the target list changes. The repository keeps its own entries (`/git-multistage`, `/dist/`, `/rulesync.local.jsonc`) and the `.agents/plugins/` exception.

## Risks / Trade-offs

- [RuleSync 43 rejects the lock that RuleSync 17 wrote, and `install --frozen` fails] → Run `rulesync install` with RuleSync 43.10.0 once to rewrite `rulesync.lock` at the same resolved commits, and commit it.
- [`rulesync gitignore` ignores `.agents/`, so new files under `.agents/plugins/` become ignored] → Keep the `!/.agents/plugins/**` exception after the RuleSync block, and check it with `git check-ignore`.
- [A contributor's `rulesync-deploy` run adds ignore entries for agents the maintainer does not use] → Accepted: the entries only ignore generated paths. Review the `.gitignore` diff before committing it.
- [Node 24 breaks the Claude Code or OpenSpec CLI that CI runs] → Run `mise run ci` and `claude plugin validate` on Node 24 before committing.
- [Unpinned `npx` picks up a newer `rulesync-deploy` with a different bundled RuleSync] → Accepted for an optional step; `install --frozen` still holds the skill sources at the locked commits.

## Migration Plan

Contributors who used `scripts/setup.sh` run `npx --yes rulesync-deploy` once after pulling. Rollback: revert the commits; `scripts/setup.sh` returns from history.
