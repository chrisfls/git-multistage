# Design

## Context

The review proposes a new order, a clearer overview, a consolidated isolation section, a reorganized coding-agents section, a separate document for the harness table, and wording edits. Two project rules limit the README: `AGENTS.md` and `openspec/config.yaml` allow no documentation files besides the README, and the README holds only the overview, the install, the quick start, the plugin install, the contributing commands, and links to the specs. The `agent-plugins` spec requires the README to give the install steps of each supported harness.

## Goals / Non-Goals

**Goals:**

- A reader sees the first command after the overview, the problem, and the install.
- A reader learns early that stages share the working tree.
- A plugin user meets the opt-in before trying the plugin.

**Non-Goals:**

- New content about behavior. Behavior stays in the specs.
- A change to any command, output, plugin, or spec.

## Decisions

- **D1. Install comes before the quick start.** The quick start runs the binary, so the reader needs the install first. The review allows this order. "Other ways" stays a subsection of the install.
- **D2. The harness table stays in the README.** A separate `docs/harnesses.md` breaks the rule against documentation files, and the `agent-plugins` spec requires the README to hold each harness's install steps. The table moves nowhere; the prose drops the count, so a new harness needs no edit of a number.
- **D3. The worktree advice goes into "How isolation works".** A separate "When to use a worktree instead" section adds a section type that the README rule does not list. One paragraph in the isolation section gives the same advice.
- **D4. The README keeps its own register.** The review's suggested sentences use contractions, such as "doesn't"; the README uses none, so the new sentences use none.
- **D5. Links point into the repository.** The review's links carry `?utm_source=chatgpt.com` and absolute-looking paths; the README keeps relative links, such as `skills/git-multistage/SKILL.md`.

## Risks / Trade-offs

- [A moved paragraph loses a fact] → Task 2.1 compares every fenced block and every harness row of the old and new README.
- [A heading change breaks an anchor] → Task 2.2 checks every `#anchor` link against the new headings.
