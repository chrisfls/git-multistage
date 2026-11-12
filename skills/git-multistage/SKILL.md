---
name: git-multistage
description: Stage and commit with git multistage in a checkout that other agents share, so your commits hold only your own changes. Use before you stage or commit, when a message says git-multistage records your edits in a stage, or when a git multistage command fails.
---

# Using git multistage as an agent

Several agents work in this checkout at the same time. Use `git multistage` instead of `git add`, `git commit`, `git restore --staged`, and `git reset` for staging and committing. Each agent stages into its own stage, so other agents' changes stay out of your commits, and you never wait for them.

## Your stage ID

Use your agent ID as your stage ID, such as `agent-a` from your task. Pass it on every command, because each shell call starts with a fresh environment:

```bash
git multistage --stage agent-a status
```

If your task gives no ID, create a stage once and use its ID from then on:

```bash
git multistage init          # prints: Created stage stage-1f3a9c2e ...
```

If you run as a separate agent session (not a subagent), `--agentic` can take your ID from the environment instead of `--stage`. Subagents share their parent's environment, so a subagent must use `--stage`.

Never use another agent's stage ID.

## Workflow with the editor hook

When a message says that git-multistage records your file edits in a stage, use that stage ID. Edit files as usual, then:

```bash
git multistage --stage <id> add --edits
git multistage --stage <id> diff --cached
git multistage --stage <id> commit -m "Add input validation"
```

`add --edits` stages exactly the edits you made with the file-editing tools (Claude Code's Edit and Write, Codex's apply_patch). Stage files that you changed through shell commands with `add` or `add --hunk`. If an edit is blocked because another agent is editing the file, make the edit again.

## Workflow without the hook

1. Edit your files.
2. Stage a file that only you changed:

   ```bash
   git multistage --stage agent-a add src/feature.ts src/new-file.ts
   ```

3. Stage only your part of a file that other agents also change:

   ```bash
   git multistage --stage agent-a hunks src/shared.ts
   git multistage --stage agent-a add src/shared.ts --hunk 3f2a9c1d0e7a --hunk 88b02c4a91d3
   ```

   Pick the hunks whose `+` and `-` lines are your edits. `staged-by=<stage>` marks hunks that another stage already staged; leave those alone.
4. Check what you will commit:

   ```bash
   git multistage --stage agent-a diff --cached
   ```

5. Commit:

   ```bash
   git multistage --stage agent-a commit -m "Add input validation"
   ```

Add `--json` to `status`, `hunks`, `add`, and `commit` when you parse the output.

## When a command fails

| Exit code | What happened | What to do |
| --- | --- | --- |
| 3 with `edit-lost` or `edit-conflict` | Another writer erased or overlapped your recorded edit. | Redo the edit, or stage the file with `hunks` and `add --hunk`. |
| 3 | Your staged change conflicts with a commit that landed on the branch. Your staged changes are kept. | Read the listed paths. Edit the files so that they include both changes, then `restore --staged <path>`, stage again, and commit. |
| 4 | A hunk ID is stale: the file changed after you listed the hunks. | Run `hunks <path>` again and use the new IDs. |
| 5 with `shared-path` | Another stage has staged changes in that file, so a whole-file `add` could take its changes. | Stage your hunks with `--hunk`. |
| 5, other | HEAD is detached or on another branch, or a merge or rebase is in progress. | Stop and report it. Do not change branches. |
| 6 | A lock stayed held, such as another Git process holding `.git/index.lock`. | Retry the command after a few seconds. If it fails again, run `git multistage doctor` and report the output. |

A commit that fails with `hook-modified` means the pre-commit hook rewrote your files on disk, such as by formatting them. Stage your changes again (`hunks`, then `add --hunk`) and commit again.

A commit that prints `nothing new to commit` found your change already on the branch, because another agent made the same edit. That is a success.

In Codex, the default sandbox makes `.git` read-only, so every `git multistage` command fails with an error such as "Operation not permitted". Run it with escalated permissions, as you would `git commit`.

## Rules

- Do not run `git add`, `git commit`, `git stash`, `git checkout -- <file>`, `git restore <file>`, `git reset --hard`, or `git switch`. They change state that every agent shares.
- Do not edit a file at the same moment as another agent. git-multistage separates staging and commits; it cannot merge two simultaneous writes to the same file on disk.
- `git status`, `git log`, `git diff <commit>`, and `git show` are safe to run.
