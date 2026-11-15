# git-multistage

**Separate staging areas in one Git checkout.**

git-multistage lets you work on several changes in the same checkout and commit them independently. Each named stage has its own Git index, so staging one change doesn't add it to another stage. All stages use the same working tree; each commits to the branch checked out when that stage was created.

```bash
git multistage --stage feature-a add src/a.ts
git multistage --stage feature-b add src/b.ts

git multistage --stage feature-a commit -m "Add feature A"
git multistage --stage feature-b commit -m "Add feature B"
```

Both commits appear in the branch's normal Git history. Neither stage includes the other stage's changes.

This is useful when you're juggling unrelated changes or running multiple coding agents in one checkout. With plain Git, those changes compete for the same index. Worktrees give them separate working directories and branches, which is useful when they need isolation but adds another checkout to manage.

## Install

Requires Git 2.45 or newer. Install with [mise](https://mise.jdx.dev):

```bash
mise use -g github:chrisfls/git-multistage
git multistage doctor
```

You can also [download a release](https://github.com/chrisfls/git-multistage/releases), [install with Go](#other-install-methods), or [build from source](#other-install-methods).

## Using stages

A stage is created when you first use its name:

```bash
git multistage --stage fix-login add src/login.ts
git multistage --stage fix-login diff --cached
git multistage --stage fix-login commit -m "Honor returnTo after login"
```

Only changes staged in `fix-login` go into that commit. Other modifications stay in the working tree or in their own stages.

### Stage part of a file

If two changes touch the same file, list its hunks and select the ones you want:

```console
$ git multistage --stage validation hunks src/shared.ts
@@ -2,1 +2,1 @@ id=d94c87b26e90
-  if (!input) return false;
+  if (!input || input.length > 256) return false;
@@ -7,1 +7,1 @@ id=3c16c257727c
-  return input.trim();
+  return input.trim().toLowerCase();
```

Stage only the first hunk, using its ID from the output:

```bash
git multistage --stage validation add src/shared.ts --hunk d94c87b26e90
git multistage --stage validation commit -m "Cap input length"
```

The lowercase conversion remains uncommitted. The hunk ID shown here is an example; use the ID printed for your diff.

### Set a default stage

Set `GIT_MULTISTAGE_ID` to stop repeating `--stage`:

```bash
export GIT_MULTISTAGE_ID=fix-login
git multistage add src/login.ts
git multistage commit -m "Fix login"
```

In PowerShell, use `$env:GIT_MULTISTAGE_ID = "fix-login"`.

Run `git multistage init` to create a stage with a randomly generated ID.

## What is shared?

Stages have separate indexes, but they do **not** have separate working trees.

- `add <path>` stages the file's current contents, including edits made by other people or agents. Use `add --hunk` when you need only part of a file.
- `add` doesn't pick up deletions or moves. Stage those explicitly with `rm` or `mv`.
- Builds and tests see all uncommitted changes in the checkout, regardless of stage.
- Each stage commits to the branch checked out when that stage was created.

If you need isolated files, builds, or tests, use `git worktree add` instead.

## Coding agents

Multiple agents can share a checkout and commit their work separately. Give each agent its own stage, and have it follow the [git-multistage skill](skills/git-multistage/SKILL.md) rather than using Git commands that modify shared state.

An optional plugin records edits made through each agent's file tools. With that record, an agent can stage only its own edits, even when several agents work on the same file:

```bash
git multistage --stage agent-a add --edits
git multistage --stage agent-a commit -m "Add input validation"
```

The plugin also provides shared-checkout rules and blocks commands such as `git commit` and `git stash` that could interfere with other agents. Its hooks run only in checkouts that opt in:

```bash
git config multistage.shared true
```

Shell commands, including formatters, are not recorded as agent edits. Use `add`, `add --hunk`, `rm`, or `mv` for those changes.

See [Agent setup](AGENT_SETUP.md) for plugin installation in Claude Code, Codex, and other coding agents, plus the rules to use without a plugin.

## Other install methods

The directory containing `git-multistage` must be on your `PATH`.

**Release archive:** Download the archive for your OS and architecture and `checksums.txt` from [Releases](https://github.com/chrisfls/git-multistage/releases). Linux, macOS, and Windows builds are available for amd64 and arm64. Verify the checksum before installing the executable.

**Go:**

```bash
go install github.com/chrisfls/git-multistage/cmd/git-multistage@latest
```

Go installs the binary in `GOBIN`, or `$(go env GOPATH)/bin` if `GOBIN` is unset.

**From source:**

```bash
git clone https://github.com/chrisfls/git-multistage
cd git-multistage
go build -o "$HOME/.local/bin/git-multistage" ./cmd/git-multistage
```

On Windows, use `go build -o "$env:USERPROFILE\bin\git-multistage.exe" .\cmd\git-multistage` in PowerShell.

## Development

The [specs](openspec/specs/) describe stages, commits, crash recovery, the CLI, and agent integrations.

To contribute, clone the repository and run `scripts/setup.sh`. It requires [mise](https://mise.jdx.dev) and [RuleSync](https://github.com/dyoshikawa/rulesync), and generates local agent configuration. Changes start under `openspec/changes/` using [OpenSpec](https://openspec.dev), as described in `AGENTS.md`.

Before pushing, run:

```bash
mise run ci
mise run ci:stress
```

`ci` runs formatting, linting, vetting, tests, and OpenSpec validation. `ci:stress` repeats the concurrency and crash tests. The CI workflow runs from the Actions tab and covers Linux, macOS, and Windows. You can run the Linux job locally with `act` and Docker.

## License

MIT. See [LICENSE](LICENSE).
