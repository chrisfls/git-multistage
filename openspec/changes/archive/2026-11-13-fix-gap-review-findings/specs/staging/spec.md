## ADDED Requirements

### Requirement: Staged-by for identical copies
In an unstaged listing, `staged-by` SHALL also name another stage that has staged a hunk with the same changed lines at the same start line of the old content, so identical copies that another stage staged are marked although their IDs differ.

#### Scenario: Copies staged by another stage
- **WHEN** a file has two identical hunks, stage `b` stages both, and stage `a` runs `hunks`
- **THEN** both copies are listed with `staged-by=b`
