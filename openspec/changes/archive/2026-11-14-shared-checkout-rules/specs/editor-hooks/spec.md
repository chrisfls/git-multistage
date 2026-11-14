# editor-hooks Specification

## MODIFIED Requirements

### Requirement: Session start notice
On a `SessionStart` event in an opted-in checkout, the hook SHALL print `additionalContext` that says other agents may edit and commit in the checkout, that their changes are theirs to leave alone, that the agent edits files with its file-editing tools, which Git commands change shared state and must not be run, and that the agent stages and commits with `git multistage`. Elsewhere it SHALL print nothing. The first-edit message SHALL leave the notice out. The hook SHALL exit with code 0.

#### Scenario: Session starts in a worktree
- **WHEN** the hook receives `SessionStart` with `cwd` in an opted-in checkout
- **THEN** it prints additional context about the other agents that names the `git-multistage` skill, and exits with code 0

#### Scenario: Session starts outside a worktree
- **WHEN** the hook receives `SessionStart` with `cwd` outside any Git worktree
- **THEN** it prints nothing and exits with code 0

#### Scenario: Session starts in a checkout that did not opt in
- **WHEN** the hook receives `SessionStart` with `cwd` in a Git worktree whose config does not set `multistage.shared`
- **THEN** it prints nothing and exits with code 0

#### Scenario: First edit of a subagent
- **WHEN** a Claude Code subagent with `agent_id` `x1` makes the first recorded edit of stage `claude-x1`
- **THEN** the first-edit message names the stage and the commands, and leaves out the note about other agents, which the subagent start notice gave

#### Scenario: First edit of a main session
- **WHEN** a main session makes the first recorded edit of its stage
- **THEN** the first-edit message names the stage and the commands, and leaves out the note about other agents

## ADDED Requirements

### Requirement: Shared checkout opt-in
The hooks SHALL act only in a checkout whose Git config sets `multistage.shared` to `true`. In any other checkout they SHALL record no edit, create no stage, print nothing, and allow every command.

#### Scenario: Edit in a checkout that did not opt in
- **WHEN** an agent edits a file with `Edit` in a Git worktree whose config does not set `multistage.shared`
- **THEN** the hook records nothing, creates no stage, and prints nothing

#### Scenario: Edit in an opted-in checkout
- **WHEN** an agent edits a file with `Edit` in a checkout whose config sets `multistage.shared` to `true`
- **THEN** the hook records the edit in the agent's stage

### Requirement: Subagent start notice
On a `SubagentStart` event in an opted-in checkout, the hook SHALL print the session start notice as `additionalContext` for the subagent, and exit with code 0.

#### Scenario: Subagent starts
- **WHEN** the hook receives `SubagentStart` with `cwd` in an opted-in checkout
- **THEN** it prints the notice with `hookEventName` `SubagentStart`

### Requirement: Shared-state Git commands
On a `PreToolUse` event of a shell tool in an opted-in checkout, the hook SHALL deny a command that runs `git add`, `git commit`, `git rm`, `git mv`, `git stash` other than `list` and `show`, `git restore`, `git reset`, `git checkout`, or `git switch`, with a reason that says what to do instead. It SHALL allow every other command, and every command that runs in another checkout.

#### Scenario: Stash
- **WHEN** an agent in an opted-in checkout runs `git stash` through `Bash`
- **THEN** the hook denies the command, and its reason says why and what to do instead

#### Scenario: Read-only Git commands
- **WHEN** an agent in an opted-in checkout runs `git status && git stash list && git multistage --stage a add --edits`
- **THEN** the hook allows the command

#### Scenario: Another checkout
- **WHEN** an agent in an opted-in checkout runs `git -C /tmp/scratch commit -m x`, and `/tmp/scratch` is a repository that did not opt in
- **THEN** the hook allows the command
