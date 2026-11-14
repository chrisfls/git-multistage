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
- [x] 5.2 Add the opt-in to the README's plugin install, and the rules for an agent without the plugin. Set the plugin manifests to the release version

## 6. Evaluation

- [x] 6.1 Run the seven scenarios with the new plugin in Claude Code and Codex, as many runs as the baseline, and record both in the Results section
- [x] 6.2 Drop each part whose scenarios do not show better or equal behavior in both agents, and record the decision
- [x] 6.3 Run `mise run ci` once, and verify that it passes. It passed on `f6cc64d`, run on an export of that commit with a Git repository around it, because `TestClineHooks` reads the Git index

## Results

Claude Code 2.1.310 with Haiku 5.5, 3 runs per cell; Codex 0.168.0 with `gpt-6-luna` at low reasoning effort, 2 runs per cell, 3 for the final wording. "Before" is v0.1.6; "after" is this change.

| Scenario | Measure | Claude before | Claude after | Codex before | Codex after |
| --- | --- | --- | --- | --- | --- |
| s1 commit | the commit holds line 2 only | 3/3 | 3/3 | 2/2 | 2/2 |
| s1 commit | the reply names other agents' files | 2/3 | 3/3 | 0/2 | 1/2 |
| s2 check the last commit | right answer, other agents' changes untouched | 3/3 | 3/3 | 2/2 | 2/3 |
| s3 discard own edit | `c.txt` restored, with the other agent's line | 3/3 | 3/3 | 1/2 | 2/2 |
| s4 branch mismatch | HEAD stays, and the reply reports it | 3/3 | 3/3 | 2/2 | 2/2 |
| s5 alone in a repository | right commit | 3/3 | 3/3 | 2/2 | 2/2 |
| s5 alone in a repository | mean input tokens | 119,506 | 90,140 | 149,624 | 155,900 |
| s6 discard through a subagent | `c.txt` restored | 3/3 | 3/3 | — | — |
| s7 stash request | other agents' changes left the working tree | 3/3 | 0/3 | 2/2 | 0/3 |
| s7 stash request | the answer is for the last commit | 3/3 | 1/3 | 2/2 | 2/3 |

A first Codex batch ran its commands through a login shell, which found the installed `git-multistage` instead of the test build, so only the skill differed: the commit held line 2 only in 0/2 runs before and 2/2 after. Both failures had edited `c.txt` with `python3` and staged the whole file.

Two ablations of s7 settled the guard and the notice's wording:

- With the first wording, which listed `git stash` but not the intent, one Codex run took the changes out with `git apply -R` and put them back. The final wording keeps other agents' changes in the working tree and names `git worktree add`; 2 of 3 Codex runs then used a worktree.
- With the notice off and the guard on, the guard denied every stash attempt: 3/3 in Claude Code and 2/2 in Codex. With the first wording, Codex still tried `git stash` in 1 of 2 runs, and the guard denied it.

Decisions, each part against its scenarios:

- Opt-in: kept. Alone in a repository, Claude Code used plain Git, loaded no skill, and read 25% fewer input tokens.
- Rules notice: kept. Taking other agents' changes out of the working tree fell from 5 of 5 runs to 0 of 6.
- Guard: kept. It stopped the stash attempts that the notice did not.
- Subagent start notice: kept. s6 is equal, and it is the subagents' only way to receive the rules now that the first-edit message leaves them out.
- Agent hints: kept. s4 is equal without the skill's failure table.
- Skill: kept. Codex staged whole files that held another agent's edit before, and staged by hunk after; the skill fell from 1,225 to 748 tokens.

Not improved: replies still name other agents' files. In s7, an agent that does not stash sometimes reports on the working tree instead of the last commit (Claude Code 1/3). The notice grew from 73 to 180 tokens, and Claude Code agents that follow "edit files with your file-editing tools" make 1 to 3 more tool calls, so s2, s3, s4, and s6 read 10–30% more input tokens.

## Workflow follow-up

- Archive the change after every task is done.
- Release the next free patch version.
