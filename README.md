# git-multistage

[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=for-the-badge)](LICENSE)

git-multistage gives a Git checkout multiple staging areas. Each stage has its own index, so you can stage unrelated changes separately and commit them to the same branch. Stages share the working tree, and you can select individual hunks when two changes touch the same file.

```bash
git multistage --stage feature-a add src/a.ts
git multistage --stage feature-b add src/b.ts
git multistage --stage feature-a commit -m "Add feature A"
git multistage --stage feature-b commit -m "Add feature B"
```

You can use it for your own changes or to let multiple coding agents work in one checkout. The commits are ordinary Git commits.

## The problem

Git normally has one index per checkout. If you're working on unrelated changes, you have to stage and commit them one at a time. `git add -p` can separate changes within a file, but it's interactive and awkward to script.

Worktrees give each change its own checkout and branch. That also means separate working trees, potentially separate dependencies and build outputs, and branches to merge back.

git-multistage keeps separate indexes in the same checkout.

## Install

Git 2.45 or newer is required. Install with [mise](https://mise.jdx.dev):

```bash
mise use -g github:chrisfls/git-multistage
```

Check your installation with `git multistage doctor`, or run `git multistage version` to print the version. You can also install from a release archive, with Go, or from a clone; see [Other ways to install](#other-ways-to-install).

## Quick start

Stage and commit a file:

```bash
git multistage --stage fix-login add src/login.ts
git multistage --stage fix-login commit -m "Honor returnTo after login"
```

The `fix-login` stage is created automatically if it doesn't exist. The commit includes only changes staged in `fix-login`; other changes stay in the working tree or in their own stages.

Because stages share a working tree, builds and tests see all uncommitted changes.

### Stage part of a file

Suppose `src/shared.ts` has two unrelated edits and you want to commit only the input-length check. List the file's hunks:

```console
$ git multistage --stage validation hunks src/shared.ts
diff src/shared.ts (modified)
@@ -2,1 +2,1 @@ id=d94c87b26e90
-  if (!input) return false;
+  if (!input || input.length > 256) return false;
@@ -7,1 +7,1 @@ id=3c16c257727c
-  return input.trim();
+  return input.trim().toLowerCase();
warning: created stage validation on refs/heads/main
```

Stage the first hunk by ID and commit it:

```console
$ git multistage --stage validation add src/shared.ts --hunk d94c87b26e90
staged 1 hunk(s) in src/shared.ts
$ git multistage --stage validation commit -m "Cap input length"
[main 7c4dad5ed0] Cap input length
 1 file(s) changed
```

The commits are on the same branch:

```console
$ git log --oneline
7c4dad5 Cap input length
d2867cd Honor returnTo after login
f2f2964 Initial commit
```

The other edit remains in the working tree:

```console
$ git status --short
 M src/shared.ts
```

### Set the stage once

Set the stage ID in your shell to avoid passing `--stage` on every command:

```bash
export GIT_MULTISTAGE_ID=fix-login
```

In PowerShell:

```powershell
$env:GIT_MULTISTAGE_ID = "fix-login"
```

`git multistage init` creates a stage with a random ID and prints it.

## How stages work

A stage commits to the branch that was checked out when the stage was created. Its index is independent of other stages, but they all use the same working tree.

`add <path>` stages the file as it currently appears on disk, including every change in it. Use `add --hunk` to stage individual hunks.

A deletion or move in the shared working tree may belong to another change, so `add` leaves those alone. Use `rm` or `mv` to stage them explicitly.

Builds, tests, and other commands run against the shared working tree. For an isolated run, create a separate worktree with `git worktree add`.

## Compared with plain Git

| Task | Plain Git | git-multistage |
| --- | --- | --- |
| Stage a file | `git add src/a.ts`, into the shared index | `git multistage --stage a add src/a.ts` |
| Stage part of a file from a script | Build a patch and run `git apply --cached` (`git add -p` is interactive) | `git multistage --stage a add src/shared.ts --hunk d94c87b26e90` |
| See what you staged | `git diff --cached`, which shows everything staged in the checkout | `git multistage --stage a diff --cached` |
| Delete or move a file | `git rm` or `git mv`, into the shared index | `git multistage --stage a rm src/old.ts` or `git multistage --stage a mv src/a.ts src/b.ts` |
| Commit | `git commit`, which takes the whole shared index | `git multistage --stage a commit -m "Fix login"` |
| Keep changes separate | Commit one change at a time, or use one worktree per change | One stage per change, in one checkout |

## Other ways to install

The directory containing `git-multistage` must be on your `PATH`.

**From a release (no Go required):** Download the archive for your OS and architecture and `checksums.txt` from [Releases](https://github.com/chrisfls/git-multistage/releases). Archives are available for Linux, macOS, and Windows on amd64 and arm64. Verify the checksum and extract the binary:

```bash
shasum -a 256 -c checksums.txt --ignore-missing
tar -xzf git-multistage_<version>_<os>_<arch>.tar.gz git-multistage
mkdir -p "$HOME/.local/bin"
mv git-multistage "$HOME/.local/bin/"
```

On Windows, unpack `git-multistage.exe` from the `.zip` into a directory on your `PATH`.

**With Go:**

```bash
go install github.com/chrisfls/git-multistage/cmd/git-multistage@latest
```

`go install` writes the binary to `GOBIN`, or to `$(go env GOPATH)/bin` if `GOBIN` is unset. Make sure that directory is on your `PATH`.

**From a clone:**

```bash
git clone https://github.com/chrisfls/git-multistage
cd git-multistage
go build -o "$HOME/.local/bin/git-multistage" ./cmd/git-multistage
```

On Windows:

```powershell
go build -o "$env:USERPROFILE\bin\git-multistage.exe" .\cmd\git-multistage
```

## Coding agents

git-multistage has features for coding agents sharing a checkout. Its optional plugin tracks edits made through each agent's file tools, so `add --edits` can stage only that agent's changes, even when other agents edit the same file. The plugin also provides shared-checkout rules and blocks Git commands that change shared state.

Edits made by shell commands, such as formatters, aren't tracked by the plugin. Stage those with `add` or `add --hunk`, or with `rm` and `mv` for deleted or moved files.

### Without the plugin

Give each agent its own stage ID (its agent ID works fine). Have agents follow the [git-multistage skill](skills/git-multistage/SKILL.md) for staging and committing, and add these shared-checkout rules to the project's `AGENTS.md` or `CLAUDE.md`:

```markdown
Other agents may edit files in this checkout and commit to this branch while you work. Commits, file changes, and stages that you did not make are theirs: do not review, revert, stage, or mention them unless they block your task, and keep their changes in the working tree, even for a moment. To run something without them, use a separate worktree from `git worktree add`. Do not run `git add`, `git commit`, `git rm`, `git mv`, `git stash`, `git restore`, `git reset`, `git checkout`, or `git switch`: they change state that every agent shares. Stage and commit with `git multistage`, as skills/git-multistage/SKILL.md describes.
```

### With the plugin

The plugin's hooks run only in checkouts that opt in. Enable them in a shared checkout:

```bash
git config multistage.shared true
```

Once enabled, the plugin gives each session and subagent the shared-checkout rules and blocks commands that change shared Git state, such as `git stash` and `git commit`. Disable it with `git config multistage.shared false`.

[Install the binary](#install) before installing the plugin. The hooks report an error if `git-multistage` isn't on your `PATH`. You can install the plugin in Claude Code, Codex, or one of the [other harnesses](#other-harnesses).

Claude Code:

```text
/plugin marketplace add chrisfls/git-multistage
/plugin install git-multistage@git-multistage
```

Codex:

```bash
codex plugin marketplace add chrisfls/git-multistage
codex plugin add git-multistage@git-multistage
```

Codex asks you to trust the plugin's hooks at the next startup. Its default sandbox makes `.git` read-only, so `git multistage` needs permission to run outside the sandbox. Add this rule to `.codex/rules/multistage.rules` in a trusted project, or to a file in `~/.codex/rules/`:

```python
prefix_rule(pattern = ["git", "multistage"], decision = "allow")
```

The rule also permits commands that can discard staged changes, including `drop --force`. For narrower permissions, use separate rules such as `pattern = ["git", "multistage", "add"]`.

To commit changes recorded by the plugin:

```bash
git multistage --stage <id> add --edits
git multistage --stage <id> commit -m "Add input validation"
```

`add --edits` stages only the lines the agent changed, even if another agent edited adjacent lines. It uses the plugin's edit record, so those changes can still be staged if another writer has since overwritten the lines or deleted the file.

### Other harnesses

The plugin also supports the harnesses below. `<clone>` refers to a clone of this repository. The binary must be on your `PATH`, and `multistage.shared` must be enabled in the checkout.

<details>
<summary>Install steps for each harness</summary>

| Harness | Install |
| --- | --- |
| Antigravity | `agy plugin install https://github.com/chrisfls/git-multistage` |
| Factory Droid | `droid plugin marketplace add https://github.com/chrisfls/git-multistage`, then `droid plugin install git-multistage@git-multistage` |
| GitHub Copilot CLI | `copilot plugin marketplace add chrisfls/git-multistage`, then `copilot plugin install git-multistage@git-multistage` |
| Devin CLI | `devin plugins install chrisfls/git-multistage` |
| Grok Build | `grok plugin install chrisfls/git-multistage --trust` |
| CodeBuddy | `codebuddy plugin add github:chrisfls/git-multistage`, then `codebuddy plugin enable git-multistage` and confirm, because the plugin installs disabled |
| Qoder CLI | `qodercli plugins marketplace add chrisfls/git-multistage`, then `qodercli plugins install git-multistage` |
| ZCode | `zcode plugins marketplace add <clone>`, then `zcode plugins install git-multistage@git-multistage --scope user` |
| MiniMax Code | `git clone https://github.com/chrisfls/git-multistage ~/.minimax/plugins/git-multistage` |
| Goose | `goose plugin install https://github.com/chrisfls/git-multistage.git` |
| OpenHands | copy `skills/git-multistage` to `~/.agents/skills/`, and [`hooks/openhands.json`](hooks/openhands.json) to `.openhands/hooks.json`; OpenHands rejects the `SubagentStart` event of the plugin's `hooks/hooks.json` |
| Oh My Pi | `omp plugin marketplace add chrisfls/git-multistage`, then `omp plugin install git-multistage@git-multistage` |
| Cursor CLI | `cursor-agent plugin marketplace add https://github.com/chrisfls/git-multistage`, then install it from `/plugin` |
| Kimi Code | `/plugins install https://github.com/chrisfls/git-multistage` |
| Muse Code | set `MUSE_EXPERIMENTAL_PLUGINS=1` if `muse plugins` says that plugins are not available, then `muse plugins marketplace add git-multistage https://github.com/chrisfls/git-multistage`, `muse plugins install git-multistage@git-multistage`, then `muse plugins approve git-multistage` |
| Hermes Agent | `hermes plugins install chrisfls/git-multistage --enable` |
| OpenCode | add `"file://<clone>/plugins/opencode/git-multistage.js"` to the `plugin` list of `opencode.json`; `opencode plugin` installs only packages from the npm registry |
| Kilo Code | `kilo plugin git:https://github.com/chrisfls/git-multistage`, and copy `<clone>/skills/git-multistage` to `~/.config/kilo/skills/`, because Kilo loads no skills from a plugin |
| MiMo Code | add `"file://<clone>/plugins/opencode/git-multistage.js"` to the `plugin` list of `~/.config/mimocode/mimocode.json` |
| CodeArts Agent | save [`plugins/opencode/git-multistage.js`](plugins/opencode/git-multistage.js) in the project's `.codeartsdoer/plugin/`, and point the agent to the skill file |
| Pi | `pi install git:github.com/chrisfls/git-multistage` |
| Amp | `amp plugins add https://raw.githubusercontent.com/chrisfls/git-multistage/master/plugins/amp/git-multistage.js`, then `amp skill add chrisfls/git-multistage` |
| Qwen Code | `qwen extensions install chrisfls/git-multistage`, then confirm the plugin and its install in the prompts |
| Gemini CLI | `gemini extensions install <clone>/plugins/gemini`, then confirm the prompt to trust the folder |
| Mistral Vibe | `cp -R <clone>/plugins/vibe ~/.vibe/plugins/git-multistage`, then `/reload-plugins` |
| Kiro CLI | merge the `hooks` of [`hooks/kiro.json`](hooks/kiro.json) into your agent's configuration in `~/.kiro/agents/`, and add `skill://<clone>/skills/git-multistage/SKILL.md` to its `resources` |
| Cline | on macOS and Linux, copy the files of [`hooks/cline/`](hooks/cline/) to `~/Documents/Cline/Hooks/`, and `skills/git-multistage/SKILL.md` to the project's `.clinerules/` |
| DeepSeek Harness | in the profile's YAML, add the plugin `@deepseek-ai/dsh-hooks-claude-code` with `configPath` set to `<clone>/hooks/hooks.json` |
| Aider | `aider --read <clone>/skills/git-multistage/SKILL.md --no-auto-commits`, or `read:` and `auto-commits: false` in `.aider.conf.yml` |

These installations include the skill and track file-tool edits, with a few exceptions:

- Aider has no hooks, so it records no edits, and it prints the `git multistage` commands for you to run. `--no-auto-commits` keeps its own commits, which take whole files, out of the shared branch.
- DeepSeek Harness loads no skills; point the agent to the skill file.
- MiniMax Code 0.7.7 did not list a plugin placed in `~/.minimax/plugins`, so its step is untested.
- Grok Build, Kimi Code, and Goose may not show the first-edit message to the model. An agent that receives no message stages its files by path, as the skill says.

Tests exercise the Claude Code and Codex hooks using the input those tools send. For the other harnesses, tests replay documented hook input or use a fake host; they haven't been tested in live sessions.

</details>

## Specs

[openspec/specs/](openspec/specs/) documents the behavior of stages, staging, commits, crash recovery, agent hooks, plugins, the CLI, releases, and local checks.

## Contributing

After cloning, run the setup script once. It installs the tool versions pinned in [mise.toml](mise.toml) and generates agent files, including `AGENTS.md` and skills for the coding agents installed on your machine. You'll need [mise](https://mise.jdx.dev) and [RuleSync](https://github.com/dyoshikawa/rulesync) (for example, `brew install rulesync`):

```bash
scripts/setup.sh
```

Start each change in `openspec/changes/` using [OpenSpec](https://openspec.dev), as described in `AGENTS.md`. Run the checks before pushing:

```bash
mise run ci          # format, lint, vet, tests, and OpenSpec validation; run it before you push
mise run ci:stress   # repeat the concurrency and crash tests
```

CI runs only when started from the Actions tab. It tests on Linux, macOS, and Windows; `act` can run the Linux job locally using Docker.

## License

MIT. See [LICENSE](LICENSE).
