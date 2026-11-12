# git-multistage

`git multistage` gives one Git checkout several independent staging areas, called stages. Each stage has its own index and its own staged changes, and commits to the checked-out branch on its own. Stages do not wait for each other, and they need no extra worktrees or branches.

Select a stage with a stage ID. A person can keep separate stages for separate changes; a coding agent uses its agent ID, so several agents can stage and commit in the same checkout at the same time.

The tool is a thin layer over Git plumbing (`GIT_INDEX_FILE`, `merge-tree`, `commit-tree`, `update-ref`). The history it writes is ordinary, linear Git history, which you can push with any Git client.

```bash
git multistage --stage fix-login add src/login.ts
git multistage --stage fix-login commit -m "Fix login redirect"
```

At the same time, in the same checkout:

```bash
git multistage --stage validation hunks src/shared.ts     # list hunks with IDs
git multistage --stage validation add src/shared.ts --hunk 3f2a9c1d0e7a
git multistage --stage validation commit -m "Update validation"
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

1. Choose a stage ID. An ID has 1 to 64 lowercase letters, digits, `.`, `_`, or `-`. Pass it on each command, or set it once for a shell:

   | Shell | Command |
   | --- | --- |
   | any, per command | `git multistage --stage <id> <command>` |
   | bash, zsh | `export GIT_MULTISTAGE_ID=<id>` |
   | PowerShell | `$env:GIT_MULTISTAGE_ID = "<id>"` |
   | cmd.exe | `set GIT_MULTISTAGE_ID=<id>` |

   `--stage` wins over `GIT_MULTISTAGE_ID`. Every command refuses to run without a stage ID. `git multistage init` creates a stage with a new random ID and prints it; `git multistage init <id>` fails when the stage already exists, so two users cannot claim one ID by accident.

2. Edit files. When the file belongs to you alone, stage it whole:

   ```bash
   git multistage add src/a.ts
   ```

3. When other stages also change the file, stage only your hunks:

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

A stage keeps its staged changes until they are committed or unstaged, so a restarted process that uses the same ID finds them where it left them.

## Coding agents

Give each agent its agent ID as its stage ID, and give it [AGENTS.md](AGENTS.md).

### Claude Code editor hook (optional)

With the editor hook, an agent never selects hunks: the hook records each file edit the agent makes, and `add --edits` stages exactly those edits. Two agents can then edit neighbouring lines of one file, and hooked agents take turns writing a file, so no write erases another. Each subagent gets its own stage from its `agent_id`. Add this to the project's `.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [{"matcher": "Edit|MultiEdit|Write", "hooks": [{"type": "command", "command": "git multistage hook claude"}]}],
    "PostToolUse": [{"matcher": "Edit|MultiEdit|Write", "hooks": [{"type": "command", "command": "git multistage hook claude"}]}]
  }
}
```

The first recorded edit tells the agent its stage ID. The agent then commits with:

```bash
git multistage --stage <id> add --edits && git multistage --stage <id> commit -m "Describe the change"
```

`--agentic` is an opt-in shortcut: when no stage ID is given, it takes the calling agent's ID from its environment, `CLAUDE_CODE_SESSION_ID` and then `PASEO_AGENT_ID`, as `claude-<id>` or `paseo-<id>`. In a Claude Code main session, that is the stage the editor hook records into. Without the flag, these variables are ignored. Subagents inherit their parent's environment, so Claude Code subagents of one session get the same ID; with the editor hook they use the stage ID the hook announces, and without it an explicit `--stage`.

## Commands

Every command takes `--stage <id>` and `--agentic`. Most commands take `--json` for machine-readable output. The tool never prompts, except `add -p`.

| Command | What it does |
| --- | --- |
| `init [<id>]` | Create a stage. Without `<id>`, pick a unique ID. Fails if the stage exists. |
| `status` | Show the stage's staged, unstaged, and untracked files, and files that other stages also staged. |
| `diff [--cached] [<git diff args>]` | `git diff` against the stage. `--cached` shows the staged changes against the stage's base commit. |
| `hunks [--cached] [<path>...]` | List hunks with IDs. Without paths, list every changed tracked file. `--cached` lists staged hunks. |
| `add <path>...` | Stage whole files: new, modified, deleted, renamed (pass both paths), binary. `-f` stages ignored files. |
| `add <path>... --hunk <id>...` | Stage only the listed hunks. `-f` stages hunks of an ignored untracked file. |
| `add --edits [<path>...]` | Stage the edits that the editor hook recorded. `-f` stages a recorded new file that `.gitignore` ignores. |
| `add -p <path>...` | Interactive hunk selection for people (`git add -p` on the stage). |
| `apply [-R] <patch>\|-` | Apply a patch to the stage's index, like `git apply --cached`. The working tree is untouched. |
| `restore --staged <path>... [--hunk <id>...]` | Unstage files, or only the listed staged hunks. |
| `commit -m <msg> [-m <msg>] [-F <file>] [--no-verify]` | Commit the staged changes onto the current branch tip. |
| `log [<git log args>]` | Show the commits made by this stage. |
| `list` | List the stages of this worktree. |
| `doctor` | Check the repository, the default index, and every stage. Changes nothing. |
| `sync [--from <commit>]` | Bring the default index up to the branch tip, keeping natively staged entries. `--from` names the commit that the index matches when the tool cannot tell. |
| `drop [--force]` | Delete the stage. Refuses when changes are staged, unless `--force`. |
| `drop --edits [<path>...]` | Forget recorded edits. |
| `hook claude` | Claude Code hook that records file edits; see above. |

### Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success. |
| 1 | Error, or nothing to commit. |
| 2 | Usage error, or no stage ID. |
| 3 | Conflict: the staged changes cannot be replayed safely onto the branch. Nothing was committed. |
| 4 | A hunk ID does not match the current diff. Nothing was staged. |
| 5 | Refused: a detached HEAD, a branch switch, an operation in progress, or a file that another stage also staged. |
| 6 | A lock stayed held until the timeout. |

With `--json`, an error prints `{"error": {"kind": ..., "message": ..., "paths": [...], "hint": ...}}` to stdout.

### Hunk IDs

`git multistage hunks` lists zero-context hunks, so each separate block of changed lines is its own hunk:

```text
diff src/shared.ts (modified)
@@ -10,1 +10,1 @@ id=3f2a9c1d0e7a
-  if (x) {
+  if (x && y) {
@@ -42,0 +43,2 @@ id=88b02c4a91d3 staged-by=validation
+  log(x);
+  log(y);
```

An ID is a hash of the path and the changed lines. It stays the same when other hunks are staged or committed, and it changes when the hunk's content changes. A selection with an unknown ID fails with exit code 4 and stages nothing. `staged-by` names other stages that staged the same hunk.

### Configuration

| Setting | Default | Effect |
| --- | --- | --- |
| `multistage.syncIndex` | `true` | After each commit, update the default index to the new branch tip. |
| `commit.gpgSign`, `user.signingKey`, `gpg.format` | Git defaults | Commits are signed as `git commit` would sign them. |
| `core.hooksPath`, hook config | Git defaults | `pre-commit`, `prepare-commit-msg`, `commit-msg`, and `post-commit` run through `git hook run`. |

## More documentation

- [AGENTS.md](AGENTS.md): instructions for coding agents.
- [LICENSE](LICENSE): MIT.
- [docs/architecture.md](docs/architecture.md): storage, the commit transaction, and the invariants.
- [docs/limitations.md](docs/limitations.md): what the tool cannot guarantee, unsupported configurations, and recovery steps.

## Development

```bash
go test ./...                                   # unit and integration tests
MULTISTAGE_STRESS=10 go test -run 'Many|Hunks|Native|Kill' ./test/   # repeat the adversarial tests
```

The integration tests build the binary with the `testhooks` build tag, which adds fault injection for crash and race tests. CI runs the suite on Linux, macOS, and Windows.

Maintainer: chrisfls.
