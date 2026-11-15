# Using git-multistage with coding agents

Multiple coding agents can edit the same checkout while keeping their commits separate. In supported agent sessions, git-multistage automatically derives a stage ID from the session, giving each session its own index without manual stage assignment. Commits from stages created on the same branch go to that branch. The files on disk are shared, so agents can still see or overwrite one another's uncommitted changes.

Install [git-multistage](README.md#install) first and make sure the binary is on your `PATH`.

## Without the plugin

Have agents follow the [git-multistage skill](skills/git-multistage/SKILL.md). In supported agent environments, git-multistage derives the stage ID from each agent session automatically. You only need `--stage <id>` or `GIT_MULTISTAGE_ID` when session detection is unavailable or you want to override it.

Add these rules to your project's `AGENTS.md` or `CLAUDE.md`:

```markdown
Other agents may edit files in this checkout and commit to this branch while you work. Commits, file changes, and stages that you did not make are theirs: do not review, revert, stage, or mention them unless they block your task, and keep their changes in the working tree, even for a moment. To run something without them, use a separate worktree from `git worktree add`. Do not run `git add`, `git commit`, `git rm`, `git mv`, `git stash`, `git restore`, `git reset`, `git checkout`, or `git switch`: they change state that every agent shares. Stage and commit with `git multistage`, as skills/git-multistage/SKILL.md describes.
```

Without edit tracking, `add <path>` stages the entire file as it appears on disk. Use `add --hunk` when agents change different parts of the same file.

## With the plugin

The plugin provides shared-checkout rules, blocks commands that modify shared Git state, and records changes made through each agent's file tools.

Its hooks only run when you enable shared-checkout mode in the repository:

```bash
git config multistage.shared true
```

Disable it with `git config multistage.shared false`.

### Claude Code

```text
/plugin marketplace add chrisfls/git-multistage
/plugin install git-multistage@git-multistage
```

### Codex

```bash
codex plugin marketplace add chrisfls/git-multistage
codex plugin add git-multistage@git-multistage
```

Codex will ask you to trust the plugin hooks at its next startup. Codex's default sandbox makes `.git` read-only, so `git multistage` needs permission to run outside the sandbox.

In a trusted project, put this in `.codex/rules/multistage.rules` (or in a rules file under `~/.codex/rules/`):

```python
prefix_rule(pattern = ["git", "multistage"], decision = "allow")
```

This rule also allows potentially destructive commands such as `git multistage drop --force`. If you want narrower permissions, allow individual commands instead, for example `pattern = ["git", "multistage", "add"]`.

### Stage edits recorded by the plugin

```bash
git multistage add --edits
git multistage commit -m "Add input validation"
```

`add --edits` uses the plugin's edit records to stage only lines changed by that agent. It can stage changes even when another writer has since overwritten those lines or deleted the file.

Edits made by shell commands, such as formatters, are not tracked. Stage those with `add` or `add --hunk`, or use `rm` and `mv` for deletions and moves.

## Other coding agents

The plugin also supports the following harnesses. `<clone>` means a local clone of the [git-multistage repository](https://github.com/chrisfls/git-multistage). Install the binary and enable `multistage.shared` before using these integrations.

| Harness | Installation |
| --- | --- |
| Antigravity | `agy plugin install https://github.com/chrisfls/git-multistage` |
| Factory Droid | `droid plugin marketplace add https://github.com/chrisfls/git-multistage`, then `droid plugin install git-multistage@git-multistage` |
| GitHub Copilot CLI | `copilot plugin marketplace add chrisfls/git-multistage`, then `copilot plugin install git-multistage@git-multistage` |
| Devin CLI | `devin plugins install chrisfls/git-multistage` |
| Grok Build | `grok plugin install chrisfls/git-multistage --trust` |
| CodeBuddy | `codebuddy plugin add github:chrisfls/git-multistage`, then `codebuddy plugin enable git-multistage` and confirm (the plugin installs disabled) |
| Qoder CLI | `qodercli plugins marketplace add chrisfls/git-multistage`, then `qodercli plugins install git-multistage` |
| ZCode | `zcode plugins marketplace add <clone>`, then `zcode plugins install git-multistage@git-multistage --scope user` |
| MiniMax Code | `git clone https://github.com/chrisfls/git-multistage ~/.minimax/plugins/git-multistage` |
| Goose | `goose plugin install https://github.com/chrisfls/git-multistage.git` |
| OpenHands | Copy `skills/git-multistage` to `~/.agents/skills/`, and `hooks/openhands.json` to `.openhands/hooks.json` (OpenHands rejects the `SubagentStart` event in `hooks/hooks.json`) |
| Oh My Pi | `omp plugin marketplace add chrisfls/git-multistage`, then `omp plugin install git-multistage@git-multistage` |
| Cursor CLI | `cursor-agent plugin marketplace add https://github.com/chrisfls/git-multistage`, then install it from `/plugin` |
| Kimi Code | `/plugins install https://github.com/chrisfls/git-multistage` |
| Muse Code | Set `MUSE_EXPERIMENTAL_PLUGINS=1` if needed, then `muse plugins marketplace add git-multistage https://github.com/chrisfls/git-multistage`, `muse plugins install git-multistage@git-multistage`, and `muse plugins approve git-multistage` |
| Hermes Agent | `hermes plugins install chrisfls/git-multistage --enable` |
| OpenCode | Add `"file://<clone>/plugins/opencode/git-multistage.js"` to the `plugin` list of `opencode.json` (the `opencode plugin` command only installs npm packages) |
| Kilo Code | `kilo plugin git:https://github.com/chrisfls/git-multistage`, then copy `<clone>/skills/git-multistage` to `~/.config/kilo/skills/` |
| MiMo Code | Add `"file://<clone>/plugins/opencode/git-multistage.js"` to the `plugin` list of `~/.config/mimocode/mimocode.json` |
| CodeArts Agent | Save `plugins/opencode/git-multistage.js` in `.codeartsdoer/plugin/` and point the agent to the skill file |
| Pi | `pi install git:github.com/chrisfls/git-multistage` |
| Amp | `amp plugins add https://raw.githubusercontent.com/chrisfls/git-multistage/master/plugins/amp/git-multistage.js`, then `amp skill add chrisfls/git-multistage` |
| Qwen Code | `qwen extensions install chrisfls/git-multistage`, then confirm installation and trust prompts |
| Gemini CLI | `gemini extensions install <clone>/plugins/gemini`, then confirm the trust prompt |
| Mistral Vibe | `cp -R <clone>/plugins/vibe ~/.vibe/plugins/git-multistage`, then `/reload-plugins` |
| Kiro CLI | Merge the `hooks` from `hooks/kiro.json` into the agent config under `~/.kiro/agents/`, and add `skill://<clone>/skills/git-multistage/SKILL.md` to `resources` |
| Cline | On macOS and Linux, copy `hooks/cline/` to `~/Documents/Cline/Hooks/` and `skills/git-multistage/SKILL.md` to the project's `.clinerules/` |
| DeepSeek Harness | In its profile YAML, add `@deepseek-ai/dsh-hooks-claude-code` with `configPath` set to `<clone>/hooks/hooks.json` |
| Aider | `aider --read <clone>/skills/git-multistage/SKILL.md --no-auto-commits`, or set `read:` and `auto-commits: false` in `.aider.conf.yml` |

### Integration notes

- Aider has no hooks, so it cannot record edits. It prints the `git multistage` commands for you to run. `--no-auto-commits` prevents Aider from making whole-file commits to the shared branch.
- DeepSeek Harness doesn't load skills automatically. Point the agent to the skill file.
- MiniMax Code 0.7.7 didn't list a plugin placed in `~/.minimax/plugins/`; that installation is untested.
- Grok Build, Kimi Code, and Goose may not show the first-edit message to the model. If an agent receives no message, it stages files by path as described in the skill.

Claude Code and Codex hooks are tested with the input those tools send. Other integrations have been tested by replaying documented hook input or using a fake host; they haven't all been tested in live sessions.
