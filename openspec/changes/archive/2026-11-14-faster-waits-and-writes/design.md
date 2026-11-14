# Design

## Context

A build of master `9cf4d2b` with timers around each `git` call and each atomic write ran the scenario of `cheaper-git-calls` (five other stages with one staged file each; in stage `claude-w`: two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, `commit`) on macOS, one command in flight. Each command's own time, outside its `git` processes, splits into:

| Part | Time per command | Commands |
| --- | --- | --- |
| Start until the first `git` call, mostly `/usr/bin/git --exec-path` through the Xcode shim | 11 to 13 ms | Every command |
| Atomic writes with `F_FULLFSYNC`, about 5 ms each | 4.4 to 7.0 ms (1 write) | Each hooked edit, `add` |
| | 10.3 ms (2 writes) | `add --edits` |
| | 16.6 to 17.1 ms (3 writes) | `commit` |
| Other Go code between `git` calls | 0.4 to 1.8 ms | Every command |

In a repository of 20,009 files, the Go code between `git` calls still took under 5 ms per command, while the 3 writes of `commit` took 75.6 ms and the 2 of `add --edits` 46 ms: `F_FULLFSYNC` flushes the drive's whole cache, which then holds the index that Git just wrote.

The `--exec-path` lookup and the other `git` calls belong to another change. This change takes the writes and the waits.

One atomic write of a 100-byte file and its rename, 100 writes, one in flight, macOS on this machine:

| Flush | Median | Mean |
| --- | --- | --- |
| `os.File.Sync` (`F_FULLFSYNC`) | 5.0 ms | 5.2 ms |
| `fsync(2)` | 0.30 ms | 0.31 ms |
| None | 0.15 ms | 0.16 ms |

A batch of 8 commits from 8 stages of one worktree, on different files, all 8 in flight, took 1.29 to 2.46 s on master; one commit alone takes about 145 ms. In one traced batch, the waits for `index.lock` slept 2.0 s in total, 795 ms in the last commit, and the 8 commits replayed 15 times, each replay with about 16 more `git` calls.

## Goals / Non-Goals

**Goals:**
- Cheaper atomic writes on macOS, and shorter and fewer waits when stages commit at the same time, with the same outputs, exit codes, commits, refs, and indexes.

**Non-Goals:**
- Fewer or cheaper `git` calls, including the `--exec-path` lookup of `Open`.
- The Go code between `git` calls, which takes under 5 ms per command.
- A durability guarantee for power loss beyond the one that Git gives its own index and refs.

## Decisions

- **Flush with `fsync(2)` on macOS.** `writeFileAtomic` calls a `syncFile` helper. On macOS it runs `syscall.Fsync` on the descriptor through `SyscallConn().Control`, which keeps the descriptor's mode; elsewhere it is `os.File.Sync`.
  - Git's default `core.fsyncMethod` on macOS is `writeout-only`, which is `fsync(2)`. Git's default `core.fsync` hardens objects in packs only, not the index, loose objects, or refs. Stage metadata names stage index files, trees, and refs that Git does not harden, so `F_FULLFSYNC` on the metadata made no stage survive a power loss that Git's own data would not survive.
  - The rename keeps each write atomic for a killed process, as the crash-recovery spec requires.
  - Rejected: no flush on every system, as Git does for its index. That changes Linux and Windows, which this change did not measure. Rejected: reading `core.fsync` and `core.fsyncMethod`. Git's components do not name git-multistage's files, so a mapping would be a new rule.
- **Check a held lock again after at most 10 ms.** `filelock.Acquire` and `lockIndex` start at 1 ms between checks and double up to 10 ms, in place of 5 ms doubling up to 160 ms. The timeouts stay: 60 s for a stage, 15 s and 3 s for `index.lock`.
  - A check is one `flock`, `LockFileEx`, or exclusive `open`, so 100 checks per second cost no measurable time. A long sleep holds a waiter after the lock is free: up to 160 ms before.
  - Rejected: a blocking `flock` in a goroutine with a timer. It wakes at once, but after a timeout the goroutine stays blocked in the kernel until the lock is free, and `index.lock` is a file that Git creates with an exclusive `open`, which no call can wait on.
- **Commits of one worktree take turns.** The turn is an OS lock on `<git-dir>/multistage/commit.lock`, taken with `filelock.Acquire`, so a killed commit never keeps it.
  - Each commit attempt takes the turn before it reads the tip. It gives the turn up right after `update-ref`, whether the update succeeded or failed, before it runs the `pre-commit` hook, and when the attempt ends early.
  - The turn covers the replay onto the tip, `commit-tree`, the journal write, the wait for `index.lock`, the checks of HEAD and of a Git operation in progress, and `update-ref`. The sync of the default index runs after the turn, still under `index.lock`, which the next commit takes before its `update-ref`, so the next commit's replay runs beside the sync. In the prototype, this cut the turn from about 115 ms to about 90 ms, and the batch of 8 from 0.99 s to 0.80 s.
  - The hook runs without the turn, so a slow hook never holds other stages' commits. After the hook, the attempt takes the turn again and reads the tip once more. When another commit moved the branch during the hook, the attempt ends, and the next attempt replays, finds the same introduced changes, and skips the hook. Every update of the branch thus happens under a turn, and a commit that holds its turn never loses the race to another git-multistage commit. This costs one `rev-parse` after each run of the hook.
  - A wait of 10 s for the turn, or any failure to take it, lets the attempt go on without the turn. The compare-and-swap of `update-ref` still decides which commit lands, so the turn changes only how often a commit replays. A holder that hangs, such as `commit-tree -S` waiting for a passphrase, delays other commits by 10 s at most. Tests shorten the wait through `testLockTimeout`.
  - The turn is per worktree: `index.lock` and HEAD are per worktree, and so are the stages.
  - Rejected: taking `index.lock` before the tip read. It blocks native Git commands, such as `git status` in the user's shell, for the whole attempt, and its waiters must still poll. Rejected: a turn around `land` only. It ends the polling between git-multistage commits, but every waiting commit has already built its commit on the old tip and still replays.
- **Tests.** An end-to-end test runs 8 commits at the same time and checks that each reports `attempts` 1. Others hold `commit.lock` from the test past a shortened turn wait, kill a commit at `before-update-ref` and time the next commit, run a slow `pre-commit` hook in one stage while another stage commits, and end that hook while the other stage holds its turn, paused by the test point `after-turn-tip` just after its tip read. `TestCommitRace` still moves the branch with a native `update-ref` inside the turn, so the replay path stays covered. The random delays of `GIT_MULTISTAGE_TEST_DELAY_MS` in `land` now run inside the turn.

## Risks / Trade-offs

- [A power loss on macOS right after a write can lose the write from the drive's cache] → Git's default leaves the index and refs that the metadata names in the same state.
- [A git-multistage of an older version in the same worktree takes no turn] → It commits with the compare-and-swap and replays as before; the turns help between new versions.
- [Several waiters wake within 10 ms of each other and race for the freed turn] → One wins and the others wait again; the order of commits is not first come, first served, as before.
- [A commit that waits for its turn holds its stage lock, so the editor hook of that stage waits too, and after its 10 s wait it records the edit without updating the keep ref until the next command] → The turn's wait ends after 10 s, and a wait of up to 15 s for `index.lock` already held the stage lock before this change.
- [A commit with a turn waits up to 15 s for an `index.lock` that a native Git command holds] → The waiting commits would wait for `index.lock` too; after 10 s they go on without the turn.

## Migration Plan

None. The first commit creates `commit.lock`. Older versions ignore it.
