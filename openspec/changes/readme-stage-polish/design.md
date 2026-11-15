# Design

## Context

Two reviews propose the edits. The first asks for a shorter overview, precise wording in "The problem", a clear setup for the partial-file example, a shorter plugin subsection, a separate document for the table of other harnesses, and the recommended install alone before the quick start. The second asks for an overview in terms of tasks with a short example, a shorter "The problem", the shared working tree in the quick start, an opening that says whether the plugin is needed, a separate `docs/agents.md` for the harness table, and sentence edits. `AGENTS.md` and `openspec/config.yaml` allow no documentation file besides the README. The `agent-plugins` spec requires the README to give the install steps of each supported harness, and the `releases` spec requires it to describe the install from a release. The archived change `readme-reader-path` decided, in its D2, to keep the harness table in the README for these reasons.

## Goals / Non-Goals

**Goals:**

- Each sentence matches the model: a stage owns an index, and a stage can hold several changes.
- The partial-file example states its goal before its commands, and its prose agrees with its output.
- A reader reaches the first command after one install method.
- A reader learns in the quick start that stages share the working tree.
- A reader learns whether the plugin is needed before its install steps.
- The agent setup takes less of the README's visible length.

**Non-Goals:**

- A change to any command, output, plugin, skill, or spec.
- A new file outside `openspec/changes/readme-stage-polish/`.

## Decisions

- **D1. The harness table stays in the README, collapsed.** Both reviews ask for a separate file, `docs/agent-integrations.md` or `docs/agents.md`. Either breaks the rule against documentation files and the `agent-plugins` requirement, as in `readme-reader-path` D2. A `<details>` block keeps every row in the README and shows one summary line on GitHub until the reader opens it. The heading "### Other harnesses" stays outside the block, so the `#other-harnesses` link still resolves.
- **D2. The overview keeps "the working tree and the branch".** The first review's version names only the working tree, and the second's says that stages "commit to the same branch". A stage commits to the branch that was checked out when it was created, so the overview says that stages share the branch, which is true in the usual case of one checkout on one branch.
- **D3. The other install methods get their own section, "Other ways to install".** It follows "Compared with plain Git", as the review orders the sections. The install section links to it. The plugin's "[install it](#install)" link stays, because the mise install is an install from a release.
- **D4. The README keeps its own register.** The reviews' sentences use contractions, such as "it's" and "doesn't"; the new sentences use none, as in `readme-reader-path` D4.
- **D5. The overview example uses two files and shows no output.** It shows the central idea in four commands. Two tasks in one file need `--hunk`, which the quick start's "Stage part of a file" keeps showing with real output. The overview says that the same-file case needs staging by hunk, so it does not claim more than `add <path>` gives.
- **D6. "Task" is the reader's unit of work, and "stage" is the tool's.** Each task gets its own stage, and each stage has its own index. No sentence says that a change or a task owns an index directly.
- **D7. Install stays early.** The second review lists "Installation" after "Coding agents", but also says to keep the binary install early. D3's split serves both reviews: one install command before the quick start, the rest later.

## Risks / Trade-offs

- [A moved block loses a fact] → Task 2.1 compares every fenced block and every table row of the old and new README.
- [A heading change breaks an anchor] → Task 2.2 checks every `#anchor` and relative link against the new headings.
- [GitHub renders Markdown in `<details>` only with a blank line after `<summary>`] → Task 1.6 leaves a blank line after `</summary>` and before `</details>`.
