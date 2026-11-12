# Spec Delta

## Purpose

A stage is a private staging area, chosen by its ID, so that several writers can stage and commit from one Git checkout without taking each other's changes.

## ADDED Requirements

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
A stage ID SHALL have 1 to 64 characters, all lowercase letters, digits, `.`, `_`, or `-`, and SHALL start with a letter or a digit. Names that Windows reserves, such as `con` and `nul`, SHALL be refused.

#### Scenario: Uppercase ID
- **WHEN** the user passes `--stage Fix`
- **THEN** the command refuses the ID with exit code 2

#### Scenario: Reserved name
- **WHEN** the user passes `--stage nul`
- **THEN** the command refuses the ID

### Requirement: Implicit stage creation
A command SHALL create its stage the first time the stage ID is used. The new stage targets the branch that HEAD points to, with the branch tip as its base, and the command SHALL print a warning that names the new stage and its branch.

#### Scenario: First use of an ID
- **WHEN** stage `fix-login` does not exist and the user runs `git multistage --stage fix-login add src/login.ts` on branch `main`
- **THEN** the file is staged in a new stage `fix-login` and a warning reads `created stage fix-login on refs/heads/main`

### Requirement: Explicit stage creation
`init <id>` SHALL create the stage and SHALL fail with exit code 5 when the stage already exists. `init` without an ID SHALL create a stage with a random ID and print that ID.

#### Scenario: ID already taken
- **WHEN** stage `a` exists and the user runs `git multistage init a`
- **THEN** the command exits with code 5 and the stage is unchanged

#### Scenario: Random ID
- **WHEN** the user runs `git multistage init`
- **THEN** the command prints the ID of a new stage, such as `stage-1f3a9c2e`

### Requirement: Stages belong to a worktree
Each worktree SHALL have its own set of stages. The same stage ID in two worktrees SHALL name two separate stages. `list` SHALL list the stages of the current worktree only.

#### Scenario: Same ID in two worktrees
- **WHEN** stage `a` stages a file in the main worktree and the user runs `git multistage --stage a status` in a linked worktree
- **THEN** the linked worktree's stage `a` shows nothing staged

### Requirement: Stage deletion
`drop` SHALL delete the stage and SHALL refuse while the stage has staged changes, unless `--force` is given. With `--force`, it SHALL print the staged tree ID. An interrupted `drop` SHALL NOT leave a stage whose index file is missing.

#### Scenario: Drop with staged changes
- **WHEN** stage `a` has staged changes and the user runs `git multistage --stage a drop`
- **THEN** the command refuses and the stage keeps its changes

#### Scenario: Forced drop
- **WHEN** the user runs `git multistage --stage a drop --force`
- **THEN** the stage is deleted and the staged tree ID is printed

### Requirement: Stage follows the branch tip
Before most commands, a stage whose base is behind the target branch tip SHALL be refreshed. With nothing staged, its index SHALL become the tip's tree. With staged changes, they SHALL be replayed onto the tip and the tip SHALL become the base. If the replay conflicts, the stage SHALL stay as it is and `status` SHALL report it as stale.

#### Scenario: Another stage committed
- **WHEN** stage `b` commits a change to `x.go` and stage `a` then runs `status`
- **THEN** stage `a` treats that commit as part of its base and does not list the change to `x.go`

#### Scenario: Refresh conflicts
- **WHEN** stage `a` staged a change to lines that a newer commit on the branch also changed
- **THEN** stage `a` keeps its staged changes and `status` warns that its base is behind
