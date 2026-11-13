# Proposal

## Why

git-multistage is written in Go, but the repository does not say why. Python and TypeScript on Node were the other candidates. A contributor who asks whether to port the tool, or who adds a feature that one of the candidates handles better, needs the criteria and how each candidate scored against them.

## What Changes

- Record the language decision in this change's `design.md`: the criteria in order of weight, how Go, Python, and TypeScript on Node meet each one, and the measurements behind the comparison.
- Archive the change, so the decision stays in `openspec/changes/archive/` next to the other designs.
- No change to code, tests, CI, or the program's behavior.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The language is an internal design choice, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `openspec/changes/archive/<date>-record-language-choice/`: the proposal, the design with the decision, and the tasks.
