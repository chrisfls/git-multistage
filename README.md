# git-multistage

[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=for-the-badge)](LICENSE)

git-multistage lets several agents commit from the same Git checkout at the same time.

Each agent gets its own staging area. It commits without waiting for the others, and its commits contain only its own changes. It works without agents too, for example to keep two unrelated changes apart.

It runs as `git multistage` and writes normal Git history.

## What it keeps apart

Stages share one working tree, so every agent edits the same files. git-multistage keeps their staging and their commits apart:

- With the [agent plugin](#coding-agents), an agent holds a short lease on a file while it edits it, so two agents' edits of one file run one after the other.
- `add --edits` stages only the lines that the agent itself changed, even when another agent changed the next line.
- If another writer overwrote those lines, `add --edits` stages nothing and fails with `edit-lost`.
- Edits made through shell commands, such as a formatter, are not recorded. Stage them with `add` or `add --hunk`.

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
mv git-multistage "$HOME/.local/bin/"
```

On Windows, unpack `git-multistage.exe` from the `.zip` into a directory on your `PATH`.

**With Go:**

```bash
go install github.com/chrisfls/git-multistage/cmd/git-multistage@latest
```

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

Two changes are in progress in one checkout, each in its own stage:

- `fix-login`: a fix in `src/login.ts`.
- `validation`: an input check in `src/shared.ts`, which also holds an unrelated edit.

### 1. Stage a whole file

`src/login.ts` belongs only to the login fix:

```console
$ git multistage --stage fix-login add src/login.ts
staged: M  src/login.ts
warning: created stage fix-login on refs/heads/main
```

### 2. Stage part of a file

Only the first of the two edits in `src/shared.ts` is part of the input check:

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

Stage it by ID:

```console
$ git multistage --stage validation add src/shared.ts --hunk d94c87b26e90
staged 1 hunk(s) in src/shared.ts
```

### 3. Check a stage

`src/login.ts` shows as not staged here, because it belongs to the other stage. The second `src/shared.ts` entry is the unrelated edit:

```console
$ git multistage --stage validation status
stage validation on main (base a1accad774, up to date)
staged:
  M  src/shared.ts
not staged:
  M  src/login.ts
  M  src/shared.ts
```

### 4. Commit

Commit in any order, or at the same time:

```console
$ git multistage --stage fix-login commit -m "Honor returnTo after login"
[main f147d75797] Honor returnTo after login
 1 file(s) changed
$ git multistage --stage validation commit -m "Cap input length"
[main 0858e389c5] Cap input length
 1 file(s) changed
```

```console
$ git log --oneline
0858e38 Cap input length
f147d75 Honor returnTo after login
a1accad Initial commit
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

When no ID is at hand, `git multistage init` creates a stage with a random ID and prints it.

## Coding agents

Give each agent a stage ID, such as its agent ID, and [the agent guide](skills/git-multistage/SKILL.md).

For Claude Code and Codex, install the plugin. It records each edit an agent makes in that agent's stage and teaches the agent the commands. The plugin calls the `git-multistage` binary, so [install it](#install) first: the hooks print an error while the binary is not on your `PATH`.

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

Codex asks you to trust the plugin's hooks the next time it starts. Its default sandbox (`workspace-write`) makes `.git` read-only, so `git multistage` fails there, as `git commit` does. A rule lets Codex run it outside the sandbox without asking. Add it to `.codex/rules/multistage.rules` in the project, or to a file in `~/.codex/rules/`:

```python
prefix_rule(pattern = ["git", "multistage"], decision = "allow")
```

Project rules load only when the project is trusted. The rule also covers commands that the agent should not run on its own, such as `drop --force`. To allow single commands instead, use `pattern = ["git", "multistage", "add"]` and similar lines.

An agent then commits its own edits with:

```bash
git multistage --stage <id> add --edits
git multistage --stage <id> commit -m "Add input validation"
```

## Specs

The behavior of every command is specified in [openspec/specs/](openspec/specs/), one spec for each capability:

- [Stages](openspec/specs/stages/spec.md): stage IDs, creation, deletion, and keeping a stage current.
- [Staging](openspec/specs/staging/spec.md): files, hunks, patches, and inspecting a stage.
- [Commit](openspec/specs/commit/spec.md): concurrent commits, refusals, messages, and Git hooks.
- [Default-index sync](openspec/specs/default-index-sync/spec.md): keeping `git status` correct.
- [Crash recovery](openspec/specs/crash-recovery/spec.md): interrupted commands, locks, and garbage collection.
- [Editor hooks](openspec/specs/editor-hooks/spec.md): recorded edits for Claude Code and Codex.
- [Agent stage detection](openspec/specs/agent-stage-detection/spec.md): `--agentic`.
- [CLI](openspec/specs/cli/spec.md): exit codes, JSON output, supported setups, and `doctor`.
- [Agent plugins](openspec/specs/agent-plugins/spec.md): the Claude Code and Codex plugins.
- [Releases](openspec/specs/releases/spec.md): the prebuilt binaries and the tag workflow that publishes them.
- [Local checks](openspec/specs/local-checks/spec.md): `mise run ci` and the workflows that run on GitHub.

## Contributing

[RuleSync](https://github.com/dyoshikawa/rulesync) generates `AGENTS.md` and each coding agent's skills from `.rulesync/` and [rulesync.jsonc](rulesync.jsonc). A fresh clone has neither until you install RuleSync, such as with `brew install rulesync`, and run:

```bash
rulesync install && rulesync generate
```

Every change starts as an [OpenSpec](https://openspec.dev) change in `openspec/changes/`, as `AGENTS.md` describes.

[mise](https://mise.jdx.dev) installs the Go, golangci-lint, `act`, and OpenSpec versions that [mise.toml](mise.toml) pins, the same ones the manual CI workflow uses:

```bash
mise install
mise run ci          # format, lint, vet, tests, and OpenSpec validation; run it before you push
mise run ci:stress   # repeat the concurrency and crash tests
```

The tests run real Git in temporary repositories. No workflow runs on push or pull request: the CI workflow starts by hand from the Actions tab and tests on Linux, macOS, and Windows. To run its Linux job locally, run `act` with a Docker engine, such as Docker Desktop, OrbStack, Colima, or Podman.

## License

MIT. See [LICENSE](LICENSE).
