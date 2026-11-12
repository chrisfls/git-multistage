---
name: git-multistage-development
description: Build, format, lint, and test git-multistage, and choose the Go skills for a change. Use before you change or review Go code in this repository, and before you report a Go change as done.
---

# Developing git-multistage

git-multistage is a Go CLI with no dependencies outside the standard library. The module is `github.com/chrisfls/git-multistage`, and its packages are under `internal/`. The end-to-end tests in `test/` run the real `git`.

## Checks

Run these from the repository root before you report a change as done. CI runs the same checks.

```bash
golangci-lint fmt ./...                 # gofumpt and goimports
golangci-lint run ./...                 # linters from .golangci.yml, build tag testhooks
go vet ./... && go vet -tags testhooks ./...
go test -count=1 ./...
```

- `.golangci.yml` is the source of truth for the linters. Each disabled linter has its reason there.
- A `//nolint` names the linter and gives a reason, such as `//nolint:nilerr // a missing stage directory means no other stage`.
- The `testhooks` build tag compiles the fault-injection hooks in `internal/ms/faults_testhooks.go`.

## Go skills

The `golang-*` skills come from `samber/cc-skills-golang`. Load the skills that match the change:

| Change | Skills |
| --- | --- |
| Locks, goroutines, processes, or crash windows | `golang-concurrency`, `golang-safety` |
| Errors and exit codes | `golang-error-handling`, `golang-cli` |
| Hook payloads, paths, and subprocess arguments | `golang-security`, `golang-safety` |
| Tests | `golang-testing` |
| Lint findings | `golang-lint`, `golang-code-style` |
| A review | `golang-code-style`, `golang-naming`, `golang-error-handling`, `golang-safety` |

## Agent configuration

RuleSync generates the agent configuration from `rulesync.jsonc` and `.rulesync/`:

- `.rulesync/rules/overview.md` generates `AGENTS.md`. Edit the rule, then regenerate. Do not edit `AGENTS.md` directly.
- `.rulesync/skills/` holds the skills of this repository.
- `rulesync.lock` pins the commit of each skill source.

```bash
rulesync install && rulesync generate   # fetch the pinned skills and write the agent files
rulesync install --update               # move the skill sources to their latest commits
```

The generated skill folders and the fetched skills are ignored by Git, so run `rulesync install && rulesync generate` after you clone the repository.
