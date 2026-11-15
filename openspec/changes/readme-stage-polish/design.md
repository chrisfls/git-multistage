# Design

## Context

The review proposes six edits: a shorter overview, precise wording in "The problem", a clear setup for the partial-file example, a shorter plugin subsection, a separate document for the table of other harnesses, and the recommended install alone before the quick start. `AGENTS.md` and `openspec/config.yaml` allow no documentation file besides the README. The `agent-plugins` spec requires the README to give the install steps of each supported harness, and the `releases` spec requires it to describe the install from a release. The archived change `readme-reader-path` decided, in its D2, to keep the harness table in the README for these reasons.

## Goals / Non-Goals

**Goals:**

- Each sentence matches the model: a stage owns an index, and a stage can hold several changes.
- The partial-file example states its goal before its commands, and its prose agrees with its output.
- A reader reaches the first command after one install method.
- The agent setup takes less of the README's visible length.

**Non-Goals:**

- A change to any command, output, plugin, skill, or spec.
- A new file outside `openspec/changes/readme-stage-polish/`.

## Decisions

- **D1. The harness table stays in the README, collapsed.** The review's `docs/agent-integrations.md` breaks the rule against documentation files and the `agent-plugins` requirement, as in `readme-reader-path` D2. A `<details>` block keeps every row in the README and shows one summary line on GitHub until the reader opens it. The heading "### Other harnesses" stays outside the block, so the `#other-harnesses` link still resolves.
- **D2. The overview keeps "and the branch".** The review's version says only that stages share the working tree. A stage also commits to the branch that was checked out when it was created, so the shared branch stays in the overview.
- **D3. The other install methods get their own section, "Other ways to install".** It follows "Compared with plain Git", as the review orders the sections. The install section links to it. The plugin's "[install it](#install)" link stays, because the mise install is an install from a release.
- **D4. The README keeps its own register.** The review's sentences use contractions, such as "it's"; the new sentences use none, as in `readme-reader-path` D4.

## Risks / Trade-offs

- [A moved block loses a fact] → Task 2.1 compares every fenced block and every table row of the old and new README.
- [A heading change breaks an anchor] → Task 2.2 checks every `#anchor` and relative link against the new headings.
- [GitHub renders Markdown in `<details>` only with a blank line after `<summary>`] → Task 1.6 leaves a blank line after `</summary>` and before `</details>`.
