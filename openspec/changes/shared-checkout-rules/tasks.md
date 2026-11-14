# Tasks

## 1. Opt-in

- [x] 1.1 Make every hook event return before it records, creates a stage, or prints, unless the checkout's config sets `multistage.shared` to `true`. Set it in the test helpers that run hooks, and add tests for a checkout without it. Verify with `go test ./test -run 'TestHook'`

## 2. Notices

- [x] 2.1 Rewrite `otherAgentsNote` as the rules notice, print it at `SessionStart` and `SubagentStart`, and drop it from the first-edit message. Add `SubagentStart` to `hooks/hooks.json`. Verify with tests of both events and of the first-edit message, and `go test ./test -run 'TestHook|TestPlugin'`

## 3. Guard

- [x] 3.1 Add the shell command check in `internal/ms/guard.go` and `Bash` to the `PreToolUse` matcher. Verify with unit tests of the parser and an end-to-end hook test of a denied and an allowed command

## 4. Hints for agents

- [x] 4.1 Add the agent hints for `branch-mismatch`, `operation-in-progress`, `index-locked`, `busy`, `shared-path`, and `hook-modified`, and the Codex hint for a read-only `.git`. Verify with tests that run the failing commands with and without `CLAUDE_CODE_SESSION_ID`

## 5. Skill and README

- [x] 5.1 Rewrite the skill: the description says when to use it, the body holds the procedure, staging by path starts with `hunks`, and failures go to the hint. Make `TestPluginFiles` check that the description starts with `Use `. Count the tokens
- [ ] 5.2 Add the opt-in to the README's plugin install, and the rules for an agent without the plugin. Set the plugin manifests to the release version

## 6. Evaluation

- [ ] 6.1 Run the seven scenarios with the new plugin in Claude Code and Codex, as many runs as the baseline, and record both in the Results section
- [ ] 6.2 Drop each part whose scenarios do not show better or equal behavior in both agents, and record the decision
- [ ] 6.3 Run `mise run ci` once, and verify that it passes

## Results

## Workflow follow-up

- Archive the change after every task is done.
- Release the next free patch version.
