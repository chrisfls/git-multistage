# Proposal

## Why

Two reviews of `README.md` found imprecise and repeated passages, a late statement of what stages isolate, and an ordering problem. "The problem" says each change gets its own index, but an index belongs to a stage, which can hold several changes. The partial-file example says the `validation` stage already exists, but its own `hunks` output prints `created stage validation`. The plugin's first subsection repeats the section's opening. The overview says twice that stages keep unrelated changes apart. A reader also scrolls past four install methods before the first command, meets the shared working tree only after the examples, and reads the install steps before learning whether the plugin is needed. The table of other harnesses makes the agent setup the largest part of the README.

## What Changes

- Overview: introduce stages through tasks: each task gets its own stage, with its own Git index, in one checkout. Say that stages share the working tree and the branch, and that a commit holds only what its stage staged, also for two tasks in one file when they stage by hunk. Add a four-line example without output: `feature-a` and `feature-b` each add and commit their own file. Name the two uses, people and coding agents, and end with `git multistage` and regular Git commits.
- "The problem": "When several tasks share a checkout, their staged changes go into the same index." Keeping them in separate commits means staging them one at a time, with `git add -p` for one file, which is interactive and hard to script. Worktrees may need separate dependencies and build outputs, and their branches need a merge. git-multistage gives each task its own stage, with its own index, in one checkout.
- Quick start: after the first example, say that stages keep staged changes apart but share the working tree, so builds and tests see every uncommitted change. Apply the wording edits: "creates the `fix-login` stage if it does not exist yet", "Other changes remain in the working tree or in their own stages.", and "set the stage ID in your shell".
- Quick start, "Stage part of a file": open with the goal. After `fix-login` is committed, `src/shared.ts` holds two unrelated edits, and you commit the input-length check and leave the other edit in the working tree. The commands and their output stay as they are.
- "How stages work": "A file deleted or moved in the shared working tree might belong to another task".
- "Coding agents": open with whether the plugin is needed. The CLI works on its own, and agents run the same commands. The optional plugin tracks the edits of each agent's file tools for `add --edits`, gives the rules of a shared checkout, and refuses the Git commands that change shared state. It tracks only edits made through an agent's file tools; the sentence on shell-command edits moves up into this opening.
- "With the plugin": drop the sentence that repeats the section's opening, and start with the opt-in, `git config multistage.shared true`.
- Install: keep the mise install, Git 2.45, and `doctor` before the quick start, with a link to a new "Other ways to install" section. Move the release, Go, and clone steps to that section, after "Compared with plain Git".
- "Other harnesses": keep the table and its notes in the README, inside a collapsed `<details>` block, so they take one line until the reader opens them.
- Specs: "The specs in openspec/specs/ document each command's behavior".

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The README keeps the install from a release that the `releases` spec requires, and the install steps of each harness that the `agent-plugins` spec requires, and no command changes, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `README.md` only. The code, the plugins, the skill, and the specs stay as they are.
