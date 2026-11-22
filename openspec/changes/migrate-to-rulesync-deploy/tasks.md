# Tasks

## 1. Prepare the checkout for rulesync-deploy

- [x] 1.1 In `mise.toml`, set `node = "24.19.0"`. Verify with `mise install` and `node --version` inside the checkout.
- [x] 1.2 Add `"features": ["rules", "skills", "subagents"]` to `rulesync.jsonc`, keep `"global": false`, and set `targets: ["*"]` in `.rulesync/rules/overview.md`. Verify that `npx --yes rulesync-deploy` generates `AGENTS.md`, `CLAUDE.md`, and `.claude/skills/openspec-propose/SKILL.md` in the checkout, and that the modification times of `~/.claude/CLAUDE.md` and `~/.codex/AGENTS.md` do not change.
- [x] 1.3 If `rulesync install --frozen` fails on the current `rulesync.lock`, rewrite the lock with the bundled RuleSync 43.10.0 at the same resolved commits. Verify that `npx --yes rulesync-deploy` passes twice in a row and that the second run leaves `rulesync.lock` unchanged.
- [x] 1.4 Set `"gitignoreDestination": "gitexclude"` in `rulesync.jsonc`, remove the hand-written generated-path block from `.gitignore`, and add `!/openspec/**/design.md`. Verify that `git ls-files -ci --exclude-standard` lists nothing, that `git check-ignore -v --no-index .agents/plugins/marketplace.json` reports no match, that `git status --short` lists no generated file, and that a second run leaves `.gitignore` unchanged.

## 2. Remove the old script

- [x] 2.1 Delete `scripts/setup.sh`. Verify that `grep -rn "setup.sh" README.md .rulesync mise.toml` finds nothing.

## 3. Update the contributing commands

- [x] 3.1 In the contributing section of `README.md`, replace `scripts/setup.sh` with `mise install`, and add `GITHUB_TOKEN=$(gh auth token) npx --yes rulesync-deploy` (project scope, no `--global`) as the optional step, with a note to delete a `rulesync.local.jsonc` left by `scripts/setup.sh`, that generates the repository's rules and skills for the contributor's agents. Leave the plugin install section and `AGENT_SETUP.md` unchanged. Verify with `git diff README.md AGENT_SETUP.md`.
- [x] 3.2 In `.rulesync/skills/git-multistage-development/SKILL.md`, replace `rulesync install && rulesync generate` with `GITHUB_TOKEN=$(gh auth token) npx --yes rulesync-deploy`, and run `rulesync install --update` through `npx`, since contributors no longer install RuleSync. Verify by reading the regenerated `.claude/skills/git-multistage-development/SKILL.md`.

## 4. Check the project

- [x] 4.1 Run `mise run ci` on Node 24. Verify that it passes.
- [x] 4.2 Run `claude plugin validate .` on Node 24. Verify that it passes.

## Workflow follow-up

- Archive the change with `openspec-archive-change` after every task is done.
