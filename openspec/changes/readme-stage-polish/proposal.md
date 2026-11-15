# Proposal

## Why

A second review of `README.md` found four imprecise or repeated passages and one ordering problem. "The problem" says each change gets its own index, but an index belongs to a stage, which can hold several changes. The partial-file example says the `validation` stage already exists, but its own `hunks` output prints `created stage validation`. The plugin's first subsection repeats the section's opening. The overview says twice that stages keep unrelated changes apart. A reader also scrolls past four install methods before the first command, and the table of other harnesses makes the agent setup the largest part of the README.

## What Changes

- Overview: say once what stages achieve. Keep that each stage has its own Git index and that stages share the working tree and the branch, then name the two uses, people and coding agents, and end with `git multistage` and regular Git commits.
- "The problem": say that git-multistage gives one checkout independent staging areas, so one set of changes is staged and committed without another. Say that `git add -p` can separate changes in one file, but is interactive and hard to script.
- Quick start, "Stage part of a file": open with the goal. After `fix-login` is committed, `src/shared.ts` holds two unrelated edits, and you commit the input-length check and leave the other edit in the working tree. The commands and their output stay as they are.
- "With the plugin": drop the sentence that repeats the section's opening, and start with the opt-in, `git config multistage.shared true`.
- Install: keep the mise install, Git 2.45, and `doctor` before the quick start, with a link to a new "Other ways to install" section. Move the release, Go, and clone steps to that section, after "Compared with plain Git".
- "Other harnesses": keep the table and its notes in the README, inside a collapsed `<details>` block, so they take one line until the reader opens them.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The README keeps the install from a release that the `releases` spec requires, and the install steps of each harness that the `agent-plugins` spec requires, and no command changes, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `README.md` only. The code, the plugins, the skill, and the specs stay as they are.
