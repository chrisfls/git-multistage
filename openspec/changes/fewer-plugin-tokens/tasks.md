# Tasks

## 1. Skill

- [ ] 1.1 Rewrite `skills/git-multistage/SKILL.md` as design.md says: each instruction once, the stage ID order, and the failure steps that the hints do not give. Verify that each instruction of the current skill is in the new skill or in the CLI hint that the skill defers to, and that `go test ./test -run TestPluginFiles` passes
- [ ] 1.2 Shorten the skill description with the same triggers, and record the token counts of the description and the body, before and after, in the Results section of this file

## 2. Hook messages

- [x] 2.1 Shorten the session start notice and `otherAgentsNote` in `internal/ms/hook.go`, keeping each instruction, and add the note to the first-edit message only for an input with `agent_id`. Extend `TestHookReplay` to check that the subagent's first-edit message carries the note and the main sessions' messages leave it out, and verify that `go test ./test -run 'TestHookReplay|TestHookAnnounces|TestHookSeparates'` passes
- [x] 2.2 Record the token counts of both messages, before and after, in the Results section

## 3. Plugin version

- [ ] 3.1 Set the version of `.claude-plugin/plugin.json`, its marketplace entry, and `.codex-plugin/plugin.json` to the release version of this change, the next patch version after the newest tag. Make `TestPluginFiles` check that the three agree, and verify that it passes and that `claude plugin validate .` passes

## 4. Integration

- [ ] 4.1 In a scratch repository where another stage holds a staged change and an unstaged edit, run `claude -p` with `--plugin-dir` set to this checkout and the new binary on `PATH`, and ask it to change one file and commit. Verify that the commit holds only that change, that the other stage and the unstaged edit stay as they were, and that the reply leaves out the other changes
- [ ] 4.2 Run `mise run ci` once, and verify that it passes

## Results

Token counts with the `o200k_base` tokenizer, for a stage ID of 23 characters:

| Text | Before | After |
| --- | ---: | ---: |
| Session start notice | 94 | 73 |
| First-edit message, main session | 122 | 67 |
| First-edit message, subagent | 122 | 117 |

## Workflow follow-up

- Archive the change after every task is done.
- Release the patch version that task 3.1 names with `mise run release <version>`, after the archive is pushed.
