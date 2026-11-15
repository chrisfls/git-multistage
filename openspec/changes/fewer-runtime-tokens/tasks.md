# Tasks

## 1. Baseline

- [x] 1.1 Run s1 and s2 of design.md with v0.1.13, 3 runs in Claude Code and 2 in Codex, and record the success counts and tokens in the Results section

## 2. Skill

- [x] 2.1 Rewrite `skills/git-multistage/SKILL.md` as design.md says, copy it to `plugins/gemini/skills/` and `plugins/vibe/skills/`, and record the token counts in the Results section. Verify that each instruction of the old skill is in the new one, and that `go test ./test -run 'TestPluginFiles|TestHarness'` passes

## 3. Evaluation

- [x] 3.1 Run s1 and s2 with the new skill, as many runs as the baseline, record them in the Results section, and verify that each success count is equal or better. If one is worse, restore the part that caused it and run again
- [ ] 3.2 Set the plugin manifests to the release version, run `mise run ci` once, and verify that it passes

## Results

Token counts with the `o200k_base` tokenizer:

| Text | Before | After |
| --- | ---: | ---: |
| Skill description, with its `name` line | 47 | 35 |
| Skill file, with its front matter | 746 | 664 |

A review of the old skill against the new one found each instruction in the new skill, except the two that design.md drops: the `diff --cached` step and the `--json` line. "Use that ID from then on" is covered by "Pass your stage ID on every command", and "It stages what you wrote" merged into the sentence before it.

Live runs, 2026-11-15: Claude Code 2.1.310 with Haiku 5.5, 3 runs per cell; Codex 0.168.0 with its default model, 2 runs per cell; 10 sessions in flight. Each run used a scratch shared checkout, the binary of `77b02b0`, and the plugin exported from v0.1.13 (before) or with this skill (after). Codex ran with a scratch `CODEX_HOME` and `HOME`. Input tokens count cached input; output tokens count reasoning.

| Scenario | Measure | Claude before | Claude after | Codex before | Codex after |
| --- | --- | --- | --- | --- | --- |
| s1 file tool | right commit, other agents' changes kept | 3/3 | 3/3 | 2/2 | 2/2 |
| s1 file tool | ran `diff --cached` | 3/3 | 0/3 | 2/2 | 0/2 |
| s1 file tool | mean tool calls | 5.0 | 4.0 | 8.0 | 7.5 |
| s1 file tool | mean input tokens per session | 108,214 | 75,874 | 126,558 | 119,332 |
| s2 shell edit | right commit, staged by hunk | 3/3 | 3/3 | 2/2 | 2/2 |
| s2 shell edit | ran `diff --cached` | 3/3 | 0/3 | 2/2 | 0/2 |
| s2 shell edit | mean tool calls | 6.7 | 6.0 | 7.0 | 8.5 |
| s2 shell edit | mean input tokens per session | 138,028 | 128,829 | 117,572 | 147,346 |

Every run loaded the skill. In s1, all three Claude Code runs with the new skill took 4 tool calls and about 76,000 input tokens, against 4 to 6 calls and 94,000 to 116,000 before. The other cells vary with the agent's own exploration, the same in both versions: Codex looked for `AGENTS.md` and the skill's path, and retried `sed -i` with macOS syntax. Two or three runs are too few to measure those cells' totals. The success counts are equal in every cell, so the change ships.

## Workflow follow-up

- Archive the change after every task is done.
- Release the patch version that task 3.2 names with `mise run release <version>`, after the archive is pushed.
