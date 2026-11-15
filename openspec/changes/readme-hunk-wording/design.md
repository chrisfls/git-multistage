# Design

## Context

Two reviews follow the archived change `readme-stage-polish`. That change's D6 made "task" the reader's unit of work and introduced stages through tasks. The second review shows that this framing makes a usage convention look like part of the tool: the tool has no notion of a task, and a user can tie a stage to a task, an agent, an experiment, or a category of changes. That change's D2 keeps "the working tree and the branch" in the overview, and its D4 keeps the README free of contractions.

## Goals / Non-Goals

**Goals:**

- The README describes stages as staging areas that the user manages, and tasks and agents as ways to use them.
- The overview says that keeping two changes apart in one file takes a step from the user.
- The four passages that the first review names read naturally and keep their meaning.

**Non-Goals:**

- A new example. The review's idea of two agents editing one file is left for a later change.
- A change to any command, output, or spec.

## Decisions

- **D1. Stages are the tool's model; tasks and agents are uses.** This replaces `readme-stage-polish` D6. The overview and "The problem" name staging areas and changes, and "task" appears only where it names someone's work, such as the rules for a shared checkout. "Coding agents" keeps "give each agent its own stage ID" as a convention that the user sets up.
- **D2. The overview keeps "and the branch".** The review's suggestion names only the working tree. Stages commit to the branch that was checked out when they were created, as `readme-stage-polish` D2 says.
- **D3. The overview names the hunk step, not the commands.** "You can stage single hunks" points to the quick start's `--hunk` example without repeating it.
- **D4. The partial-file example keeps its link to `fix-login`, through "Next".** The review drops the link, but the example ends with "Both commits appear on the same branch" and a log that holds the `fix-login` commit. "Next" keeps that sequence.
- **D5. The merged plugin sentence names the agent as its subject.** The review's "letting it stage" can read as the plugin staging. "So the agent can stage" keeps who acts clear. The second sentence keeps "refuses the Git commands that change shared state", which says what the hooks do.
- **D6. The overview drops "It runs as `git multistage`".** The example right above it shows the command, as the review's version does.

## Risks / Trade-offs

- [An edit changes a fenced block or a link] → Task 2.1 compares the blocks, rows, and links of the old and new README.
