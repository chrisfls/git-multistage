# Tasks

## 1. README

- [ ] 1.1 Rewrite the overview as the proposal, D2, D5, and D6 say. Verify: the overview says once that stages keep changes apart, names the Git index, the shared working tree and branch, staging by hunk for one file, people and coding agents, `git multistage`, and regular Git commits, and holds the four-line two-file example.
- [ ] 1.2 Rewrite "The problem" as the proposal says. Verify: it keeps "interactive and hard to script", no sentence says that a change or a task owns an index directly, and no new contraction appears.
- [ ] 1.3 Replace the setup paragraph of "Stage part of a file" with the goal. Verify: the paragraph names no pre-existing `validation` stage, and the commands and output of the example are unchanged.
- [ ] 1.4 Rewrite the opening of "Coding agents" as the proposal says, move the shell-edit sentence into it, and start "With the plugin" with the opt-in. Verify: the opening says the CLI works without the plugin and names `add --edits`, the rules, the refused commands, and the shell-edit limit; every other sentence and block of "Coding agents" remains.
- [ ] 1.5 Split the install as D3 says. Verify: "Install" holds the mise command, the Git version, `doctor`, `version`, and a link to "Other ways to install"; the new section holds every block of the old "Other ways" and follows "Compared with plain Git".
- [ ] 1.6 Wrap the harness table and its notes in a `<details>` block under "### Other harnesses", as D1 says. Verify: every harness row and note remains, with a blank line after `</summary>` and before `</details>`.

- [ ] 1.7 Add the shared-working-tree sentence after the first quick-start example. Verify: it says that builds and tests see every uncommitted change.
- [ ] 1.8 Apply the wording edits of the proposal to the quick start, "How stages work", and "Specs". Verify: each new sentence is in the README, and no new contraction appears.

## 2. Checks

- [ ] 2.1 Compare the fenced blocks and table rows of the old and new README. Verify: a script lists no block or row of the old README that the new one lacks.
- [ ] 2.2 Check every `#anchor` and relative link, and run `openspec validate --all --strict`. Verify: each link resolves, and validation exits with code 0.
