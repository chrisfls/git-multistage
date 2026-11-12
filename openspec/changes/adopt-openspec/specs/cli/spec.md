# Spec Delta

## Purpose

These conventions are shared by every git-multistage command, so that scripts and agents can call the tool non-interactively and act on its results.

## ADDED Requirements

### Requirement: Exit codes
Commands SHALL exit with 0 on success, 1 on an error or nothing to commit, 2 on a usage error or missing stage ID, 3 on a conflict, 4 on a hunk ID that does not match, 5 on a refusal, and 6 when a lock stays held until its timeout. A command that exits with 3 or 4 SHALL have committed or staged nothing.

#### Scenario: Unknown command
- **WHEN** the user runs `git multistage frobnicate`
- **THEN** the command exits with code 2

### Requirement: JSON output
With `--json`, `status`, `hunks`, `add`, `commit`, `list`, `doctor`, `sync`, `drop`, and `init` SHALL print one JSON object to stdout. A failure SHALL print `{"error": {...}}` with `kind`, `message`, and optional `details` and `hint`, and SHALL exit with its code.

#### Scenario: Stale hunk as JSON
- **WHEN** the user runs `add x.go --hunk <old-id> --json`
- **THEN** stdout holds an error object with `kind` `stale-hunk` and the command exits with code 4

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
`doctor` SHALL change nothing. It SHALL report the Git version, the HEAD state, operations in progress, a leftover `index.lock`, whether the default index matches the branch, and each stage's base, staged files, pending commit, and problems.

#### Scenario: Doctor on a lagging index
- **WHEN** the default index is behind the branch and the user runs `doctor`
- **THEN** it reports the lag, names the closest commit, and changes nothing
