# git-multistage

[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=for-the-badge)](LICENSE)

git-multistage lets you manage multiple independent staging areas in a single Git checkout, so you can stage and commit unrelated changes separately.

Each stage has its own Git index, and all stages share the working tree and the branch. You choose the stage on each command, and you can stage single hunks when unrelated changes touch the same file.

```bash
git multistage --stage feature-a add src/a.ts
git multistage --stage feature-b add src/b.ts
git multistage --stage feature-a commit -m "Add feature A"
git multistage --stage feature-b commit -m "Add feature B"
```

Use it to work on several changes at once, or to let several coding agents share a checkout. Every commit is a regular Git commit.

## The problem

Git has one staging area, the index, for each checkout. When several tasks share a checkout, their staged changes go into the same index, and `git commit` commits all of it together.

To keep the tasks in separate commits, you stage one task at a time. When two tasks change the same file, `git add -p` can separate them, but it is interactive and hard to script.

Separate worktrees keep changes apart, but each worktree has its own working tree and branch. Each may need separate dependencies and build outputs, and its branch has to be merged back.

git-multistage gives each task its own stage, with its own index, in one checkout, so you can stage and commit the changes of one task without those of another.

## Install

Requires Git 2.45 or newer. Install the `git-multistage` binary with [mise](https://mise.jdx.dev):

```bash
mise use -g github:chrisfls/git-multistage
```

`git multistage doctor` checks the setup, and `git multistage version` prints the version. To install from a release archive, with Go, or from a clone, see [Other ways to install](#other-ways-to-install).

## Quick start

Stage and commit a file:

```bash
git multistage --stage fix-login add src/login.ts
git multistage --stage fix-login commit -m "Honor returnTo after login"
```

The first command creates the `fix-login` stage if it does not exist yet. The commit contains only changes staged under `fix-login`. Other changes remain in the working tree or in their own stages.

Stages keep staged changes apart, but they share the working tree: builds and tests see every uncommitted change in the checkout.

### Stage part of a file

After `fix-login` is committed, `src/shared.ts` holds two unrelated edits. You want to commit an input-length check, and leave the other edit in the working tree.

List the hunks of the file:

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

Stage only the first hunk by its ID, and commit:

```console
$ git multistage --stage validation add src/shared.ts --hunk d94c87b26e90
staged 1 hunk(s) in src/shared.ts
$ git multistage --stage validation commit -m "Cap input length"
[main 7c4dad5ed0] Cap input length
 1 file(s) changed
```

Both commits appear on the same branch:

```console
$ git log --oneline
7c4dad5 Cap input length
d2867cd Honor returnTo after login
f2f2964 Initial commit
```

The unrelated edit is still in the working tree:

```console
$ git status --short
 M src/shared.ts
```

### Set the stage once

To avoid passing `--stage` on every command, set the stage ID in your shell:

```bash
export GIT_MULTISTAGE_ID=fix-login
```

In PowerShell:

```powershell
$env:GIT_MULTISTAGE_ID = "fix-login"
```

To create a stage with a random ID, run `git multistage init`. It prints the ID.

## How stages work

Each stage has its own Git index. All stages share the working tree, and a stage commits to the branch that was checked out when the stage was created. Staging and committing in one stage leaves out what other stages staged.

`add <path>` stages the whole file as it is on disk, with every change in it. To stage part of a file, use `add --hunk`, as in the quick start.

A file deleted or moved in the shared working tree might belong to another task, so `add` does not stage a deletion or a move. Use `rm` or `mv` to stage one.

Commands that you run in the checkout, such as a build or the tests, see every uncommitted change in the working tree. To run something without them, use a separate worktree from `git worktree add`.

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

Whichever way you use, make sure that the directory of the `git-multistage` binary is on your `PATH`.

**From a release**, with no Go toolchain. [Releases](https://github.com/chrisfls/git-multistage/releases) hold archives for Linux, macOS, and Windows on amd64 and arm64, and a `checksums.txt`. Download the archive for your system and `checksums.txt`, then check and unpack it:

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

`go install` puts the binary in `GOBIN`, or in `$(go env GOPATH)/bin` when `GOBIN` is not set. Add that directory to your `PATH` if it is missing.

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

git-multistage works on its own as a command-line tool, and coding agents run the same commands. Give each agent its own stage ID, such as its agent ID, and point it to [the agent guide](skills/git-multistage/SKILL.md).

The optional plugin adds edit tracking. It records the lines that an agent changes through its file tools, so the agent can stage only those lines with `add --edits`, even when other agents edit the same file at the same time. The plugin also gives the agent the rules of a shared checkout, and refuses the Git commands that change shared state.

The plugin tracks only edits made through an agent's file tools. Stage the changes of a shell command, such as a formatter, with `add` or `add --hunk`, or with `rm` and `mv` for a deleted or moved file.

### Without the plugin

Point the agent to the agent guide, and put the rules of a shared checkout in the project's `AGENTS.md` or `CLAUDE.md`:

```markdown
Other agents may edit files in this checkout and commit to this branch while you work. Commits, file changes, and stages that you did not make are theirs: do not review, revert, stage, or mention them unless they block your task, and keep their changes in the working tree, even for a moment. To run something without them, use a separate worktree from `git worktree add`. Do not run `git add`, `git commit`, `git rm`, `git mv`, `git stash`, `git restore`, `git reset`, `git checkout`, or `git switch`: they change state that every agent shares. Stage and commit with `git multistage`, as skills/git-multistage/SKILL.md describes.
```

### With the plugin

The plugin's hooks act only in a checkout that opts in, so a repository that one agent uses alone stays as it is. In each checkout that agents share, run:

```bash
git config multistage.shared true
```

In an opted-in checkout, the plugin gives each session and subagent the rules of a shared checkout at its start, and refuses the Git commands that change shared state, such as `git stash` and `git commit`. `git config multistage.shared false` turns the plugin off for the checkout.

The plugin calls the `git-multistage` binary, so [install it](#install) first: the hooks report an error while the binary is not on your `PATH`. Install the plugin in Claude Code, in Codex, or in one of the [other harnesses](#other-harnesses).

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

Codex asks you to trust the plugin's hooks the next time it starts. Its default sandbox makes `.git` read-only, so `git multistage` needs a rule to run outside it. Add the rule to `.codex/rules/multistage.rules` in a trusted project, or to a file in `~/.codex/rules/`:

```python
prefix_rule(pattern = ["git", "multistage"], decision = "allow")
```

This rule also lets the agent run commands that discard staged changes, such as `drop --force`, without asking. To allow single commands instead, use `pattern = ["git", "multistage", "add"]` and similar lines.

With the plugin installed, the agent stages its recorded edits and commits them:

```bash
git multistage --stage <id> add --edits
git multistage --stage <id> commit -m "Add input validation"
```

`add --edits` stages only the lines that the agent itself changed, even when another agent changed the next line. It stages them from the plugin's record, so the stage keeps the agent's work even after another writer overwrote those lines or deleted the file.

### Other harnesses

The plugin also installs in the harnesses below. `<clone>` is a clone of this repository. The binary must be on your `PATH`, and the checkout must set `multistage.shared`, as for Claude Code.

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

Each harness above gets the skill, and records the edits of its file tools, except where this list says otherwise:

- Aider has no hooks, so it records no edits, and it prints the `git multistage` commands for you to run. `--no-auto-commits` keeps its own commits, which take whole files, out of the shared branch.
- DeepSeek Harness loads no skills; point the agent to the skill file.
- MiniMax Code 0.7.7 did not list a plugin placed in `~/.minimax/plugins`, so its step is untested.
- Grok Build, Kimi Code, and Goose may not show the first-edit message to the model. An agent that receives no message stages its files by path, as the skill says.

The tests run Claude Code and Codex hooks as those tools send them. For the other harnesses, they replay each harness's documented hook input, or drive the plugin in a fake host. A real session in the harness has not been run.

</details>

## Specs

The specs in [openspec/specs/](openspec/specs/) document each command's behavior, one spec for each capability: stages, staging, commits, crash recovery, the agent hooks and plugins, the CLI, releases, and the local checks.

## Contributing

Run the setup script once after a clone. It installs the tool versions that [mise.toml](mise.toml) pins, and generates the agent files, such as `AGENTS.md` and the skills in each coding agent's folder, for the coding agents installed on this machine. It needs [mise](https://mise.jdx.dev) and [RuleSync](https://github.com/dyoshikawa/rulesync), such as `brew install rulesync`:

```bash
scripts/setup.sh
```

Every change starts as an [OpenSpec](https://openspec.dev) change in `openspec/changes/`, as `AGENTS.md` describes. Run the checks before you push:

```bash
mise run ci          # format, lint, vet, tests, and OpenSpec validation; run it before you push
mise run ci:stress   # repeat the concurrency and crash tests
```

The CI workflow runs only when started from the Actions tab, and tests on Linux, macOS, and Windows. `act` runs its Linux job locally with a Docker engine.

## License

MIT. See [LICENSE](LICENSE).
