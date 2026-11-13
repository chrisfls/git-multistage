# Proposal

## Why

OpenSpec is installed, but `openspec/specs/` is empty, and agents may still change code without a plan. The behavior is written down only in `docs/`, which every change must keep in step by hand. Agents should plan every change against one contract of current behavior. So the specs replace the docs, and an approved OpenSpec change is required before any edit.

## What Changes

- Add baseline specs that record the behavior that git-multistage has today. The sources are `README.md`, `docs/`, and the code and tests. Where they disagree, the code wins.
- **BREAKING** for readers of the docs: remove `docs/` (`agent-guide.md`, `agents.md`, `architecture.md`, `cli.md`, and `limitations.md`). From then on, behavior is written only in specs, and no new docs are written.
- Keep `README.md` as the repository's landing page: what the tool does, how to install it, a quick start, the plugin install, and links to `openspec/specs/`.
- Make `skills/git-multistage/SKILL.md` the only copy of the agent guide. The plugin test stops comparing it with `docs/agent-guide.md`.
- Require an OpenSpec change, with its proposal, specs (or `skip_specs: true`), design when needed, and tasks, before any change to the repository. The user must approve the artifacts before implementation starts. This covers code, tests, CI, and tooling.
- Validate every spec and change in CI with `openspec validate --all --strict`.
- Give OpenSpec artifacts precedence over the plan and spec files that other skills write, such as Superpowers' `docs/superpowers/` plans.

## Capabilities

### New Capabilities

- `stages`: stage IDs and how a command selects one, creation, listing, deletion, per-worktree scope, and keeping a stage current with the branch tip.
- `staging`: staging whole files, hunks by ID, interactive hunks, and patches; unstaging; and inspecting a stage with `status`, `diff`, `hunks`, and `log`.
- `commit`: the commit transaction: target branch, refusals, message handling, Git hooks, signing, concurrent commits, and verification of the committed tree.
- `default-index-sync`: keeping Git's default index in step with stage commits, and the `sync` command.
- `crash-recovery`: the pending-commit journal, damaged stages, locks and their timeouts, and protection of staged objects from garbage collection.
- `editor-hooks`: the Claude Code and Codex edit hooks, file leases, recorded edits, and `add --edits` and `drop --edits`.
- `agent-stage-detection`: `--agentic`, which takes a stage ID from an agent's environment.
- `cli`: shared command conventions: exit codes, JSON output, non-interactive use, Git version and repository requirements, and `doctor`.
- `agent-plugins`: the Claude Code and Codex plugins that install the editor hooks and the `git-multistage` skill.

### Modified Capabilities

None. `openspec/specs/` is empty.

## Impact

- `openspec/specs/`: nine capability specs, created when this change is archived.
- `docs/`: deleted.
- `README.md`: links point to `openspec/specs/` and `skills/git-multistage/SKILL.md` in place of `docs/`.
- `test/plugin_test.go`: checks the skill's front matter in place of comparing it with the guide.
- `.rulesync/rules/overview.md` and the generated `AGENTS.md`: the rule that requires an approved change, and a pointer to the skill in place of the guide.
- `openspec/config.yaml`: artifact rules that keep behavior in specs and out of new docs.
- `.github/workflows/ci.yml`: a job that installs the pinned OpenSpec CLI and validates `openspec/`.
- No change to the program's behavior.
