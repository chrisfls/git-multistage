# local-checks Specification

## Purpose
The project's checks run on the developer's machine with one command, and GitHub runs a workflow on its own only to publish a release.

## Requirements

### Requirement: Local check command
`mise run ci` SHALL run, in order, the format check, the linters, `go vet` with and without the `testhooks` build tag, the test suite, and the OpenSpec validation with `--strict`. It SHALL exit with a non-zero code when any step fails. `mise run ci:stress` SHALL run the concurrency and crash tests repeatedly.

#### Scenario: All checks pass
- **WHEN** a contributor runs `mise run ci` in a clean checkout with the pinned tools installed
- **THEN** every step runs and the command exits with code 0

#### Scenario: A check fails
- **WHEN** a contributor runs `mise run ci` and the linters report a finding
- **THEN** the command exits with a non-zero code

### Requirement: No automatic test runs on GitHub
No workflow SHALL run on `push` or `pull_request`. The test workflow SHALL run only when started by hand. The only workflow that runs on its own SHALL be the release workflow, on a tag.

#### Scenario: Push
- **WHEN** a commit is pushed to the default branch
- **THEN** no workflow run starts

#### Scenario: Manual test run
- **WHEN** a maintainer starts the test workflow by hand
- **THEN** it runs the tests on Linux, macOS, and Windows
