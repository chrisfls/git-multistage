# Proposal

## Why

A review of `README.md` found that a new reader reads four sections before the first command, and that the README explains how git-multistage works better than why to use it. The first two sentences of the overview say the same thing, and "at the same time" can suggest separate working trees. The limit of the isolation, a shared working tree with separate stages, is spread over three sections. The plugin's opt-in, `git config multistage.shared true`, comes after the install commands, so a user can install the plugin and see it do nothing. Several sentences repeat a word or explain the sentence before them.

## What Changes

- Reorder the sections: overview, the problem, install, quick start, how isolation works, compared with plain Git, coding agents, specs, contributing, license.
- Rewrite the overview: each agent has its own staging area in one shared checkout, and every commit is a regular Git commit.
- Merge "What it keeps apart" into a new "How isolation works" section after the quick start. It leads with the boundary: stages share the working tree and keep apart only what is staged and committed. It says why `add` leaves deletions and moves to `rm` and `mv`, and when to use a separate worktree instead.
- In "Coding agents", separate the two setups, the agent guide alone and the plugin. Move the `multistage.shared` opt-in to just after the plugin's introduction, and the limit on shell edits next to `add --edits`.
- Keep the table of other harnesses in the README, and drop the count "29" from the prose.
- Apply the review's wording edits, such as "Stage and commit a file:" and "Both commits appear on the same branch:".
- Keep the content of the comparison table, the partial-file example, the Codex sandbox warning, and the note on which harnesses a real session has tested.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The README keeps the install steps of each harness that the `agent-plugins` and `releases` specs require, and no command changes, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `README.md` only. The code, the plugins, the skill, and the specs stay as they are.
