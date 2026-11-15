# Proposal

## Why

Two follow-up reviews of `README.md` found that the README describes a convention as a property of the tool. "git-multistage gives each task its own staging area" suggests that the tool knows about tasks and ties a stage to each one. The tool lets the user create, choose, and manage several staging areas; tying a stage to a task, an agent, or anything else is the user's choice. The overview also overstates what a stage does: "each stage takes its own hunks" reads as if a stage picks its hunks by itself, but `add <path>` stages the whole file, and only `add --hunk` or the plugin's `add --edits` keeps two changes apart in one file. The reviews also found an awkward closing sentence in "The problem", an unnatural setup sentence in the partial-file example, and two plugin sentences that can be one.

## What Changes

- Overview: describe the capability, not a convention. "git-multistage lets you manage multiple independent staging areas in a single Git checkout, so you can stage and commit unrelated changes separately." Then: each stage has its own Git index, and all stages share the working tree and the branch; you choose the stage on each command, and you can stage single hunks when unrelated changes touch the same file. Keep the four-line example. Close with the two uses, several changes at once and several coding agents in one checkout, and with regular Git commits.
- "The problem": describe the reader's work as changes instead of tasks, and close with "git-multistage lets you use several independent staging areas in the same checkout, so you can stage and commit unrelated changes without mixing them."
- "How stages work": "might belong to another change" in place of "another task".
- Quick start, "Stage part of a file": "Next, suppose `src/shared.ts` contains two unrelated edits. You want to commit the input-length check and leave the other change in the working tree." Replace "Stage only the first hunk by its ID, and commit:" with "Stage the first hunk by its ID, then commit:".
- "Coding agents": merge the plugin's first two sentences: the plugin tracks the edits that an agent makes through its file tools, so the agent can stage only its own changes with `add --edits`, even when other agents edit the same file. The sentence on the rules and the refused commands stays.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. No command changes, and the README keeps every install step that the `agent-plugins` and `releases` specs require, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `README.md` only.
