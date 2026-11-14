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

### Requirement: Git hooks
`commit` SHALL run `prepare-commit-msg` and `commit-msg` on the message, then `pre-commit` on a temporary index holding the tip with the staged change applied, then `post-commit`, all through `git hook run`. A hook is absent when the hooks directory has no file of its name and no `hook.*` setting exists; `commit` SHALL skip an absent hook without starting `git hook run`. `--no-verify` (`-n`) SHALL skip `pre-commit` and `commit-msg`. A failing hook other than `post-commit` SHALL exit with code 1. If `pre-commit` changes the tree of its index, `commit` SHALL exit with code 1 and error kind `hook-modified`, and the stage SHALL keep its changes.

#### Scenario: Hook re-stages a reformatted file
- **WHEN** a `pre-commit` hook reformats a staged file and runs `git add` on it
- **THEN** the commit exits with code 1 with error kind `hook-modified`, nothing is committed, and the stage keeps its staged change

#### Scenario: Hook rewrites only the working tree
- **WHEN** a `pre-commit` hook reformats a staged file without `git add`
- **THEN** the commit lands with the staged content, and the working-tree file keeps the hook's change

#### Scenario: Skip hooks
- **WHEN** the user runs `commit --no-verify -m "x"`
- **THEN** `pre-commit` and `commit-msg` do not run

#### Scenario: Hook in core.hooksPath
- **WHEN** `core.hooksPath` names a relative directory that holds a `pre-commit` hook
- **THEN** `commit` runs that hook

#### Scenario: Hook from a setting
- **WHEN** no hook file exists and `hook.lint.event` is `pre-commit` with `hook.lint.command` set
- **THEN** `commit` runs that command as its `pre-commit` hook

#### Scenario: No hooks
- **WHEN** the hooks directory holds only Git's sample hooks and no `hook.*` setting exists
- **THEN** `commit` starts no `git hook run` and lands the commit
