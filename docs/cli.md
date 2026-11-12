# Command reference

```text
git multistage [--stage <id> | --agentic] <command> [<args>]
```

Most commands take `--json` for machine-readable output. Nothing prompts for input except `add -p`.

## Stage IDs

Every command except `list`, `doctor`, `sync`, and `hook` works on one stage. The stage ID comes from, in order:

1. `--stage <id>`, before or after the command name.
2. The `GIT_MULTISTAGE_ID` environment variable.
3. With `--agentic` only, the calling agent's ID (see [coding agents](agents.md#--agentic)).

Without any of these, the command exits with code 2. An ID has 1 to 64 characters: lowercase letters, digits, `.`, `_`, and `-`, starting with a letter or digit. IDs are lowercase so that two of them never collide on a case-insensitive file system.

| Shell | Set the stage for the session |
| --- | --- |
| bash, zsh | `export GIT_MULTISTAGE_ID=<id>` |
| PowerShell | `$env:GIT_MULTISTAGE_ID = "<id>"` |
| cmd.exe | `set GIT_MULTISTAGE_ID=<id>` |

A command creates its stage the first time the stage is used. `git multistage init <id>` creates it on purpose and fails if the stage already exists, so two users cannot claim one ID by accident. `git multistage init` without an ID picks a random one and prints it.

Stages belong to a worktree. A repository without linked worktrees has one set of stages; each linked worktree has its own.

## Commands

| Command | What it does |
| --- | --- |
| `init [<id>]` | Create a stage. Without `<id>`, pick a random ID. Fails if the stage exists. |
| `status` | Show the stage's staged, unstaged, and untracked files, recorded edits, and files that other stages also staged. |
| `diff [--cached] [<git diff args>]` | Run `git diff` against the stage. `--cached` compares the staged changes with the stage's base commit. |
| `hunks [--cached] [<path>...]` | List hunks with IDs. Without paths, list every changed tracked file. `--cached` lists staged hunks. |
| `add <path>...` | Stage whole files: new, modified, deleted, renamed (pass both paths), or binary. `-f` stages ignored files. |
| `add <path>... --hunk <id>...` | Stage only the listed hunks. `-f` stages hunks of an ignored untracked file. |
| `add --edits [<path>...]` | Stage the edits that an editor hook recorded. `-f` stages a recorded new file that `.gitignore` ignores. |
| `add -p <path>...` | Pick hunks interactively, as `git add -p` does. |
| `apply [-R] <patch>\|-` | Apply a patch to the stage's index, like `git apply --cached`. The working tree is not touched. |
| `restore --staged <path>... [--hunk <id>...]` | Unstage files, or only the listed staged hunks. |
| `commit -m <msg> [-m <msg>] [-F <file>] [--no-verify]` | Commit the staged changes onto the current branch tip. |
| `log [<git log args>]` | Show the commits that this stage made. |
| `list` | List the stages of this worktree. |
| `doctor` | Check the repository, the default index, and every stage. Changes nothing. |
| `sync [--from <commit>]` | Bring the default index up to the branch tip, keeping entries that were staged by hand. `--from` names the commit that the index matches when the tool cannot tell. |
| `drop [--force]` | Delete the stage. Refuses while changes are staged, unless `--force`. |
| `drop --edits [<path>...]` | Forget recorded edits. |
| `hook claude`, `hook codex` | Editor hooks that record file edits; see [coding agents](agents.md). |

## Hunk IDs

`hunks` lists zero-context hunks, so each block of changed lines is a hunk of its own:

```text
diff src/shared.ts (modified)
@@ -2,1 +2,1 @@ id=d94c87b26e90 staged-by=validation
-  if (!input) return false;
+  if (!input || input.length > 256) return false;
@@ -7,1 +7,1 @@ id=3c16c257727c
-  return input.trim();
+  return input.trim().toLowerCase();
```

An ID is a hash of the path and the changed lines. It stays the same while other hunks are staged or committed, and changes when the hunk's content changes. When two hunks of a file make the same change, their IDs also cover their positions. `staged-by` names other stages that staged the same hunk.

A selection that names an unknown ID fails with exit code 4 and stages nothing. That happens when the file changed after the hunks were listed: run `hunks` again and use the new IDs.

Edits on adjacent lines, with no unchanged line between them, form one hunk, and so does a new file. Editor hooks record those edits separately (`add --edits`); without a hook, stage part of such a hunk with `apply`.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success. |
| 1 | Error, or nothing to commit. |
| 2 | Usage error, or no stage ID. |
| 3 | Conflict: the staged changes cannot be replayed onto the branch, or recorded edits do not fit the stage. Nothing was committed or staged. |
| 4 | A hunk ID does not match the current diff. Nothing was staged. |
| 5 | Refused: HEAD is not where the stage commits, an operation in progress (a rebase, a cherry-pick sequence, or a merge that changed the same paths) blocks the commit, the stage already exists (`init`), or another stage has staged the same file. |
| 6 | A lock stayed held until the timeout. |

[Limitations and recovery](limitations.md#recovery) lists what to do for each.

## JSON output

With `--json`, `status`, `hunks`, `add`, `commit`, `list`, `doctor`, `sync`, `drop`, and `init` print one JSON object to stdout; most include a `warnings` array. A failure prints an error object and exits with its code:

```json
{"error": {"kind": "stale-hunk", "message": "hunk IDs do not match the current diff; nothing was changed", "details": ["unknown hunk d94c87b26e90"], "hint": "..."}}
```

Scripts should branch on the exit code and `kind`; `message` and `hint` are written for people.

## Settings

| Setting | Default | Effect |
| --- | --- | --- |
| `multistage.syncIndex` | `true` | After each commit, bring the default index up to the new branch tip. |
| `commit.gpgSign`, `user.signingKey`, `gpg.format` | Git's defaults | Commits are signed as `git commit` would sign them. |
| `core.hooksPath` and hook configuration | Git's defaults | `pre-commit`, `prepare-commit-msg`, `commit-msg`, and `post-commit` run through `git hook run`. |

Author and committer come from Git's configuration and the `GIT_AUTHOR_*` and `GIT_COMMITTER_*` variables.
