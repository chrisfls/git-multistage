## ADDED Requirements

### Requirement: Doctor reports orphaned stage index locks
`doctor` SHALL warn about each index lock file in the directory of a stage that no command holds, and name the stage and the file's path. It SHALL leave the file in place. It SHALL skip a stage that a command holds.

#### Scenario: Doctor finds a stage's orphaned index lock
- **WHEN** a Git child of a killed command left the lock file of stage `a`'s index, no command holds stage `a`, and the user runs `doctor`
- **THEN** it warns with the stage ID `a` and the path of the lock file, and leaves the file in place
