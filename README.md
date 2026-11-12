# git-multistage

[![CI](https://img.shields.io/github/actions/workflow/status/chrisfls/git-multistage/ci.yml?branch=main&style=for-the-badge&logo=github)](https://github.com/chrisfls/git-multistage/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=for-the-badge)](LICENSE)

git-multistage gives a Git checkout more than one staging area. Each stage has its own index and commits to the checked-out branch on its own, so several coding agents, or one person with several changes in flight, can stage and commit in the same checkout at the same time.

It runs as `git multistage` and makes every change through Git's own plumbing, so the result is ordinary linear history that any Git client can push.

## Context: one index per checkout

A Git checkout has one index. When two agents work in the same checkout, `git add` puts both agents' changes into that index, and the next `git commit` takes all of it. If both agents edited the same file, the only way to split their changes is `git add -p`, which waits at a prompt for someone to answer.

The usual workaround is a worktree per agent. That works, but each worktree is a separate directory with its own build and dependencies, and its branch has to be merged back. git-multistage keeps the single checkout and gives each agent its own index.

## Compared with plain Git

| Task | git-multistage | Plain Git |
| --- | --- | --- |
| Stage your file | `git multistage --stage a add src/a.ts` | `git add src/a.ts`, into the index that every agent shares |
| Stage one hunk from a script | `git multistage --stage a add src/shared.ts --hunk d94c87b26e90` | `git add -p`, which asks at a prompt |
| See what you staged | `git multistage --stage a diff --cached` | `git diff --cached`, which shows every agent's staged changes |
| Commit while others commit | `git multistage --stage a commit -m "Fix login"` | `git commit`, which takes the whole shared index and fails while another commit holds `index.lock` |
| Keep agents apart | One stage per agent, in one checkout | One worktree per agent |

Beyond the core commands:

- Agents select hunks of a shared file [by ID](docs/cli.md#hunk-ids), without prompts. An ID stays the same while other hunks are staged or committed, and is refused once the file changes under it.
- A [commit](docs/architecture.md#the-commit-transaction) replays the stage's changes onto the branch tip as it is at that moment, and moves the branch only if nobody else moved it first. Edits that two stages made on neighbouring lines both land.
- Optional [editor hooks](docs/agents.md) for Claude Code and Codex record each edit an agent makes, so `add --edits` stages exactly that agent's edits. Subagents get stages of their own.
- `git status` keeps working. After each commit, the [ordinary index](docs/architecture.md#the-default-index) catches up with the branch, and entries that someone staged by hand stay as they were.
- A commit killed halfway leaves no commit or exactly one, and the stage's next command [finishes or discards it](docs/architecture.md#crash-recovery).

## Install

git-multistage needs Git 2.45 or newer.

```bash
go install github.com/chrisfls/git-multistage/cmd/git-multistage@latest
```

To build from a clone instead:

```bash
git clone https://github.com/chrisfls/git-multistage
cd git-multistage
go build -o "$HOME/.local/bin/git-multistage" ./cmd/git-multistage
```

Git runs any `git-multistage` binary on your `PATH` as `git multistage`. On Windows, build `git-multistage.exe` into a directory on `%PATH%`:

```powershell
go build -o "$env:USERPROFILE\bin\git-multistage.exe" .\cmd\git-multistage
```

Run `git multistage doctor` inside a repository to check the setup.

## Quick start

Two changes are in progress in one checkout: a fix in `src/login.ts`, and an input check in `src/shared.ts`, a file that also holds an unrelated edit. Each change gets its own stage. `src/login.ts` belongs to the fix alone, so it is staged whole:

```console
$ git multistage --stage fix-login add src/login.ts
staged: M  src/login.ts
warning: created stage fix-login on refs/heads/main
```

`src/shared.ts` has two edits, and only the first belongs to the validation change. List the hunks, then stage that one by its ID:

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
$ git multistage --stage validation add src/shared.ts --hunk d94c87b26e90
staged 1 hunk(s) in src/shared.ts
```

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

Commit each stage when its work is done, in any order or at the same moment:

```console
$ git multistage --stage fix-login commit -m "Honor returnTo after login"
[main f147d75797] Honor returnTo after login
 1 file(s) changed
$ git multistage --stage validation commit -m "Cap input length"
[main 0858e389c5] Cap input length
 1 file(s) changed
$ git log --oneline
0858e38 Cap input length
f147d75 Honor returnTo after login
a1accad Initial commit
$ git status --short
 M src/shared.ts
```

Each commit holds one change. The `toLowerCase` edit, which nobody staged, is still in the working tree.

A shell can set the stage once with `GIT_MULTISTAGE_ID` (`export GIT_MULTISTAGE_ID=fix-login`, or `$env:GIT_MULTISTAGE_ID = "fix-login"` in PowerShell) instead of passing `--stage` each time. `git multistage init` creates a stage with a random ID when you would rather not pick one.

## Coding agents

Give each agent a stage ID, usually its own agent ID, along with [AGENTS.md](AGENTS.md). Agents in Claude Code or Codex can skip hunk selection: an editor hook records every edit they make, and one command stages those edits:

```bash
git multistage --stage <id> add --edits && git multistage --stage <id> commit -m "Add input validation"
```

[docs/agents.md](docs/agents.md) covers the hook setup for both tools, how sessions and subagents get their stage IDs, and the opt-in `--agentic` flag.

## Documentation

- [Command reference](docs/cli.md): commands, stage IDs, exit codes, JSON output, hunk IDs, and settings.
- [Coding agents](docs/agents.md): editor hooks for Claude Code and Codex, and `--agentic`.
- [AGENTS.md](AGENTS.md): the short guide to give an agent.
- [Architecture](docs/architecture.md): storage, the commit transaction, the default index, and the invariants.
- [Limitations and recovery](docs/limitations.md): what a shared checkout cannot guarantee, which setups are refused, and how to recover from each failure.

## Contributing

```bash
go test ./...                                                       # unit and integration tests
MULTISTAGE_STRESS=10 go test -run 'Many|Hunks|Native|Kill' ./test/  # repeat the concurrency and crash tests
```

The integration tests run the real binary against temporary repositories. They build it with the `testhooks` tag, which adds the fault injection that the crash and race tests use. CI runs the suite on Linux, macOS, and Windows.

## License

MIT. See [LICENSE](LICENSE).
