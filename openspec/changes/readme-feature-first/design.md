# Design

## Context

The review asks that every section before "Coding agents" describe git-multistage in terms of changes, tasks, and stages, and that "Coding agents" keep all its content. The `readme-reader-path` change set the section order, which stays. The stages spec says that a new stage targets the branch that HEAD points to when the stage is created.

## Goals / Non-Goals

**Goals:**

- A reader who uses no coding agent understands and can use every section before "Coding agents".
- "Coding agents" presents the plugin as an extension of the stages: it knows which lines an agent changed through its file tools.

**Non-Goals:**

- A change to any command, output, plugin, spec, or plugin manifest description.
- Less content in "Coding agents".

## Decisions

- **D1. The README keeps its register.** The review's suggested sentences use contractions, such as "you're" and "doesn't"; the README uses none, so the new sentences use none.
- **D2. "How stages work" states the branch exactly.** A stage commits to the branch that was checked out when the stage was created, as the stages spec says. The overview can say that stages share the branch, because that is the usual case; the stage section gives the exact rule.
- **D3. The plain-Git cell of the last table row names a way to keep changes apart.** The review suggests "One shared index per checkout", which names the limit and not a way to do the task, as the other cells of that column do. The cell says "Commit one change at a time, or use one worktree per change", and the git-multistage cell says "One stage per change, in one checkout".
- **D4. The worktree advice stays in "How stages work".** It now speaks of tasks: a build or a test run sees the uncommitted changes of every stage, and a worktree gives a separate working tree.

## Risks / Trade-offs

- [A sentence loses a fact while it loses the agent wording] → Task 2.1 compares the fenced blocks and table rows, and task 2.2 lists every remaining "agent" before "Coding agents".
