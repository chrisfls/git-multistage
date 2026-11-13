# Design

## Context

The flagged functions are long sequences of steps: argument parsing, command dispatch, the commit transaction, edit replay, and stage opening. Many steps end in an early return, which counts toward the cyclomatic complexity.

## Goals / Non-Goals

**Goals:**
- Every flagged function falls under the thresholds, or carries a `//nolint` with a reason.

**Non-Goals:**
- Changing an algorithm, an error message, or the order of Git calls.

## Decisions

- **Extract steps, keep their order.** Each extracted function is one step of the original, called at the same point with the same inputs; early returns become returned errors or booleans. This keeps crash windows and lock scopes where they were.
- **Command dispatch through small handlers.** `(*app).run` keeps its switch but moves the option checks of each command group into helpers.
- **`//nolint` only where splitting would cut a transaction or a tight algorithm in a way that hides its invariants.** Each one names the linter and the reason; the change's report lists them.
- **One commit per file or pair of files.** Each runs the full checks of the git-multistage-development skill, so a refactor that changes behavior is caught at its own commit. The configuration that enables the linters lands last, when the code passes it.

## Risks / Trade-offs

- [An extracted step changes behavior subtly, such as a deferred cleanup that now runs earlier] → Deferred calls stay in the function that owned them, and the full end-to-end suite runs after each commit.
