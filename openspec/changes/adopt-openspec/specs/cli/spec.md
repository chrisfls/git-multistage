# Spec Delta

## Purpose

These conventions are shared by every git-multistage command, so that scripts and agents can call the tool non-interactively and act on its results.

## ADDED Requirements

### Requirement: Exit codes
Commands SHALL exit with 0 on success, 1 on an error or nothing to commit, 2 on a usage error or a missing or ambiguous stage ID, 3 on a conflict, 4 on a hunk ID that does not match, 5 on a refusal, and 6 when a lock stays held until its timeout or a commit gives up because the branch keeps moving. A command that exits with 3 or 4 SHALL have committed or staged nothing.

#### Scenario: Unknown command
- **WHEN** the user runs `git multistage frobnicate`
- **THEN** the command exits with code 2

### Requirement: JSON output
With `--json`, `init`, `status`, `hunks`, `add`, `apply`, `restore`, `commit`, `list`, `doctor`, `sync`, and `drop` SHALL print one JSON object to stdout. A failure SHALL print `{"error": {...}, "warnings": ...}` with `kind`, `message`, and optional `paths`, `details`, and `hint`, and exit with its code, except an unknown option, a missing option value, or no repository when `--json` follows the command name. `diff`, `log`, and `add -p` SHALL refuse `--json` with exit code 2.

#### Scenario: Stale hunk as JSON
- **WHEN** the user runs `add x.go --hunk <old-id> --json`
- **THEN** stdout holds an error object with `kind` `stale-hunk` and the command exits with code 4

#### Scenario: Diff refuses JSON
- **WHEN** the user runs `git multistage --stage a diff --json`
- **THEN** stdout holds an error object with `kind` `usage` and the command exits with code 2

#### Scenario: Usage error with trailing --json
- **WHEN** the user runs `git multistage --stage a commit --amend --json`
- **THEN** stderr holds `error: unknown option --amend` and the command exits with code 2

### Requirement: Non-interactive
No command except `add -p` SHALL prompt for input.

#### Scenario: Commit without a message
- **WHEN** the user runs `commit` without `-m` or `-F`
- **THEN** the command fails at once and opens no editor

### Requirement: Supported repositories
The tool SHALL require Git 2.45 or newer and SHALL fail at startup with an older Git. It SHALL refuse a bare repository. It SHALL support sparse checkout, a sparse index, and a split index.

#### Scenario: Bare repository
- **WHEN** the user runs `git multistage --stage a status` in a bare repository
- **THEN** the command fails

### Requirement: Platforms
The tool SHALL work on Linux, macOS, and Windows.

#### Scenario: Windows
- **WHEN** the test suite runs on Windows
- **THEN** it passes as on Linux and macOS

### Requirement: Working tree is never written
git-multistage SHALL NOT write files in the working tree. Git hooks that it runs may.

#### Scenario: Commit leaves files alone
- **WHEN** stage `a` commits while unstaged edits are on disk
- **THEN** every file on disk is unchanged

### Requirement: Doctor
`doctor` SHALL NOT change any stage, index, ref, or working-tree file; it only deletes the leftovers of an interrupted `drop`. It SHALL report the Git version, the HEAD state, operations in progress, a leftover `index.lock`, a disabled `multistage.syncIndex`, whether the default index lags the branch, each stage's problems, and files staged in several stages. `--json` SHALL add each stage's base, staged files, and pending commit.

#### Scenario: Doctor on a lagging index
- **WHEN** a stage committed with `multistage.syncIndex` set to false and the user runs `doctor`
- **THEN** it warns that the default index is behind the branch, names the lagging paths, suggests `git multistage sync`, and changes nothing

#### Scenario: Doctor cannot prove a lag
- **WHEN** the default index differs from the branch tip and neither the sync record nor the reflog proves a lag
- **THEN** it reports natively staged changes and, when the closest commit is not the tip, names that commit and suggests `git multistage sync --from <commit>`
