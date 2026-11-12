# Proposal

## Why

OpenSpec is installed, but `openspec/specs/` is empty, and agents may still change code without a plan. Agents should plan every change against a written contract of current behavior. So the repository needs baseline specs, and a rule that requires an approved OpenSpec change before any edit.

## What Changes

- Add baseline specs that record the behavior that git-multistage has today. The sources are `README.md`, `docs/`, and the code and tests.
- Require an OpenSpec change, with its proposal, specs (or `skip_specs: true`), design when needed, and tasks, before any change to the repository. The user must approve the artifacts before implementation starts. This covers code, tests, docs, CI, and tooling.
- Make the specs the source of truth for behavior. `README.md` and `docs/` stay as user documentation, and a change that alters behavior updates both.
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
- `agent-plugins`: the Claude Code and Codex plugins that install the editor hooks and the agent guide skill.

### Modified Capabilities

None. `openspec/specs/` is empty.

## Impact

- `openspec/specs/`: nine capability specs, created when this change is archived.
- `.rulesync/rules/overview.md` and the generated `AGENTS.md`: the rule that requires an approved change.
- `openspec/config.yaml`: artifact rules that keep docs and specs in step.
- `.github/workflows/ci.yml`: a job that installs the pinned OpenSpec CLI and validates `openspec/`.
- No change to the Go code or to the program's behavior.
