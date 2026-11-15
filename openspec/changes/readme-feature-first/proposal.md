# Proposal

## Why

The README presents git-multistage as a tool for coding agents that also works for people: the overview says "It also works without agents", and the problem, the quick start, the stage section, and the comparison table explain everything through agents. The feature is several independent staging areas in one Git checkout, and it serves a person who keeps unrelated changes apart as much as it serves agents. The general sections should make sense to a reader who uses no agent, and the coding-agents section should present the plugin as an extension of that feature.

## What Changes

- Rewrite the overview around the feature: a checkout gets several independent staging areas, each stage commits only what it staged, and the stages share the working tree and the branch. People and coding agents appear as two equal uses.
- Rewrite "The problem" around Git's single index: everything staged goes into one commit, splitting a file needs the interactive `git add -p`, and worktrees separate the changes at the cost of a working tree, a dependency install, a build output, and a branch each.
- In the quick start, present the partial-file example as two unrelated changes in progress, with no agents.
- Rename "How isolation works" to "How stages work". Open it with what a stage holds and what stages share, then explain `add`, `add --hunk`, `rm`, and `mv`, then the limits of the shared working tree and when a worktree fits better. Move the plugin's `add --edits` out of this section into the coding-agents section.
- In the comparison table, replace the agent wording: "Keep changes separate" for the last row, and "everything staged in the checkout" for `git diff --cached`.
- Open "Coding agents" with the plugin as an extension: agents use the CLI like any other tool, and the optional plugin adds edit tracking, the rules of a shared checkout, and the safeguards.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The README keeps the install steps of each harness that the `agent-plugins` and `releases` specs require, and no command changes, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `README.md` only. The code, the plugins, the skill, and the specs stay as they are.
