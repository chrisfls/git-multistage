# Tasks

## 1. Skill

- [x] 1.1 Rewrite `skills/git-multistage/SKILL.md` as design.md says: each instruction once, the stage ID order, and the failure steps that the hints do not give. Verify that each instruction of the current skill is in the new skill or in the CLI hint that the skill defers to, and that `go test ./test -run TestPluginFiles` passes
- [x] 1.2 Shorten the skill description with the same triggers, and record the token counts of the description and the body, before and after, in the Results section of this file

## 2. Hook messages

- [x] 2.1 Shorten the session start notice and `otherAgentsNote` in `internal/ms/hook.go`, keeping each instruction, and add the note to the first-edit message only for an input with `agent_id`. Extend `TestHookReplay` to check that the subagent's first-edit message carries the note and the main sessions' messages leave it out, and verify that `go test ./test -run 'TestHookReplay|TestHookAnnounces|TestHookSeparates'` passes
- [x] 2.2 Record the token counts of both messages, before and after, in the Results section

## 3. Plugin version

- [x] 3.1 Set the version of `.claude-plugin/plugin.json`, its marketplace entry, and `.codex-plugin/plugin.json` to the release version of this change, the next patch version after the newest tag. Make `TestPluginFiles` check that the three agree, and verify that it passes and that `claude plugin validate .` passes

## 4. Integration

- [x] 4.1 In a scratch repository where another stage holds a staged change and an unstaged edit, run `claude -p` with `--plugin-dir` set to this checkout and the new binary on `PATH`, and ask it to change one file and commit. Verify that the commit holds only that change, that the other stage and the unstaged edit stay as they were, and that the reply leaves out the other changes
- [x] 4.2 Run `mise run ci` once, and verify that it passes. It passed on `0431700` in 97 s, run on an export of that commit, because the shared checkout held another change in progress

## Results

Token counts with the `o200k_base` tokenizer, for a stage ID of 23 characters:

| Text | Before | After |
| --- | ---: | ---: |
| Skill description, with its `name` line | 68 | 55 |
| Skill file, with its front matter | 1,684 | 1,225 |
| Session start notice | 94 | 73 |
| First-edit message, main session | 122 | 67 |
| First-edit message, subagent | 122 | 117 |

A review of the old skill against the new one found each instruction in the new skill or in the hint that it defers to. The new skill drops explanation that asks for no action, such as the `Multistage-Moved` trailer. The review also found four sentences that a model could read two ways, which the skill now words with one reading: `add --edits` under its condition, `add <path>` for the deletion rule, `rm -f` only for a file that still exists, and `--agentic` as the exception to `--stage`.

Smoke runs of task 4.1: `claude -p` 2.1.310 with Haiku 5.5, the plugin from `--plugin-dir`, user settings and MCP servers off, in a scratch repository where stage `other` holds a staged change of `b.txt` and an unstaged edit of `a.txt`. The prompt asked to change line 2 of `c.txt` and commit it. Two runs for each plugin:

| Plugin | Turns | Input tokens | Output tokens | Cost |
| --- | --- | --- | --- | --- |
| Before | 8, 8 | 122,393; 102,072 | 1,232; 1,492 | $0.0039, $0.0039 |
| After | 7, 7 | 97,827; 97,641 | 875; 913 | $0.0032, $0.0032 |

Each run loaded the skill and committed only `c.txt`. Stage `other` and the edit of `a.txt` stayed as they were. Both runs with the new plugin staged with `add --edits` in the stage that the first-edit message named. One run with the old plugin ran `init` before its first edit and staged `c.txt` by path. Two runs per plugin are too few to measure a difference in whole sessions; the counts above show the same behavior at a lower cost.

Every reply, with either plugin, named `a.txt` and `b.txt` as left out of the commit, against the note that tells the agent not to mention other agents' changes. The old plugin gives the same result, so this change keeps the behavior, and does not fix it.

## Workflow follow-up

- Archive the change after every task is done.
- Release the patch version that task 3.1 names with `mise run release <version>`, after the archive is pushed.
