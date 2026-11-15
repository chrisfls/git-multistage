# Tasks

## 1. README

- [ ] 1.1 Rewrite the overview as the proposal and D2 say. Verify: the overview says once that stages keep changes apart, and still names the Git index, the shared working tree and branch, people and coding agents, `git multistage`, and regular Git commits.
- [ ] 1.2 Rewrite the last paragraph of "The problem" and the `git add -p` sentence as the proposal says. Verify: no sentence says that each change has its own index, and no new contraction appears.
- [ ] 1.3 Replace the setup paragraph of "Stage part of a file" with the goal. Verify: the paragraph names no pre-existing `validation` stage, and the commands and output of the example are unchanged.
- [ ] 1.4 Start "With the plugin" with the opt-in, and drop the repeated sentence. Verify: every other sentence and block of "Coding agents" remains.
- [ ] 1.5 Split the install as D3 says. Verify: "Install" holds the mise command, the Git version, `doctor`, `version`, and a link to "Other ways to install"; the new section holds every block of the old "Other ways" and follows "Compared with plain Git".
- [ ] 1.6 Wrap the harness table and its notes in a `<details>` block under "### Other harnesses", as D1 says. Verify: every harness row and note remains, with a blank line after `</summary>` and before `</details>`.

## 2. Checks

- [ ] 2.1 Compare the fenced blocks and table rows of the old and new README. Verify: a script lists no block or row of the old README that the new one lacks.
- [ ] 2.2 Check every `#anchor` and relative link, and run `openspec validate --all --strict`. Verify: each link resolves, and validation exits with code 0.
