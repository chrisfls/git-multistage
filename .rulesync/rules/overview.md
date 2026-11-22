---
root: true
targets: ["*"]
description: How an agent commits and pushes its work in the git-multistage repository.
---

# Working in the git-multistage repository

Several agents work in this checkout at the same time. Stage, commit, and push your work with `git multistage`, as the `git-multistage` skill in [skills/git-multistage/SKILL.md](skills/git-multistage/SKILL.md) describes. The skill gives your stage ID, the staging commands, and what to do when a command fails.

If `git multistage` is missing, install it from this checkout:

```bash
go install ./cmd/git-multistage
```

## Plan every change with OpenSpec

Every change to this repository starts as an OpenSpec change in `openspec/changes/`. This covers code, tests, CI, and tooling. The specs of current behavior are in `openspec/specs/`.

- Plan the change with the `openspec-propose` skill: a proposal, spec deltas, a design when needed, and tasks. A change that alters no behavior sets `skip_specs: true` in its `.openspec.yaml` and still has a proposal and tasks.
- Show the artifacts to the user, and stop. Do not edit any file outside `openspec/changes/<name>/` before the user approves the change.
- After the approval, implement the tasks with `openspec-apply-change`. Archive the change with `openspec-archive-change` only after every task is done.
- Write behavior in specs only. Do not add documentation files. `README.md` holds only the overview, the install, the quick start, the plugin install, the contributing commands, and links to the specs.
- OpenSpec artifacts take precedence over the plan and spec files of other skills, such as Superpowers' `docs/superpowers/`. Do not write those files here.

The skills run the `openspec` CLI, which `mise.toml` pins to the same version as the skills' ref in `rulesync.jsonc`. Run `mise install` to install it.

## Commit often

- Commit each step as soon as it is finished and its checks pass. Keep one change in each commit, such as a fix and its test.
- Before you commit Go code, run the checks of the `git-multistage-development` skill.
- Stage and commit only through your own stage:

  ```bash
  git multistage --stage <id> add --edits        # or: add <path>, add <path> --hunk <id>
  git multistage --stage <id> diff --cached
  git multistage --stage <id> commit -m "fix: keep warnings in --json output"
  ```

- Do not run `git add`, `git commit`, `git stash`, `git checkout -- <file>`, `git restore <file>`, `git reset --hard`, or `git switch`. They change state that every agent shares.

## Push often

- Push after each commit: `git push`.
- Do not force-push.
- If the push is rejected because the remote moved, stop and report it. Do not run `git pull`, `git merge`, or `git rebase`: they rewrite files that other agents are editing.
- If the repository has no remote, report it with the commit, and continue.
