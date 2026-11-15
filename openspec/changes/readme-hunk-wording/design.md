# Design

## Context

The review suggests four wording edits and says to stop iterating on sentences after them. The archived change `readme-stage-polish` set the README's terms in its D6: "task" is the reader's unit of work, "stage" is the tool's, and no sentence says that a task owns an index directly. Its D2 keeps "the working tree and the branch" in the overview, and its D4 keeps the README free of contractions.

## Goals / Non-Goals

**Goals:**

- The overview says that keeping two tasks apart in one file takes a step from the user.
- The four passages read naturally and keep their meaning.

**Non-Goals:**

- A new example. The review's idea of two agents editing one file is left for a later change.
- A change to any command, output, or spec.

## Decisions

- **D1. The overview names the step, not the commands.** "You can stage single hunks" points to the quick start's `--hunk` example without repeating it there.
- **D2. "The problem" closes with a stage, not an index.** The review's "giving each task its own index" breaks `readme-stage-polish` D6, and the overview already names the index. The closing sentence names a separate stage for each task.
- **D3. The partial-file example keeps its link to `fix-login`, through "Next".** The review drops the link, but the example ends with "Both commits appear on the same branch" and a log that holds the `fix-login` commit. "Next" keeps that sequence without the extra clause.
- **D4. The merged plugin sentence names the agent as its subject.** The review's "letting it stage" can read as the plugin staging. "So the agent can stage" keeps who acts clear. The second sentence keeps "refuses the Git commands that change shared state", which says what the hooks do.

## Risks / Trade-offs

- [An edit changes a fenced block or a link] → Task 2.1 compares the blocks, rows, and links of the old and new README.
