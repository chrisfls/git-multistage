# Proposal

## Why

A third review of `README.md` found that the overview overstates what a stage does: "When two tasks change the same file, each stage takes its own hunks" reads as if each stage picks its hunks by itself, but `add <path>` stages the whole file, and only `add --hunk` or the plugin's `add --edits` keeps two tasks apart in one file. It also found an awkward closing sentence in "The problem", an unnatural setup sentence in the partial-file example, and two sentences in the plugin's introduction that can be one.

## What Changes

- Overview: say that each commit contains only the changes staged for its task, and that when tasks change the same file, you can stage single hunks to keep their changes apart.
- "The problem": replace the closing sentence with "git-multistage solves this with a separate stage for each task in the same checkout, so you can stage and commit the tasks independently."
- Quick start, "Stage part of a file": "Next, suppose `src/shared.ts` contains two unrelated edits. You want to commit the input-length check and leave the other change in the working tree." Replace "Stage only the first hunk by its ID, and commit:" with "Stage the first hunk by its ID, then commit:".
- "Coding agents": merge the plugin's first two sentences: the plugin tracks the edits that an agent makes through its file tools, so the agent can stage only its own changes with `add --edits`, even when other agents edit the same file. The sentence on the rules and the refused commands stays.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. No command changes, and the README keeps every install step that the `agent-plugins` and `releases` specs require, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `README.md` only.
