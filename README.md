# git-multistage

[![CI](https://img.shields.io/github/actions/workflow/status/chrisfls/git-multistage/ci.yml?branch=main&style=for-the-badge&logo=github)](https://github.com/chrisfls/git-multistage/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=for-the-badge)](LICENSE)

git-multistage lets several agents commit from the same Git checkout at the same time.

Each agent gets its own staging area, called a stage. An agent stages its changes in its stage and commits them when it is ready. It does not wait for the other agents to finish, and its commits contain only its own changes.

You can also use stages without agents, for example to keep two unrelated changes apart.

Git runs the tool as `git multistage`. It uses only standard Git commands, so the history it writes is normal Git history.

## The problem

A Git checkout has one staging area, the index. All agents in the checkout share it.

- When two agents run `git add`, both sets of changes go into the index.
- The next `git commit` then takes all of them.
- When two agents edit the same file, you can only split their changes with `git add -p`. That command asks questions at a prompt, so an agent cannot use it.

A common solution is one worktree per agent. But each worktree is a separate folder. It needs its own build and its own dependencies, and its branch must be merged back later.

git-multistage keeps one checkout. It gives each agent its own index instead.

## Compared with plain Git

| Task | Plain Git | git-multistage |
| --- | --- | --- |
| Stage a file | `git add src/a.ts` puts it in the shared index | `git multistage --stage a add src/a.ts` puts it in stage `a` |
| Stage part of a file from a script | `git add -p` asks at a prompt, so you must write a patch by hand | `git multistage --stage a add src/shared.ts --hunk d94c87b26e90` |
| See what you staged | `git diff --cached` shows what every agent staged | `git multistage --stage a diff --cached` shows stage `a` only |
| Commit | `git commit` takes the whole shared index | `git multistage --stage a commit -m "Fix login"` takes stage `a` only |
| Keep agents apart | One worktree per agent | One stage per agent, in one checkout |

## What else it does

- **Stage part of a file:** Each block of changed lines has an ID. An agent stages a block by its ID. If the file changes after the agent listed the IDs, the old ID is refused. See [hunk IDs](docs/cli.md#hunk-ids).
- **Commit at the same time:** Many stages can commit to the same branch at once. Each commit is added on top of the newest commit. No commit is lost. See [the commit transaction](docs/architecture.md#the-commit-transaction).
- **Record agent edits:** Optional hooks for Claude Code and Codex record each edit an agent makes. The agent then stages exactly its own edits with one command. See [coding agents](docs/agents.md).
- **Keep `git status` correct:** After each commit, the tool updates Git's normal index. Changes that a person staged by hand are kept. See [the default index](docs/architecture.md#the-default-index).
- **Survive crashes:** If a commit is stopped halfway, the next command finishes it or cancels it. The commit never appears twice. See [crash recovery](docs/architecture.md#crash-recovery).

## Install

You need Git 2.45 or newer.

Install with Go:

```bash
go install github.com/chrisfls/git-multistage/cmd/git-multistage@latest
```

Or build from a clone:

```bash
git clone https://github.com/chrisfls/git-multistage
cd git-multistage
go build -o "$HOME/.local/bin/git-multistage" ./cmd/git-multistage
```

Put the binary in a folder on your `PATH`. Git then finds it and runs it as `git multistage`.

On Windows, build `git-multistage.exe` into a folder on your `%PATH%`:

```powershell
go build -o "$env:USERPROFILE\bin\git-multistage.exe" .\cmd\git-multistage
```

To check your setup, run `git multistage doctor` in a repository.

## Quick start

In this example, two changes are in progress in one checkout:

- A login fix in `src/login.ts`.
- An input check in `src/shared.ts`. This file also has a second, unrelated edit.

Each change gets its own stage: `fix-login` and `validation`.

### 1. Stage a whole file

`src/login.ts` belongs only to the login fix, so stage the whole file:

```console
$ git multistage --stage fix-login add src/login.ts
staged: M  src/login.ts
warning: created stage fix-login on refs/heads/main
```

The stage is created the first time you use it.

### 2. Stage part of a file

`src/shared.ts` has two edits. Only the first edit belongs to the input check. List the edits with their IDs:

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

Then stage the first edit by its ID:

```console
$ git multistage --stage validation add src/shared.ts --hunk d94c87b26e90
staged 1 hunk(s) in src/shared.ts
```

### 3. Check a stage

Each stage shows only what it staged:

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

Commit each stage when its work is done. The order does not matter, and the commits can happen at the same time:

```console
$ git multistage --stage fix-login commit -m "Honor returnTo after login"
[main f147d75797] Honor returnTo after login
 1 file(s) changed
$ git multistage --stage validation commit -m "Cap input length"
[main 0858e389c5] Cap input length
 1 file(s) changed
```

Each commit contains one change:

```console
$ git log --oneline
0858e38 Cap input length
f147d75 Honor returnTo after login
a1accad Initial commit
```

The second edit in `src/shared.ts` was not staged, so it is still in your working tree:

```console
$ git status --short
 M src/shared.ts
```

### Set the stage once

Instead of `--stage` on every command, you can set the stage for your shell:

```bash
export GIT_MULTISTAGE_ID=fix-login
```

In PowerShell:

```powershell
$env:GIT_MULTISTAGE_ID = "fix-login"
```

If you do not want to choose a name, `git multistage init` creates a stage with a random ID.

## Coding agents

Give each agent its own stage ID. Its agent ID is a good choice. Also give the agent [AGENTS.md](AGENTS.md), which explains the commands.

With Claude Code or Codex, you can install an editor hook. The hook records each edit the agent makes. The agent then stages and commits its edits like this:

```bash
git multistage --stage <id> add --edits
git multistage --stage <id> commit -m "Add input validation"
```

See [docs/agents.md](docs/agents.md) to set up the hooks.

## Documentation

- [Command reference](docs/cli.md): all commands, stage IDs, exit codes, and settings.
- [Coding agents](docs/agents.md): editor hooks for Claude Code and Codex.
- [AGENTS.md](AGENTS.md): a short guide to give to an agent.
- [Architecture](docs/architecture.md): how the tool works inside.
- [Limitations and recovery](docs/limitations.md): what the tool cannot do, and how to fix each error.

## Contributing

Run the tests:

```bash
go test ./...
```

Run the concurrency and crash tests 10 times:

```bash
MULTISTAGE_STRESS=10 go test -run 'Many|Hunks|Native|Kill' ./test/
```

The tests use real Git repositories in temporary folders. CI runs them on Linux, macOS, and Windows.

## License

MIT. See [LICENSE](LICENSE).
