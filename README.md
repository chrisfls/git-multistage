# git-multistage

`git multistage` lets several coding agents stage and commit in one Git checkout at the same time. Each agent stages into its own index, and commits to the checked-out branch whenever it finishes. Agents do not wait for each other, and they need no extra worktrees or branches.

The tool is a thin layer over Git plumbing (`GIT_INDEX_FILE`, `merge-tree`, `commit-tree`, `update-ref`). The history it writes is ordinary, linear Git history, which you can push with any Git client.

```bash
export GIT_MULTISTAGE_ID=agent-a
git multistage add src/a.ts
git multistage commit -m "Implement A"
```

At the same time, in the same checkout:

```bash
export GIT_MULTISTAGE_ID=agent-b
git multistage hunks src/shared.ts            # list hunks with IDs
git multistage add src/shared.ts --hunk 3f2a9c1d0e7a
git multistage commit -m "Update validation"
```

Both commits land on the branch, each with only its own changes.

## Install

Requirements: Git 2.45 or newer, and Go 1.22 or newer to build.

```bash
git clone <repository> git-multistage
cd git-multistage
go build -o "$HOME/.local/bin/git-multistage" ./cmd/git-multistage
```

Put the binary in a directory on your `PATH`. Git then runs it as `git multistage`. On Windows, build `git-multistage.exe` and put it in a directory on `%PATH%`:

```powershell
go build -o "$env:USERPROFILE\bin\git-multistage.exe" .\cmd\git-multistage
```

Check the installation with `git multistage doctor` inside a repository.

## Getting started

1. Give each agent its own ID. An ID has 1 to 64 lowercase letters, digits, `.`, `_`, or `-`.

   | Shell | Command |
   | --- | --- |
   | bash, zsh | `export GIT_MULTISTAGE_ID=agent-a` |
   | PowerShell | `$env:GIT_MULTISTAGE_ID = "agent-a"` |
   | cmd.exe | `set GIT_MULTISTAGE_ID=agent-a` |
   | any, per command | `git multistage --agent agent-a <command>` |

   When no ID is available, `git multistage init` creates a stage with a new random ID and prints it. `git multistage init <id>` fails when the stage already exists, so two agents cannot claim the same ID by accident. Every command refuses to run without an ID.

2. Edit files. When the file belongs to you alone, stage it whole:

   ```bash
   git multistage add src/a.ts
   ```

3. When other agents also edit the file, stage only your hunks:

   ```bash
   git multistage hunks src/shared.ts
   git multistage add src/shared.ts --hunk <id> --hunk <id>
   ```

4. Check and commit:

   ```bash
   git multistage status
   git multistage diff --cached
   git multistage commit -m "Describe the change"
   ```

A restarted agent that uses the same ID finds its staged changes where it left them.

For Claude Code and other agents, give them [AGENTS.md](AGENTS.md).

## Commands

Every command takes `--agent <id>`. Most commands take `--json` for machine-readable output. The tool never prompts, except `add -p`.

| Command | What it does |
| --- | --- |
| `init [<id>]` | Create a stage. Without `<id>`, pick a unique ID. Fails if the stage exists. |
| `status` | Show the stage's staged, unstaged, and untracked files, and files that other agents also staged. |
| `diff [--cached] [<git diff args>]` | `git diff` against the stage. `--cached` shows the staged changes against the stage's base commit. |
| `hunks [--cached] [<path>...]` | List hunks with IDs. Without paths, list every changed tracked file. `--cached` lists staged hunks. |
| `add <path>...` | Stage whole files: new, modified, deleted, renamed (pass both paths), binary. `-f` stages ignored files. |
| `add <path>... --hunk <id>...` | Stage only the listed hunks. `-f` stages hunks of an ignored untracked file. |
| `add -p <path>...` | Interactive hunk selection for people (`git add -p` on the stage). |
| `apply [-R] <patch>\|-` | Apply a patch to the stage's index, like `git apply --cached`. The working tree is untouched. |
| `restore --staged <path>... [--hunk <id>...]` | Unstage files, or only the listed staged hunks. |
| `commit -m <msg> [-m <msg>] [-F <file>] [--no-verify]` | Commit the staged changes onto the current branch tip. |
| `log [<git log args>]` | Show the commits made by this stage. |
| `list` | List the stages of this worktree. |
| `doctor` | Check the repository, the default index, and every stage. Changes nothing. |
| `sync [--from <commit>]` | Bring the default index up to the branch tip, keeping natively staged entries. `--from` names the commit that the index matches when the tool cannot tell. |
| `drop [--force]` | Delete the stage. Refuses when changes are staged, unless `--force`. |

### Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success. |
| 1 | Error, or nothing to commit. |
| 2 | Usage error, or no agent ID. |
| 3 | Conflict: the staged changes cannot be replayed safely onto the branch. Nothing was committed. |
| 4 | A hunk ID does not match the current diff. Nothing was staged. |
| 5 | Refused: a detached HEAD, a branch switch, an operation in progress, or a file that another agent also staged. |
| 6 | A lock stayed held until the timeout. |

With `--json`, an error prints `{"error": {"kind": ..., "message": ..., "paths": [...], "hint": ...}}` to stdout.

### Hunk IDs

`git multistage hunks` lists zero-context hunks, so each separate block of changed lines is its own hunk:

```text
diff src/shared.ts (modified)
@@ -10,1 +10,1 @@ id=3f2a9c1d0e7a
-  if (x) {
+  if (x && y) {
@@ -42,0 +43,2 @@ id=88b02c4a91d3 staged-by=agent-b
+  log(x);
+  log(y);
```

An ID is a hash of the path and the changed lines. It stays the same when other hunks are staged or committed, and it changes when the hunk's content changes. A selection with an unknown ID fails with exit code 4 and stages nothing. `staged-by` names other agents that staged the same hunk.

### Configuration

| Setting | Default | Effect |
| --- | --- | --- |
| `multistage.syncIndex` | `true` | After each commit, update the default index to the new branch tip. |
| `commit.gpgSign`, `user.signingKey`, `gpg.format` | Git defaults | Commits are signed as `git commit` would sign them. |
| `core.hooksPath`, hook config | Git defaults | `pre-commit`, `prepare-commit-msg`, `commit-msg`, and `post-commit` run through `git hook run`. |

## More documentation

- [AGENTS.md](AGENTS.md): instructions for coding agents.
- [docs/architecture.md](docs/architecture.md): storage, the commit transaction, and the invariants.
- [docs/limitations.md](docs/limitations.md): what the tool cannot guarantee, unsupported configurations, and recovery steps.

## Development

```bash
go test ./...                                   # unit and integration tests
MULTISTAGE_STRESS=10 go test -run 'Many|Hunks|Native|Kill' ./test/   # repeat the adversarial tests
```

The integration tests build the binary with the `testhooks` build tag, which adds fault injection for crash and race tests. CI runs the suite on Linux, macOS, and Windows.

Maintainer: chrisfls.
