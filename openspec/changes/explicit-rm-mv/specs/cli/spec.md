## MODIFIED Requirements

### Requirement: Working tree is never written
git-multistage SHALL NOT write files in the working tree, except `rm` and `mv`, which delete and move the files they name. Git hooks that it runs may.

#### Scenario: Commit leaves files alone
- **WHEN** stage `a` commits while unstaged edits are on disk
- **THEN** every file on disk is unchanged

#### Scenario: Move writes only its paths
- **WHEN** stage `a` runs `mv x.go y.go` while other files have unstaged edits
- **THEN** only `x.go` and `y.go` change on disk

### Requirement: JSON output
With `--json`, `init`, `status`, `hunks`, `add`, `apply`, `restore`, `rm`, `mv`, `commit`, `list`, `doctor`, `sync`, and `drop` SHALL print one JSON object to stdout. A failure SHALL print `{"error": {...}, "warnings": ...}` with `kind`, `message`, and optional `paths`, `details`, and `hint`, and exit with its code, except an unknown option, a missing option value, or no repository when `--json` follows the command name. `diff`, `log`, and `add -p` SHALL refuse `--json` with exit code 2.

#### Scenario: Stale hunk as JSON
- **WHEN** the user runs `add x.go --hunk <old-id> --json`
- **THEN** stdout holds an error object with `kind` `stale-hunk` and the command exits with code 4

#### Scenario: Diff refuses JSON
- **WHEN** the user runs `git multistage --stage a diff --json`
- **THEN** stdout holds an error object with `kind` `usage` and the command exits with code 2

#### Scenario: Usage error with trailing --json
- **WHEN** the user runs `git multistage --stage a commit --amend --json`
- **THEN** stderr holds `error: unknown option --amend` and the command exits with code 2
