# Design

## Context

`ancestors()` in `internal/ms/procs.go` calls `processTable()`. On Unix that runs `ps -A -o pid=,ppid=,comm=` with `context.Background()`. On Windows it reads a Toolhelp snapshot in process, with no child process. `agentStageID` treats an error from `ancestors()` as an unreadable table.

## Goals / Non-Goals

**Goals:**
- No `--agentic` command waits more than about 3 seconds on `ps`.

**Non-Goals:**
- A deadline for the Windows Toolhelp snapshot. It is a kernel call that does not wait on another process, and a goroutine that outlives its deadline would only move the hang elsewhere.

## Decisions

- **`exec.CommandContext` with `context.WithTimeout`, and `cmd.WaitDelay`.** The deadline kills `ps`. `WaitDelay` closes the output pipe shortly after, so a child of a `ps` wrapper that keeps the pipe open cannot hold `Output()`.
- **3 seconds, as a package variable.** `ps -A` takes milliseconds on a normal system; 3 seconds leaves room for a loaded one. The test replaces `ps` on `PATH` with a script that sleeps, so it needs no hook.
- **Return the context error.** `agentStageID` already handles any error as an unreadable table, so no new branch is needed.

## Risks / Trade-offs

- [A very slow but working `ps` now counts as unreadable] → The priority order decides, as on any system without `ps`.
