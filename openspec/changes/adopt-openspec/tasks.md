# Tasks

## 1. Require an approved change

- [ ] 1.1 Replace the "Plan changes with OpenSpec" section of `.rulesync/rules/overview.md` with the gate rule from design.md, run `rulesync generate`, and verify that `rulesync generate --check` passes and that `AGENTS.md` holds the rule
- [ ] 1.2 Add a `rules.tasks` entry to `openspec/config.yaml` that requires a behavior change to update `README.md` or `docs/` in the same task group, and verify that `openspec instructions tasks --change adopt-openspec --json` returns the rule

## 2. Validate OpenSpec in CI

- [ ] 2.1 Add an `openspec` job to `.github/workflows/ci.yml` that sets up Node 22, installs `@fission-ai/openspec@2.0.0`, and runs `openspec validate --all --strict`, then verify that the same commands pass locally and that the pushed CI run passes

## 3. Check the baseline against the code

- [ ] 3.1 Check each requirement and scenario of the nine spec deltas against `internal/`, `cmd/`, and `test/`, and verify that every mismatch is fixed in the spec, with the docs corrected in the same commit, or listed for the user as an open question
- [ ] 3.2 Run `openspec validate adopt-openspec --strict` after the fixes and verify that it passes

## Workflow follow-up

- Archive the change with `openspec-archive-change` after the user approves the checked specs. This writes the nine specs to `openspec/specs/`.
- Verify that `openspec validate --all --strict` passes after the archive.
