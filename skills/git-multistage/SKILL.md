---
name: git-multistage
description: Stage and commit with git multistage in a checkout that other agents share. Use before you stage or commit, when a git-multistage message names your stage, and when a git multistage command fails.
---

# Using git multistage as an agent

Other agents edit files and commit to this branch while you work. Commits, file changes, and stages that you did not make are theirs: do not review, revert, stage, or mention them unless they block your task. Stage and commit your own changes with `git multistage`, so that other agents' changes stay out of your commits.

## Your stage ID

Pass your stage ID on every command, because each shell call starts with a fresh environment. Use the first ID that applies:

1. The stage that a git-multistage message names, such as "git-multistage records your file edits in stage claude-1a2b". The message comes after your first edit with a file-editing tool, so make that edit before you choose an ID.
2. Your agent ID from your task, such as `agent-a`.
3. A new stage. `git multistage init` prints its ID, such as `Created stage stage-1f3a9c2e`. Use that ID from then on.

A separate agent session can pass `--agentic` instead of `--stage <id>` to take its ID from the environment. A subagent shares its parent's environment, so it must pass `--stage`. Never use another agent's stage ID.

## Stage and commit

When a git-multistage message says that it records your file edits, stage and commit them with:

```bash
git multistage --stage <id> add --edits
git multistage --stage <id> diff --cached
git multistage --stage <id> commit -m "Add input validation"
```

`add --edits` stages exactly the edits that you made with the file-editing tools (Claude Code's Edit and Write, Codex's apply_patch), including apply_patch's deletions and moves. It stages what you wrote, even when another agent later changed those lines or deleted the file.

Before you commit, stage other changes by path: files that you changed through shell commands, and all your edits when no git-multistage message names your stage.

```bash
git multistage --stage <id> add src/feature.ts
git multistage --stage <id> hunks src/shared.ts
git multistage --stage <id> add src/shared.ts --hunk 3f2a9c1d0e7a --hunk 88b02c4a91d3
```

Use `add <path>` for a file that only you changed. In a file that other agents also change, list the hunks and stage the hunks whose `+` and `-` lines are your edits. `staged-by=<stage>` marks a hunk that another stage already staged; leave it alone.

Delete and move tracked files with `rm` and `mv`, which change the working tree and stage the change:

```bash
git multistage --stage <id> rm src/old.ts
git multistage --stage <id> mv src/a.ts src/b.ts
```

`add <path>` stages no deletions and no moves, because a missing file can be another agent's deletion or move. If you already deleted or moved the file with a shell command, run the same `rm` or `mv`: it stages the change and leaves the disk alone. If the file still exists and differs from your stage, `rm` needs `-f`, because the difference may be another agent's unstaged edit.

Add `--json` to `status`, `hunks`, `add`, and `commit` when you parse the output.

## When a command fails

Most errors print a `hint:` line, or a `hint` field with `--json`, that says how to recover. Follow it, except in these cases:

| Failure | What to do |
| --- | --- |
| Exit 5: another stage has staged changes in the file (`shared-path`) | Stage your hunks with `hunks` and `add --hunk`. |
| Exit 5: HEAD is detached or on another branch, or a merge or rebase is in progress | Stop and report it. Do not change branches. |
| Exit 6: a lock stayed held, such as `.git/index.lock` | Retry after a few seconds. If it fails again, run `git multistage doctor` and report its output. |
| `hook-modified`: the pre-commit hook rewrote your files on disk, such as by formatting them | Stage your changes again with `hunks` and `add --hunk`, and commit again. |

A commit that prints `nothing new to commit` found your change already on the branch, because another agent made the same edit. That is a success.

In Codex, the default sandbox makes `.git` read-only, so every `git multistage` command fails with an error such as "Operation not permitted". Run it with escalated permissions, as you would `git commit`.

## Rules

- Do not run `git add`, `git commit`, `git rm`, `git mv`, `git restore`, `git reset`, `git stash`, `git checkout -- <file>`, or `git switch`. They change state that every agent shares.
- Do not edit a file at the same moment as another agent. git-multistage cannot merge two simultaneous writes to one file on disk.
- `git status`, `git log`, `git diff <commit>`, and `git show` are safe to run.
