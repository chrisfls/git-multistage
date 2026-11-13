## ADDED Requirements

### Requirement: Deletion next to a later change
`add --edits` SHALL treat a recorded deletion as lost only when a later change on disk, at the point of the deletion, adds back at least one of the deleted lines. A later change of a neighbouring line that adds none of them SHALL leave the deletion in effect.

#### Scenario: Neighbour line changed after a deletion
- **WHEN** agent `s` deletes lines `c` and `d` of `a b c d e` with a hooked edit, agent `t` then changes `e` to `E`, and `s` runs `add --edits`
- **THEN** the command stages the deletion, and the commit holds `a b e`

#### Scenario: Deleted line put back with a neighbour change
- **WHEN** agent `s` deletes a line with a hooked edit, and a later change puts that line back and also changes the line next to it
- **THEN** `add --edits` fails with `edit-lost` and stages nothing
