# git-multistage

[![CI](https://img.shields.io/github/actions/workflow/status/chrisfls/git-multistage/ci.yml?branch=main&style=for-the-badge&logo=github)](https://github.com/chrisfls/git-multistage/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=for-the-badge)](LICENSE)

git-multistage lets several agents commit from the same Git checkout at the same time.

Each agent gets its own staging area. It commits without waiting for the others, and its commits contain only its own changes. It works without agents too, for example to keep two unrelated changes apart.

It runs as `git multistage` and writes normal Git history.

## The problem

A Git checkout has one staging area, the index, shared by every agent working in it. One agent's `git commit` takes what the others staged. Splitting a file that two agents edited takes `git add -p`, which is interactive.

The usual workaround is one worktree per agent, but each one needs its own build and dependencies, and its branch has to be merged back.

git-multistage keeps one checkout and gives each agent its own index.

## Compared with plain Git

| Task | Plain Git | git-multistage |
| --- | --- | --- |
| Stage a file | `git add src/a.ts`, into the shared index | `git multistage --stage a add src/a.ts` |
| Stage part of a file from a script | Write a patch by hand (`git add -p` is interactive) | `git multistage --stage a add src/shared.ts --hunk d94c87b26e90` |
| See what you staged | `git diff --cached`, which shows every agent's changes | `git multistage --stage a diff --cached` |
| Commit | `git commit`, which takes the whole shared index | `git multistage --stage a commit -m "Fix login"` |
| Keep agents apart | One worktree per agent | One stage per agent, in one checkout |

## What else it does

- **Stage part of a file:** pick blocks of changed lines by ID. An ID listed before the file changed is refused. See [the staging spec](openspec/specs/staging/spec.md).
- **Commit at the same time:** each commit lands on the newest tip of the branch, and none is lost. See [the commit spec](openspec/specs/commit/spec.md).
- **Record agent edits:** hooks for Claude Code and Codex record each edit, so an agent stages exactly its own edits with one command. See [the editor hooks spec](openspec/specs/editor-hooks/spec.md).
- **Keep `git status` correct:** Git's normal index follows each commit, and changes a person staged by hand are kept. See [the default-index sync spec](openspec/specs/default-index-sync/spec.md).
- **Survive crashes:** the next command finishes or cancels a commit that was stopped halfway, so a commit never appears twice. See [the crash recovery spec](openspec/specs/crash-recovery/spec.md).

## Install

Requires Git 2.45 or newer.

```bash
go install github.com/chrisfls/git-multistage/cmd/git-multistage@latest
```

Or build from a clone:

```bash
git clone https://github.com/chrisfls/git-multistage
cd git-multistage
go build -o "$HOME/.local/bin/git-multistage" ./cmd/git-multistage
```

The binary must be on your `PATH`. To build on Windows:

```powershell
go build -o "$env:USERPROFILE\bin\git-multistage.exe" .\cmd\git-multistage
```

`git multistage doctor` checks the setup.

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

`src/login.ts` shows as not staged here, because it belongs to the other stage:

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

`git multistage init` creates a stage with a random ID.

## Coding agents

Give each agent a stage ID, such as its agent ID, and [the agent guide](skills/git-multistage/SKILL.md).

For Claude Code and Codex, install the plugin. It records each edit an agent makes in that agent's stage and teaches the agent the commands. The `git-multistage` binary must be on your `PATH`.

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

Project rules load only when the project is trusted. The rule also covers commands that the agent should not run on its own, such as `drop --force`; use `pattern = ["git", "multistage", "add"]` and similar lines to allow single commands instead.

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

## Contributing

[mise](https://mise.jdx.dev) installs the Go, golangci-lint, and `act` versions that [mise.toml](mise.toml) pins, the same ones CI uses:

```bash
mise install
go test ./...                                                       # all tests
MULTISTAGE_STRESS=10 go test -run 'Many|Hunks|Native|Kill' ./test/  # repeat the concurrency and crash tests
golangci-lint fmt --diff ./... && golangci-lint run ./...           # format and lint
```

The tests run real Git in temporary repositories. CI covers Linux, macOS, and Windows. To run the Linux jobs locally before pushing, run `act` with a Docker engine, such as Docker Desktop, OrbStack, Colima, or Podman.

## License

MIT. See [LICENSE](LICENSE).
