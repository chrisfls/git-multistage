## MODIFIED Requirements

### Requirement: Stage follows the branch tip
Before `status`, `diff`, `hunks`, `add`, `apply`, `restore`, `rm`, and `mv`, a stage whose base is behind its branch tip SHALL be refreshed. With nothing staged, its index SHALL become the tip's tree. With staged changes, they SHALL be replayed onto the tip, following moves as `commit` does, and the tip SHALL become the base. If the replay conflicts, or a staged path was deleted on the tip, the stage SHALL stay as it is and the command SHALL warn that its base is behind, naming each deleted path and its deleting commit.

#### Scenario: Another stage committed
- **WHEN** stage `b` commits a change to `x.go` and stage `a` then runs `status`
- **THEN** stage `a` treats that commit as part of its base and does not list the change to `x.go`

#### Scenario: Refresh conflicts
- **WHEN** stage `a` staged a change to lines that a newer commit on the branch also changed
- **THEN** stage `a` keeps its staged changes and `status` warns that its base is behind

#### Scenario: Refresh follows a move
- **WHEN** stage `a` staged a change to `x.go`, stage `b` committed `mv x.go y.go`, and stage `a` runs `status`
- **THEN** stage `a`'s staged change is in `y.go`

#### Scenario: Refresh meets a deletion
- **WHEN** stage `a` staged a change to `x.go`, stage `b` committed `rm x.go`, and stage `a` runs `status`
- **THEN** stage `a` keeps its staged change, and `status` warns that `x.go` was deleted by stage `b`'s commit
