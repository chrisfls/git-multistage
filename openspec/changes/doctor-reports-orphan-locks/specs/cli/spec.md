## MODIFIED Requirements

### Requirement: Doctor
`doctor` SHALL NOT change any stage, index, ref, or working-tree file; it only deletes the leftovers of an interrupted `drop`. It SHALL report the Git version, the HEAD state, operations in progress, a leftover `index.lock`, a leftover index lock file of a stage that no command holds, with the stage and the path, a disabled `multistage.syncIndex`, whether the default index lags the branch, each stage's problems, and files staged in several stages. `--json` SHALL add each stage's base, staged files, and pending commit.

#### Scenario: Doctor on a lagging index
- **WHEN** a stage committed with `multistage.syncIndex` set to false and the user runs `doctor`
- **THEN** it warns that the default index is behind the branch, names the lagging paths, suggests `git multistage sync`, and changes nothing

#### Scenario: Doctor cannot prove a lag
- **WHEN** the default index differs from the branch tip and neither the sync record nor the reflog proves a lag
- **THEN** it reports natively staged changes and, when the closest commit is not the tip, names that commit and suggests `git multistage sync --from <commit>`

#### Scenario: Doctor finds a stage's orphaned index lock
- **WHEN** a Git child of a killed command left the lock file of stage `a`'s index, no command holds stage `a`, and the user runs `doctor`
- **THEN** it warns with the stage ID `a` and the path of the lock file, and leaves the file in place
