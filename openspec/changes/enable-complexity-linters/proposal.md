# Proposal

## Why

`.golangci.yml` disables gocyclo, nestif, funlen, dupl, and goconst until the functions they flag are refactored. With the thresholds of the recommended configuration, they flag 35 functions with a cyclomatic complexity above 13, 4 nested blocks, and 56 repeated strings. Long functions that mix several steps are where the recent review findings hid.

## What Changes

- Enable gocyclo (above 13), nestif (5), funlen (120 lines, 80 statements), dupl (100 tokens), and goconst (3 characters, 4 occurrences) with the thresholds of the golang-lint skill's recommended configuration.
- Refactor each flagged function into smaller functions without a change of behavior. The full test suite passes after each refactor commit.
- Where a function cannot be split safely, keep it whole with a `//nolint` that names the linter and the reason.
- goconst does not check `_test.go` files: the literals of a test are its fixtures, and a constant would hide them from the reader of the test.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change refactors code and changes the lint configuration only, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `.golangci.yml`.
- Most files of `internal/ms`, `internal/gitx`, and `internal/hunks`, and a few end-to-end tests. No exported behavior, output, or exit code changes.
