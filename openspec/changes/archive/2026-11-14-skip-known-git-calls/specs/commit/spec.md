# Spec Delta

## MODIFIED Requirements

### Requirement: Commit message
`commit` SHALL take the message from `-m` (repeatable, parts joined by a blank line) or `-F <file>` (`-F -` reads stdin); with both, `-F` SHALL win. Without either it SHALL exit with code 2 and SHALL NOT open an editor. It SHALL clean the message with `git stripspace` before the `commit-msg` hook, and again after that hook when it ran, SHALL exit with code 1 when the result is empty, and SHALL NOT apply `commit.cleanup` or `commit.template`.

#### Scenario: No message
- **WHEN** the user runs `commit` without `-m` or `-F`
- **THEN** the command exits with code 2 and no commit is created

#### Scenario: Both -m and -F
- **WHEN** the user runs `commit -m "a" -F msg.txt`
- **THEN** the commit message is the content of `msg.txt`

## ADDED Requirements

### Requirement: Absent Git hooks
`commit` SHALL treat a Git hook as absent when the hooks directory (`core.hooksPath`, or `hooks` in the common Git directory) holds no entry of its name and no `hook.*` setting exists. It SHALL skip an absent hook without starting `git hook run`, and SHALL run `git hook run` whenever it cannot tell.

#### Scenario: Hook in core.hooksPath
- **WHEN** `core.hooksPath` names a relative directory that holds a `pre-commit` hook
- **THEN** `commit` runs that hook

#### Scenario: Hook from a setting
- **WHEN** no hook file exists and `hook.lint.event` is `pre-commit` with `hook.lint.command` set
- **THEN** `commit` runs that command as its `pre-commit` hook

#### Scenario: No hooks
- **WHEN** the hooks directory holds only Git's sample hooks and no `hook.*` setting exists
- **THEN** `commit` starts no `git hook run` and lands the commit
