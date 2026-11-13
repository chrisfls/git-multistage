# Tasks

## 1. Local command

- [x] 1.1 Add the `ci` and `ci:stress` tasks to `mise.toml`
- [x] 1.2 Run `mise run ci` and `mise run ci:stress`, and fix what they report

## 2. Workflow

- [x] 2.1 Change `ci.yml` to `workflow_dispatch` only, with each matrix job running `mise run ci`
- [x] 2.2 Check with `act -l` that no workflow lists `push` or `pull_request`, apart from the release workflow's tag trigger

## 3. Docs and context

- [x] 3.1 Update the README contributing section, and remove the CI badge and the CI coverage sentence
- [x] 3.2 Update the checks section of the development skill source, and regenerate with RuleSync
- [x] 3.3 Update the project context in `openspec/config.yaml`
