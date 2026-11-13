## MODIFIED Requirements

### Requirement: Identical hunks get distinct IDs
When several hunks of one file make the same change, the ID of each copy SHALL also hash the stage's base commit, a blob, and the copy's start line in that blob, so a selection never moves to another copy. An unstaged listing SHALL use the working-tree snapshot and the copy's line in it. A staged listing SHALL use the file's blob in the base and the copy's line in it. Staging or restoring one copy SHALL keep the IDs of the others; a new base SHALL make them unknown.

#### Scenario: Two identical edits
- **WHEN** a file has the same one-line change at line 3 and at line 30
- **THEN** `hunks` lists two different IDs

#### Scenario: Restore one staged copy
- **WHEN** a stage has staged three identical hunks of one file, the user lists them with `hunks --cached`, and restores the first with `restore --staged --hunk`
- **THEN** `restore --staged --hunk` with the listed ID of the second copy restores that copy

#### Scenario: Staged copy after a new base
- **WHEN** the user lists staged identical hunks with `hunks --cached`, and the stage then moves to a new base that changes the file
- **THEN** `restore --staged --hunk` with a listed copy ID exits with code 4 and changes nothing
