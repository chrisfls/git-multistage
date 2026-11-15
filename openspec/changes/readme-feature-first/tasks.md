# Tasks

## 1. README

- [x] 1.1 Rewrite the overview and "The problem" around the feature. Verify: neither section names agents except as one of two uses in the overview.
- [x] 1.2 Present the quick start's partial-file example as two unrelated changes. Verify: the example names no agent, and its commands and output stay as they are.
- [x] 1.3 Rename "How isolation works" to "How stages work" and rewrite it as the proposal and D2 and D4 say; move `add --edits` to "Coding agents". Verify: the section holds each fact of the old section except the plugin sentence, which "Coding agents" holds.
- [x] 1.4 Replace the agent wording of the comparison table as D3 says. Verify: the table names no agent.
- [x] 1.5 Open "Coding agents" with the plugin as an extension of the stages. Verify: every block, harness row, and sentence of the old section remains, except the opening.

## 2. Checks

- [ ] 2.1 Compare the fenced blocks and table rows of the old and new README. Verify: a script lists no block or row of the old README that the new one lacks, except the table rows that 1.4 rewords.
- [ ] 2.2 List each use of "agent" before "## Coding agents", and each contraction that the old README lacks. Verify: the only "agent" is the overview's mention of coding agents, and no new contraction appears.
- [ ] 2.3 Check every `#anchor` and relative link, and run `openspec validate --all --strict`. Verify: each link resolves, and validation exits with code 0.
