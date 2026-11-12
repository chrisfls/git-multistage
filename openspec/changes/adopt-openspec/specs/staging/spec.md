# Spec Delta

## Purpose

Staging puts chosen changes from the shared working tree into one stage's index, by whole file, hunk, or patch, and shows what a stage holds.

## ADDED Requirements

### Requirement: Stage whole files
`add <path>...` SHALL stage whole files as `git add` does, with ignore rules, attributes, filters, deletions, renames (both paths given), and binary files. Ignored files SHALL need `-f`. Only the stage's own index SHALL change.

#### Scenario: Stage a modified file
- **WHEN** the user runs `git multistage --stage a add src/login.ts`
- **THEN** `src/login.ts` is staged in stage `a`, and the default index and other stages are unchanged

#### Scenario: Ignored file
- **WHEN** the user runs `add` on a file that `.gitignore` ignores, without `-f`
- **THEN** the file is not staged

### Requirement: Refuse whole files that another stage staged
`add <path>` SHALL refuse with exit code 5, and stage nothing, when another stage of the worktree has staged changes in that file. `--allow-shared` SHALL override the check.

#### Scenario: File shared with another stage
- **WHEN** stage `b` has staged a hunk of `shared.ts` and stage `a` runs `add shared.ts`
- **THEN** the command exits with code 5 and stage `a` is unchanged

#### Scenario: Override
- **WHEN** stage `a` runs `add shared.ts --allow-shared`
- **THEN** the whole file is staged in stage `a`

### Requirement: List hunks with stable IDs
`hunks [--cached] [<path>...]` SHALL list zero-context hunks, each with a 12-hex-digit ID derived from the path and the changed lines. An ID SHALL stay the same while other hunks are staged or committed, and SHALL change when the hunk's content changes. `staged-by` SHALL name other stages that staged the same hunk.

#### Scenario: IDs survive other staging
- **WHEN** a file has hunks `h1` and `h2`, and `h1` is staged
- **THEN** a new `hunks` listing still shows `h2` with the same ID

#### Scenario: Hunk staged by another stage
- **WHEN** stage `b` has staged hunk `h1` and stage `a` runs `hunks`
- **THEN** `h1` is listed with `staged-by=b`

### Requirement: Identical hunks get distinct IDs
When several hunks of one file make the same change, their IDs SHALL also cover their positions, so a selection never moves to another copy.

#### Scenario: Two identical edits
- **WHEN** a file has the same one-line change at line 3 and at line 30
- **THEN** `hunks` lists two different IDs

### Requirement: Stage hunks by ID
`add <path>... --hunk <id>...` SHALL stage exactly the listed hunks, from one snapshot of the file. If any ID matches no hunk of the current diff, the command SHALL exit with code 4 and stage nothing. An untracked ignored file SHALL need `-f`.

#### Scenario: Stage one hunk
- **WHEN** `shared.ts` has hunks `d94c87b26e90` and `3c16c257727c` and the user runs `add shared.ts --hunk d94c87b26e90`
- **THEN** only the first hunk is staged

#### Scenario: Stale hunk ID
- **WHEN** the file changed after `hunks` ran and the user passes the old ID
- **THEN** the command exits with code 4 and nothing is staged

### Requirement: Files without hunks are staged whole
Files with a `filter` attribute, symbolic links, submodules, binary files, and deletions SHALL have no hunks and SHALL be staged whole.

#### Scenario: Binary file
- **WHEN** the user runs `hunks` on a changed binary file
- **THEN** the file is listed without hunks and can be staged only with `add <path>`

### Requirement: Interactive hunk selection
`add -p <path>...` SHALL let the user pick hunks interactively, as `git add -p` does. It SHALL be the only command that prompts for input.

#### Scenario: Pick a hunk
- **WHEN** the user runs `add -p shared.ts` and accepts the first hunk only
- **THEN** only that hunk is staged

### Requirement: Apply a patch to a stage
`apply [-R] <patch>|-` SHALL apply a patch to the stage's index, as `git apply --cached` does, and SHALL NOT touch the working tree.

#### Scenario: Patch from stdin
- **WHEN** the user pipes a patch into `git multistage --stage a apply -`
- **THEN** the patch's changes are staged in stage `a` and the files on disk are unchanged

### Requirement: Unstage
`restore --staged <path>...` SHALL unstage whole files. With `--hunk <id>...`, it SHALL unstage only the listed staged hunks and keep the rest.

#### Scenario: Unstage one hunk
- **WHEN** stage `a` has staged hunks `h1` and `h2` of a file and runs `restore --staged <path> --hunk h1`
- **THEN** only `h2` stays staged

### Requirement: Inspect a stage
`status` SHALL show the stage's staged, unstaged, and untracked files, its recorded edits, and the files that other stages also staged. `diff [--cached]` SHALL run `git diff` against the stage; `--cached` compares the staged changes with the base. `log` SHALL show the commits that the stage made.

#### Scenario: Another stage's file
- **WHEN** stage `fix-login` staged `src/login.ts` and stage `validation` runs `status`
- **THEN** `src/login.ts` is listed as not staged for `validation`
