# Design

## Context

RuleSync installs the six core OpenSpec skills, pinned to `v2.0.0`. `openspec/` holds `config.yaml` and nothing else. `AGENTS.md`, generated from `.rulesync/rules/overview.md`, asks agents to plan changes to behavior with OpenSpec, but it does not require a plan or an approval. Several agents commit to `master` from one checkout and push after each commit, so the repository has no pull request in which a reviewer could stop unplanned work. `README.md` and `docs/` describe today's behavior in detail and are the main source for the baseline.

## Goals / Non-Goals

**Goals:**

- Record today's behavior as nine capability specs that a reviewer can check against the code and tests.
- Make an approved OpenSpec change a precondition for every edit, in the instructions that Claude Code and Codex read.
- Catch malformed specs and changes in CI.

**Non-Goals:**

- No change to program behavior or Go code.
- No mechanical check in CI that each commit belongs to a change. Agents commit straight to `master`, so the gate is the instruction and the user's approval.
- No rewrite of `README.md` or `docs/`.

## Decisions

### The baseline lands through this change

The baseline specs are ADDED deltas of this change. Archiving the change creates `openspec/specs/`. The other way, writing `openspec/specs/` by hand, would skip the review that the new rule requires for every change. Going through a change also gives the user one place to approve the baseline.

### Nine capabilities, cut by what the user sees

The capabilities follow the user's view of the tool: stages, staging, commit, default-index sync, crash recovery, editor hooks, agent stage detection, CLI conventions, and plugins. They do not follow Go packages, which can change without a change in behavior. Each spec holds observable behavior only: commands, flags, exit codes, files that Git or a user can see, and messages. Storage layout, lock files, and algorithms stay in `docs/architecture.md`.

### Code wins over docs in the baseline

The specs come from `README.md` and `docs/`. Before archive, each requirement is checked against the code and the tests. Where the docs and the code disagree, the spec records what the code does, and the docs are fixed in the same task. Where the intended behavior is unclear, the requirement is left out, and the question is listed for the user. The baseline describes the tool as it is, so it can introduce no new behavior.

### Specs are the contract, and docs stay for users

`openspec/specs/` becomes the source of truth for behavior. `README.md` and `docs/` stay as user documentation. A `tasks` rule in `openspec/config.yaml` makes each change that alters behavior update both in the same task group. Moving all of `docs/` into specs would remove the user guide, and the spec format is not written for people who install the tool.

### The gate lives in AGENTS.md

`.rulesync/rules/overview.md` gets a rule, and RuleSync copies it into `AGENTS.md`, which both Claude Code and Codex read:

- Every change to the repository needs an OpenSpec change first. This covers code, tests, docs, CI, and tooling.
- A change that alters no behavior sets `skip_specs: true` and still has a proposal and tasks.
- No edit outside `openspec/changes/<name>/` starts before the user approves that change's artifacts.
- An agent archives a change only after its tasks are done.
- OpenSpec artifacts take precedence over the plan and spec files of other skills, such as Superpowers' `docs/superpowers/`. Those files are not written in this repository.

The OpenSpec skills already stop after planning and wait for a new request, so the rule matches them.

### CI validates with the pinned CLI

A new `openspec` job in `.github/workflows/ci.yml` installs `@fission-ai/openspec@2.0.0`, the version that `rulesync.jsonc` pins, on Node 22, and runs `openspec validate --all --strict` with `OPENSPEC_NO_UPDATE_CHECK=1`. Strict mode turns format warnings into failures, such as a scenario with no WHEN or THEN step. An upgrade bumps the CLI version and the RuleSync `ref` together.

## Risks / Trade-offs

- [The baseline misstates behavior that the docs describe wrongly] → The code cross-check before archive, and the user's review of the specs.
- [A small fix, such as a typo, now needs a proposal and tasks] → `skip_specs: true` keeps such a change to two short files. The user asked for every change to be planned.
- [Agents with work in progress started it without a change] → The rule applies to work that starts after it lands. Uncommitted work in the checkout at that time is reported to the user and is not rewritten.
- [The npm install makes CI slower] → One extra job of about 20 seconds that runs in parallel with the others.
- [Specs and docs drift apart] → The `tasks` rule in `config.yaml`, and the review of each change.

## Migration Plan

1. Land the rule, the `config.yaml` rules, and the CI job.
2. Cross-check the specs against the code, and fix the docs where they are wrong.
3. Archive the change, which writes `openspec/specs/`.

To roll back, revert the commits. The specs are files only, and no code depends on them.
