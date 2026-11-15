# Tasks

## 1. README

- [x] 1.1 Rewrite the overview as the proposal and D1, D2, D3, and D6 say. Verify: the overview opens with "lets you manage multiple independent staging areas", says that the user chooses the stage and can stage single hunks, names the working tree and the branch, and holds the four-line example; it does not use "task".
- [x] 1.2 Rewrite "The problem" in terms of changes, with the closing sentence of the proposal. Verify: the section does not use "task", keeps "interactive and hard to script", and no sentence says that a task or a change owns an index.
- [x] 1.3 Replace "another task" in "How stages work" with "another change". Verify: the only "task" left in the README is in the rules block for a shared checkout and the comparison table's "Task" column.
- [x] 1.4 Rewrite the setup and the staging sentence of "Stage part of a file" as the proposal and D4 say. Verify: the setup starts with "Next, suppose", and the commands and output are unchanged.
- [x] 1.5 Merge the plugin's first two sentences as D5 says. Verify: the agent is the subject of the staging clause, and the sentences on the rules, the refused commands, and shell-command edits remain.

## 2. Checks

- [x] 2.1 Compare the fenced blocks, table rows, and links of the old and new README, and run `openspec validate --all --strict`. Verify: no block or row is missing, each link resolves, the README holds no contraction, and validation exits with code 0.

  Result: both versions hold 21 fenced blocks and 39 table rows, none missing; all 18 links resolve; the README holds no contraction; and `openspec validate --all --strict` passes 12 items with 0 failures.
