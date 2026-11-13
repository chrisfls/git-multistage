# Design

## Context

RuleSync installs the six core OpenSpec skills, pinned to `v2.0.0`. `openspec/` holds `config.yaml` and nothing else. `AGENTS.md`, generated from `.rulesync/rules/overview.md`, asks agents to plan changes to behavior with OpenSpec, but it does not require a plan or an approval. It also sends agents to `docs/agent-guide.md`. Several agents commit to `master` from one checkout and push after each commit, so the repository has no pull request in which a reviewer could stop unplanned work.

`docs/` describes today's behavior in five files. `test/plugin_test.go` requires `skills/git-multistage/SKILL.md` to hold the text of `docs/agent-guide.md` after its front matter. `README.md` links to `docs/` in seven places.

## Goals / Non-Goals

**Goals:**

- Record today's behavior as nine capability specs that a reviewer can check against the code and tests.
- Replace `docs/` with the specs, so behavior is written in one place.
- Make an approved OpenSpec change a precondition for every edit, in the instructions that agents read.
- Catch malformed specs and changes in CI.

**Non-Goals:**

- No change to program behavior.
- No mechanical check in CI that each commit belongs to a change. Agents commit straight to `master`, so the gate is the instruction and the user's approval.
- No rewrite of `README.md` beyond its links and the content that only `docs/` held.

## Decisions

### The baseline lands through this change

The baseline specs are ADDED deltas of this change. Archiving the change creates `openspec/specs/`. Writing `openspec/specs/` by hand would skip the review that the new rule requires for every change. Going through a change also gives the user one place to approve the baseline.

### Nine capabilities, cut by what the user sees

The capabilities follow the user's view of the tool: stages, staging, commit, default-index sync, crash recovery, editor hooks, agent stage detection, CLI conventions, and plugins. They do not follow Go packages, which can change without a change in behavior. Each spec holds observable behavior only: commands, flags, exit codes, files that Git or a user can see, and messages.

### Specs replace the docs

`docs/` is deleted after the specs are checked against the code. From then on:

- Behavior is written only in `openspec/specs/`, through the deltas of a change.
- The design of internals goes into the `design.md` of the change that alters it. Archived changes keep their design under `openspec/changes/archive/`.
- `README.md` stays as the landing page. It holds what the tool does, the install, a quick start, the plugin install with the Codex sandbox rule, the contributing commands, and links to the specs. It changes only when one of those changes.
- No other documentation file is written. The `config.yaml` rules tell each artifact to put behavior in specs and to add no docs.

The text of `docs/architecture.md`, such as the storage layout, the lock table, and the commit algorithm, stays in Git history and in code comments. The invariants that a user can observe are requirements in the `commit`, `default-index-sync`, `crash-recovery`, and `cli` specs.

### The skill is the agent guide

`skills/git-multistage/SKILL.md` already holds the guide's full text, and the plugin ships it. It becomes the only copy. `AGENTS.md` points to it, `README.md` links to it, and `test/plugin_test.go` checks its front matter in place of comparing it with `docs/agent-guide.md`.

### Code wins over docs in the baseline

The specs come from `README.md` and `docs/`. Before `docs/` is deleted, each requirement is checked against the code and the tests. Where the docs and the code disagree, the spec records what the code does. Where the intended behavior is unclear, the requirement is left out, and the question is listed for the user. The baseline describes the tool as it is, so it introduces no new behavior.

### The gate lives in AGENTS.md

`.rulesync/rules/overview.md` gets a rule, and RuleSync copies it into `AGENTS.md`, which the coding agents read:

- Every change to the repository needs an OpenSpec change first. This covers code, tests, CI, and tooling.
- A change that alters no behavior sets `skip_specs: true` and still has a proposal and tasks.
- No edit outside `openspec/changes/<name>/` starts before the user approves that change's artifacts.
- An agent archives a change only after its tasks are done.
- Behavior goes into specs, and no documentation file is added.
- OpenSpec artifacts take precedence over the plan and spec files of other skills, such as Superpowers' `docs/superpowers/`. Those files are not written in this repository.

The OpenSpec skills already stop after planning and wait for a new request, so the rule matches them.

### CI validates with the pinned CLI

A new `openspec` job in `.github/workflows/ci.yml` installs `@fission-ai/openspec@2.0.0`, the version that `rulesync.jsonc` pins, on Node 22, and runs `openspec validate --all --strict` with `OPENSPEC_NO_UPDATE_CHECK=1`. Strict mode turns format warnings into failures, such as a scenario with no WHEN or THEN step. An upgrade bumps the CLI version and the RuleSync `ref` together.

## Risks / Trade-offs

- [The baseline misstates behavior that the docs describe wrongly] → The code cross-check before `docs/` is deleted, and the user's review of the specs.
- [Readers lose the architecture and recovery guides] → Recovery steps stay in each error's `hint`, and the architecture stays in Git history and code comments. Each future change records its internal design in `design.md`.
- [Links to `docs/` from outside the repository break] → Accepted. The README links to the specs.
- [A small fix, such as a typo, now needs a proposal and tasks] → `skip_specs: true` keeps such a change to two short files. The user asked for every change to be planned.
- [Agents with work in progress started it without a change] → The rule applies to work that starts after it lands. Uncommitted work in the checkout at that time is reported to the user and is not rewritten.
- [The npm install makes CI slower] → One extra job of about 20 seconds that runs in parallel with the others.

## Migration Plan

1. Land the rule, the `config.yaml` rules, and the CI job.
2. Check the specs against the code.
3. Point the README, `AGENTS.md`, and the plugin test away from `docs/`, then delete `docs/`.
4. Archive the change, which writes `openspec/specs/`.

To roll back, revert the commits. The deleted docs come back from Git history.
