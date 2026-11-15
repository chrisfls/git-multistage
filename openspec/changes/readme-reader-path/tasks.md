# Tasks

## 1. README

- [x] 1.1 Reorder the sections as D1 says, and rewrite the overview. Verify: the headings of `README.md` appear in the order of the proposal.
- [x] 1.2 Replace "What it keeps apart" with "How isolation works" after the quick start, with the boundary sentence first, the reworded deletion and move sentence, and the worktree paragraph of D3. Verify: the section holds each fact of the old section.
- [x] 1.3 Reorganize "Coding agents": the two setups first, the opt-in after the plugin's introduction, the shell-edit limit next to `add --edits`, and the harness table without the count. Verify: no "29" remains, and the opt-in comes before the Claude Code install commands.
- [x] 1.4 Apply the wording edits of the proposal without contractions. Verify: `grep -nE "n't|'s |'re " README.md` finds no new contraction.

## 2. Checks

- [x] 2.1 Compare the fenced blocks and the harness table rows of the old and new README. Verify: a script lists no block or row of the old README that the new one lacks, except blocks the proposal rewords.

  Result: the old and new README both hold 20 fenced blocks and 39 table rows, and every block and row of the old README is in the new one.
- [x] 2.2 Check every `#anchor` and relative link of the new README. Verify: each anchor matches a heading, and each relative path exists.

  Result: the 3 anchors (`#coding-agents`, `#install`, `#other-harnesses`) match headings, and the 9 relative paths exist.
- [x] 2.3 Run `openspec validate --all --strict`. Verify: it exits with code 0.

  Result: 12 items passed, 0 failed.
