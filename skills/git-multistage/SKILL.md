---
name: git-multistage
description: Use before you stage or commit in a Git checkout that other agents share, or when a git multistage command fails.
---

# Staging and committing with git multistage

## Your stage ID

Pass your stage ID on every command, because each shell call starts with a fresh environment. Use the first ID that applies:

1. The stage that a git-multistage message names, such as "git-multistage records your file edits in stage claude-1a2b". The message comes after your first edit with a file-editing tool, so make that edit before you choose an ID.
2. Your agent ID from your task, such as `agent-a`.
3. A new stage. `git multistage init` prints its ID, such as `Created stage stage-1f3a9c2e`.

A separate agent session can pass `--agentic` instead of `--stage <id>` to take its ID from the environment. A subagent shares its parent's environment, so it must pass `--stage`. Never use another agent's stage ID.

## Stage and commit

Stage and commit the edits that a git-multistage message records with:

```bash
git multistage --stage <id> add --edits
git multistage --stage <id> commit -m "Add input validation"
```

`add --edits` stages exactly the edits that you made with your file-editing tools, including the deletions and moves of an apply_patch patch, even when another agent later changed those lines or deleted the file.

Before you commit, stage your other changes by hunk: files that you changed through shell commands, and all your edits when no git-multistage message names your stage. List the hunks of the file, and stage the hunks whose `+` and `-` lines are your edits:

```bash
git multistage --stage <id> hunks src/shared.ts
git multistage --stage <id> add src/shared.ts --hunk 3f2a9c1d0e7a --hunk 88b02c4a91d3
```

Stage a whole file with `add <path>` only when every hunk is yours. Leave alone a hunk marked `staged-by=<stage>`: another stage staged it.

Delete and move tracked files with `rm` and `mv`, which change the working tree and stage the change:

```bash
git multistage --stage <id> rm src/old.ts
git multistage --stage <id> mv src/a.ts src/b.ts
```

`add <path>` stages no deletion or move, because a missing file can be another agent's deletion or move. If you already deleted or moved the file with a shell command, run the same `rm` or `mv`: it stages the change and leaves the disk alone. If the file still exists and differs from your stage, `rm` needs `-f`, because the difference may be another agent's unstaged edit.

When a command fails, follow the `hint:` line of its error.
