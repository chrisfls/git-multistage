# stages Specification

## Purpose
A stage is a private staging area, chosen by its ID, so that several writers can stage and commit from one Git checkout without taking each other's changes.

## Requirements

### Requirement: Stage ID selection
A command that works on one stage SHALL take the stage ID from `--stage <id>`, given before or after the command name. If that flag is absent, it SHALL use `GIT_MULTISTAGE_ID`. If neither is set, it SHALL use the agent's ID only when `--agentic` is given. `list`, `doctor`, `sync`, and `hook` need no stage ID.

#### Scenario: Flag wins over the environment
- **WHEN** `GIT_MULTISTAGE_ID=a` is set and the user runs `git multistage --stage b status`
- **THEN** the command reports stage `b`

#### Scenario: Flag after the command name
- **WHEN** the user runs `git multistage status --stage b`
- **THEN** the command reports stage `b`

#### Scenario: No stage ID
- **WHEN** no `--stage`, no `GIT_MULTISTAGE_ID`, and no `--agentic` is given to `status`
- **THEN** the command exits with code 2

### Requirement: Stage ID format
A stage ID SHALL have 1 to 64 characters, all lowercase letters, digits, `.`, `_`, or `-`, and SHALL start with a letter or a digit. An ID that ends in `.` or `.lock`, contains `..`, or is a Windows reserved name (`con`, `prn`, `aux`, `nul`, `com0`-`com9`, `lpt0`-`lpt9`), with or without an extension, SHALL be refused with exit code 2.

#### Scenario: Uppercase ID
- **WHEN** the user passes `--stage Fix`
- **THEN** the command refuses the ID with exit code 2

#### Scenario: Reserved name
- **WHEN** the user passes `--stage nul` or `--stage nul.txt`
- **THEN** the command refuses the ID with exit code 2

#### Scenario: Name that is not a valid ref
- **WHEN** the user passes `--stage a.lock`, `--stage a.`, or `--stage a..b`
- **THEN** the command refuses the ID with exit code 2

### Requirement: Implicit stage creation
Every command that works on one stage, except `log`, SHALL create its stage the first time the stage ID is used. The new stage targets the branch that HEAD points to, or a detached HEAD itself, with the current commit as its base. Each such command except `diff` SHALL print the warning `created stage <id> on <target>`, where `<target>` is the full ref name or `HEAD`. `log` SHALL exit with code 1 for a stage that does not exist, and SHALL NOT create it.

#### Scenario: First use of an ID
- **WHEN** stage `fix-login` does not exist and the user runs `git multistage --stage fix-login add src/login.ts` on branch `main`
- **THEN** the file is staged in a new stage `fix-login` and a warning reads `created stage fix-login on refs/heads/main`

#### Scenario: Diff on a new stage
- **WHEN** stage `d` does not exist and the user runs `git multistage --stage d diff`
- **THEN** `list` shows stage `d` and no warning is printed

#### Scenario: Log of an unknown stage
- **WHEN** stage `x` does not exist and the user runs `git multistage --stage x log`
- **THEN** the command exits with code 1 and `list` shows no stage `x`

### Requirement: Explicit stage creation
`init <id>` SHALL create the stage and SHALL fail with exit code 5 when the stage's directory already exists. `init` without `<id>` SHALL use the stage ID from `--stage`, `GIT_MULTISTAGE_ID`, or `--agentic` when one is given; otherwise it SHALL create a stage with a random ID of the form `stage-` plus 8 hex digits. It SHALL print the stage ID, its branch, and its base.

#### Scenario: ID already taken
- **WHEN** stage `a` exists and the user runs `git multistage init a`
- **THEN** the command exits with code 5 and the stage is unchanged

#### Scenario: Random ID
- **WHEN** no `--stage`, no `GIT_MULTISTAGE_ID`, and no `--agentic` is given and the user runs `git multistage init`
- **THEN** the command prints the ID of a new stage, such as `Created stage stage-1f3a9c2e on main at <base>`

#### Scenario: ID from the environment
- **WHEN** `GIT_MULTISTAGE_ID=zz` is set and the user runs `git multistage init`
- **THEN** the command creates stage `zz`

### Requirement: Stages belong to a worktree
Each worktree SHALL have its own set of stages. The same stage ID in two worktrees SHALL name two separate stages. `list` SHALL list the stages of the current worktree only.

#### Scenario: Same ID in two worktrees
- **WHEN** stage `a` stages a file in the main worktree and the user runs `git multistage --stage a status` in a linked worktree
- **THEN** the linked worktree's stage `a` shows nothing staged

### Requirement: Stage deletion
`drop` SHALL delete the stage and SHALL refuse with exit code 5 while the stage has staged changes or is damaged, unless `--force` is given. When the dropped stage had staged changes, `drop --force` SHALL print the staged tree ID. An interrupted `drop` SHALL NOT leave a stage whose index file is missing.

#### Scenario: Drop with staged changes
- **WHEN** stage `a` has staged changes and the user runs `git multistage --stage a drop`
- **THEN** the command exits with code 5 and the stage keeps its changes

#### Scenario: Forced drop
- **WHEN** stage `a` has staged changes and the user runs `git multistage --stage a drop --force`
- **THEN** the stage is deleted and the staged tree ID is printed

#### Scenario: Forced drop of an empty stage
- **WHEN** stage `a` has nothing staged and the user runs `git multistage --stage a drop --force`
- **THEN** the stage is deleted and no tree ID is printed

### Requirement: Stage follows the branch tip
Before `status`, `diff`, `hunks`, `add`, `apply`, `restore`, `rm`, and `mv`, a stage whose base is behind its branch tip SHALL be refreshed. With nothing staged, its index SHALL become the tip's tree. With staged changes, they SHALL be replayed onto the tip and the tip SHALL become the base. If the replay conflicts, the stage SHALL stay as it is and the command SHALL warn that its base is behind.

#### Scenario: Another stage committed
- **WHEN** stage `b` commits a change to `x.go` and stage `a` then runs `status`
- **THEN** stage `a` treats that commit as part of its base and does not list the change to `x.go`

#### Scenario: Refresh conflicts
- **WHEN** stage `a` staged a change to lines that a newer commit on the branch also changed
- **THEN** stage `a` keeps its staged changes and `status` warns that its base is behind

### Requirement: Refresh follows moves and deletions
The refresh SHALL replay staged changes onto the tip following moves, as `commit` does. When a staged path was deleted on the tip, the stage SHALL stay as it is, and the command SHALL warn that its base is behind, naming each deleted path and its deleting commit.

#### Scenario: Refresh follows a move
- **WHEN** stage `a` staged a change to `x.go`, stage `b` committed `mv x.go y.go`, and stage `a` runs `status`
- **THEN** stage `a`'s staged change is in `y.go`

#### Scenario: Refresh meets a deletion
- **WHEN** stage `a` staged a change to `x.go`, stage `b` committed `rm x.go`, and stage `a` runs `status`
- **THEN** stage `a` keeps its staged change, and `status` warns that `x.go` was deleted by stage `b`'s commit

### Requirement: HEAD away from the stage's target
When HEAD points to another branch than the stage's target, or a detached HEAD moved to a commit that does not contain the stage's base, the stage SHALL NOT be refreshed and each refreshing command SHALL warn about it. The stage SHALL be reported as stale when its base differs from its branch tip.

#### Scenario: HEAD on another branch
- **WHEN** stage `a` targets `refs/heads/main` and the user checks out `other` and runs `git multistage --stage a status`
- **THEN** stage `a` keeps its base and the command warns that HEAD is on `refs/heads/other` while the stage targets `refs/heads/main`
