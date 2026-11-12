---
root: true
targets: ["codexcli"]
description: How an agent commits and pushes its work in the git-multistage repository.
---

# Working in the git-multistage repository

Several agents work in this checkout at the same time. Stage, commit, and push your work with `git multistage`, as [docs/agent-guide.md](docs/agent-guide.md) describes. The guide gives your stage ID, the staging commands, and what to do when a command fails.

If `git multistage` is missing, install it from this checkout:

```bash
go install ./cmd/git-multistage
```

## Plan changes with OpenSpec

Plan each change to behavior as an OpenSpec change in `openspec/changes/`, and keep the specs of current behavior in `openspec/specs/`. Use the `openspec-*` skills: `openspec-propose` to plan, `openspec-apply-change` to implement, and `openspec-archive-change` when the change ships. The skills run the `openspec` CLI. Install it with `npm install -g @fission-ai/openspec@2.0.0`, the version that `rulesync.jsonc` pins.

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
