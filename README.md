# git-multistage

[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=for-the-badge)](LICENSE)

git-multistage lets several agents commit from the same Git checkout at the same time.

Each agent gets its own staging area and can commit independently, without including changes staged by other agents. It works without agents too, for example to keep two unrelated changes apart.

It runs as `git multistage` and writes normal Git history.

## What it keeps apart

Stages share a working tree, so agents can edit the same files. git-multistage keeps their staged changes and their commits apart.

With the [agent plugin](#coding-agents), agents that edit the same file through their file tools take turns, so their edits do not overlap. The plugin also records which lines each agent changed, so each agent can stage its own edits in a file that others edit too.

Edits made through shell commands, such as a formatter, are not recorded. Stage them separately, with `add` or `add --hunk`.

## The problem

A Git checkout has one staging area, the index, shared by every agent working in it. One agent's `git commit` takes what the others staged. Splitting a file that two agents edited takes `git add -p`, which is interactive.

The usual workaround is one worktree per agent. Each worktree can need its own dependency install and build output, and its branch has to be merged back.

git-multistage keeps one checkout and gives each agent its own index.

## Compared with plain Git

| Task | Plain Git | git-multistage |
| --- | --- | --- |
| Stage a file | `git add src/a.ts`, into the shared index | `git multistage --stage a add src/a.ts` |
| Stage part of a file from a script | Build a patch and run `git apply --cached` (`git add -p` is interactive) | `git multistage --stage a add src/shared.ts --hunk d94c87b26e90` |
| See what you staged | `git diff --cached`, which shows every agent's changes | `git multistage --stage a diff --cached` |
| Commit | `git commit`, which takes the whole shared index | `git multistage --stage a commit -m "Fix login"` |
| Keep agents apart | One worktree per agent | One stage per agent, in one checkout |

## Install

Requires Git 2.45 or newer. Install the `git-multistage` binary with [mise](https://mise.jdx.dev):

```bash
mise use -g github:chrisfls/git-multistage
```

`git multistage doctor` checks the setup, and `git multistage version` prints the version.

### Other ways

Each way puts the `git-multistage` binary in a directory that must be on your `PATH`.

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

## Quick start

Stage a file in a stage, and commit the stage:

```bash
git multistage --stage fix-login add src/login.ts
git multistage --stage fix-login commit -m "Honor returnTo after login"
```

The first command creates `fix-login` if it does not exist yet. The commit contains only changes staged under `fix-login`. Other changes in the checkout stay in the working tree, or in their own stages.

### Stage part of a file

Two agents are working in the same checkout, each with its own stage. `fix-login` is committed as above. The `validation` stage holds an input check in `src/shared.ts`, which also holds an unrelated edit.

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

Each stage made its own commit:

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

To skip `--stage` on every command, set it for the shell:

```bash
export GIT_MULTISTAGE_ID=fix-login
```

In PowerShell:

```powershell
$env:GIT_MULTISTAGE_ID = "fix-login"
```

No stage ID yet? `git multistage init` creates a stage with a random ID and prints it.

## Coding agents

Give each agent its own stage ID, such as its agent ID, and point it to [the agent guide](skills/git-multistage/SKILL.md).

For Claude Code and Codex, install the plugin. The plugin records each edit that an agent makes with its file tools in that agent's stage, and teaches the agent the commands. The agent then stages its recorded edits and commits them:

```bash
git multistage --stage <id> add --edits
git multistage --stage <id> commit -m "Add input validation"
```

`add --edits` stages only the lines that the agent itself changed, even when another agent changed the next line. If another writer overwrote those lines, it stages nothing and fails with `edit-lost`.

The plugin calls the `git-multistage` binary, so [install it](#install) first: the hooks report an error while the binary is not on your `PATH`.

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

## Specs

[openspec/specs/](openspec/specs/) specifies the behavior of every command, one spec for each capability: stages, staging, commits, crash recovery, the agent hooks and plugins, the CLI, releases, and the local checks.

## Contributing

[RuleSync](https://github.com/dyoshikawa/rulesync) generates `AGENTS.md` and the skills in each coding agent's folder, such as `.claude/skills/`, so a fresh clone has neither. Install RuleSync, such as with `brew install rulesync`, and run:

```bash
rulesync install && rulesync generate
```

Every change starts as an [OpenSpec](https://openspec.dev) change in `openspec/changes/`, as `AGENTS.md` describes. [mise](https://mise.jdx.dev) installs the tool versions that [mise.toml](mise.toml) pins:

```bash
mise install
mise run ci          # format, lint, vet, tests, and OpenSpec validation; run it before you push
mise run ci:stress   # repeat the concurrency and crash tests
```

The CI workflow runs only when started from the Actions tab, and tests on Linux, macOS, and Windows. `act` runs its Linux job locally with a Docker engine.

## License

MIT. See [LICENSE](LICENSE).
