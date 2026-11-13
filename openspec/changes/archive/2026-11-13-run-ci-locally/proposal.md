# Proposal

## Why

GitHub Actions has billed $12.00 in a month to this repository, and every job now fails to start on the billing error. The cost came from the CI workflow: every push and pull request ran lint, OpenSpec validation, and a test job on Linux, macOS, and Windows, plus a stress run. Windows minutes count twice and macOS minutes ten times. Agents push after every commit, so one hour of work started dozens of runs. The checks that matter run on the developer's machine, and GitHub is needed for releases.

## What Changes

- The CI workflow stops running on push and pull request. It stays in the repository, triggered by hand only (`workflow_dispatch`), for a cross-platform test run on Linux and Windows when someone chooses to pay for it.
- The checks that CI ran become one local command, `mise run ci`: format, lint, vet, the test suite, and OpenSpec validation. `mise run ci:stress` repeats the concurrency and crash tests.
- `README.md` and the development skill tell contributors to run `mise run ci` before they push. The README drops the CI badge and the sentence that says CI covers three systems.
- The project context in `openspec/config.yaml` says the tool must work on all three systems, and that only macOS is tested on every change, with the manual workflow for the others.
- The release workflow (change `release-binaries`) stays the only workflow that runs on its own, on a tag, and only builds.

## Capabilities

### New Capabilities

- `local-checks`: the single local command that runs the project's checks, and what runs on GitHub and when.

### Modified Capabilities

None.

## Impact

- `.github/workflows/ci.yml`, `mise.toml`, `README.md`, `.rulesync/skills/git-multistage-development/SKILL.md`, and `openspec/config.yaml`.
- Linux and Windows get no automatic test run. A regression there is found only when someone starts the manual workflow, or after a release build. This is the cost of the change, and the proposal accepts it.
- Out of scope: self-hosted runners, and fixing the billing.
