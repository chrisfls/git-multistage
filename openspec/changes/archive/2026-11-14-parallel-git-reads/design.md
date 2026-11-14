# Design

## Context

Each `git` process costs 13 to 15 ms on macOS, one command in flight. Reads that leave every index and ref alone can run at the same time: `diff-index --cached`, `diff`, and `ls-files` on a stage's index run with `GIT_OPTIONAL_LOCKS=0`, so they do not refresh the index, and `show -s` reads only objects. `write-tree` can rewrite the index that it reads, to store the cache tree, but no other read runs on that index in `protect`.

## Goals / Non-Goals

**Goals:**
- Shorter commands, measured one command in flight, with the same outputs, exit codes, and files.
- No more `git` processes than before.

**Non-Goals:**
- Running writes at the same time.
- Changing the order of output or of the stages in `also staged in other stages`.

## Decisions

- **One helper, `parallel`.** It starts each function in a goroutine, at most 8 at once, waits for all, and returns the first error in argument order. A sequence of the calls would have returned the same error when the failures do not depend on timing. The module stays without third-party modules, so it uses `sync` instead of `errgroup`.
- **Skip the other stages for an empty stage.** `Status` reports a shared path only for a path in its own staged changes, so with none it reads no other stage.
- **Stage results keep their order.** `otherStagesStaged` stores each stage's paths by its position in the sorted directory listing and merges them in that order, so the stage lists in the output stay sorted.
- **Shared tree lookups.** The other stages usually have one base, so their concurrent `TreeOf` calls for it would each miss the cache. `TreeOf` keeps one lookup for each commit: the first caller runs `rev-parse` and later callers wait for it. A failed lookup is removed, so a later call reads again, as before.
- **What runs together.** `Status`: staged, unstaged, and untracked reads, then the other stages. `protect`: `write-tree`, the base tree, and the keep ref. PreToolUse: the snapshot and the branch tip.

## Risks / Trade-offs

- [Concurrent processes compete for CPU when many commands run at once] → The bound of 8 limits one command. The end-to-end suite and its concurrency tests run with the change.
- [A data race in the callers] → The unit tests run with `-race`, and a `-race` build of the tool ran `add`, `status` with shared paths, and the hook without a report.

## Migration Plan

None. No behavior or file changes.
