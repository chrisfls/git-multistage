# Tasks

## 1. Require an approved change

- [x] 1.1 Replace the "Plan changes with OpenSpec" section of `.rulesync/rules/overview.md` with the gate rule from design.md, and point its first paragraph to the `git-multistage` skill in place of `docs/agent-guide.md`. Run `rulesync generate`, and verify that `rulesync generate --check` passes and that `AGENTS.md` holds the rule
- [x] 1.2 Add `rules` entries to `openspec/config.yaml` for `proposal`, `design`, and `tasks` that put behavior in specs and add no documentation file, and verify that `openspec instructions tasks --change adopt-openspec --json` returns them

## 2. Validate OpenSpec in CI

- [x] 2.1 Add an `openspec` job to `.github/workflows/ci.yml` that sets up Node 22, installs `@fission-ai/openspec@2.0.0`, and runs `openspec validate --all --strict`, then verify that the same commands pass locally and that the pushed CI run passes

## 3. Check the baseline against the code

- [ ] 3.1 Check each requirement and scenario of the nine spec deltas against `internal/`, `cmd/`, and `test/`, and verify that every mismatch is fixed in the spec to match the code, or listed for the user as an open question
- [ ] 3.2 Run `openspec validate adopt-openspec --strict` after the fixes and verify that it passes

## 4. Replace the docs

- [x] 4.1 Change `TestPluginFiles` in `test/plugin_test.go` to check the skill's front matter in place of comparing it with `docs/agent-guide.md`, and verify that `go test -run TestPluginFiles ./test/` passes
- [ ] 4.2 Point the links of `README.md` to `openspec/specs/` and `skills/git-multistage/SKILL.md`, and move the Codex sandbox rule from `docs/agents.md` into the README's plugin section. Verify that `git grep -n 'docs/' -- README.md AGENTS.md .rulesync skills test` lists no link to `docs/`
- [ ] 4.3 Delete `docs/`, and verify that `go test ./...` passes and that `git grep -n 'docs/\(agent-guide\|agents\|architecture\|cli\|limitations\)'` outside `openspec/` finds nothing

## Workflow follow-up

- Archive the change with `openspec-archive-change` after the user approves the checked specs. This writes the nine specs to `openspec/specs/`.
- Verify that `openspec validate --all --strict` passes after the archive.
