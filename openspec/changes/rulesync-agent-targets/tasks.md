# Tasks

## 1. Add the targets

- [x] 1.1 Add `antigravity-cli`, `opencode`, `cursor`, `devin`, and `grokcli` to `targets` in `rulesync.jsonc`, each with `["skills"]`, and update the comment on why only `codexcli` takes rules. Run `rulesync install && rulesync generate`, and verify that `.cursor/skills/`, `.devin/skills/`, `.grok/skills/`, and `.opencode/skills/` hold the same skills as `.claude/skills/`

## 2. Ignore the generated files

- [x] 2.1 Replace the RuleSync block of `.gitignore` with the agent folders and `AGENTS.md`, keeping `/.rulesync/skills/.curated/` and re-including `/.agents/plugins/`. Verify that `git check-ignore` matches `AGENTS.md` and `.cursor/skills/x`, and does not match `.agents/plugins/marketplace.json`, `.claude-plugin/plugin.json`, or `.codex-plugin/plugin.json`
- [ ] 2.2 Remove `AGENTS.md` from the index through `git multistage` without deleting it from disk, and verify that `git ls-files AGENTS.md` prints nothing, `AGENTS.md` still exists, and `rulesync generate --check` passes
- [ ] 2.3 Add `rulesync install && rulesync generate` to the README's contributing steps, and verify that `go test -run TestPluginFiles ./test/` passes and `git status --short` lists no file under an agent folder
