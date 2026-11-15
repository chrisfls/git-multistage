# Tasks

## 1. README

- [ ] 1.1 Rewrite the hunk sentence of the overview as the proposal and D1 say. Verify: the overview says that the user can stage single hunks, and no sentence says that a stage picks its hunks by itself.
- [ ] 1.2 Replace the closing sentence of "The problem" as D2 says. Verify: the sentence names a stage for each task, and no sentence says that a task owns an index.
- [ ] 1.3 Rewrite the setup and the staging sentence of "Stage part of a file" as the proposal and D3 say. Verify: the setup starts with "Next, suppose", and the commands and output are unchanged.
- [ ] 1.4 Merge the plugin's first two sentences as D4 says. Verify: the agent is the subject of the staging clause, and the sentences on the rules, the refused commands, and shell-command edits remain.

## 2. Checks

- [ ] 2.1 Compare the fenced blocks, table rows, and links of the old and new README, and run `openspec validate --all --strict`. Verify: no block or row is missing, each link resolves, the README holds no contraction, and validation exits with code 0.
