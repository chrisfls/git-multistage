# Tasks

## 1. Baseline

- [ ] 1.1 Run s1 and s2 of design.md with v0.1.13, 3 runs in Claude Code and 2 in Codex, and record the success counts and tokens in the Results section

## 2. Skill

- [ ] 2.1 Rewrite `skills/git-multistage/SKILL.md` as design.md says, copy it to `plugins/gemini/skills/` and `plugins/vibe/skills/`, and record the token counts in the Results section. Verify that each instruction of the old skill is in the new one, and that `go test ./test -run 'TestPluginFiles|TestHarness'` passes

## 3. Evaluation

- [ ] 3.1 Run s1 and s2 with the new skill, as many runs as the baseline, record them in the Results section, and verify that each success count is equal or better. If one is worse, restore the part that caused it and run again
- [ ] 3.2 Set the plugin manifests to the release version, run `mise run ci` once, and verify that it passes

## Results

## Workflow follow-up

- Archive the change after every task is done.
- Release the patch version that task 3.2 names with `mise run release <version>`, after the archive is pushed.
