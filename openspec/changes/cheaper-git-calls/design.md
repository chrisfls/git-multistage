# Design

## Context

A logging wrapper for `git` on `PATH` recorded every call of one scenario on master `3da9762`: five other stages with one staged file each, then in stage `claude-w` two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, and `commit`. `status` started 9 processes, a hooked edit 6 + 10, and `commit` 24. The calls that this change removes or makes cheaper:

| Calls | Where | Why they cost more than needed |
| --- | --- | --- |
| Every internal call | Every command, on macOS | `/usr/bin/git` is the Xcode shim, which adds about 3.4 to 6 ms to each run |
| `symbolic-ref -q HEAD`, then `rev-parse -q --verify <branch>^{commit}` | `refresh` of every stage command; `prepareCommit` and the index sync of `commit` | Two processes, one after the other, for two reads that one `rev-parse` can make |
| `stripspace` | `commit` | A text transformation that needs no repository |
| `mktree` | `protect` of `status`, `diff --cached`, and the other commands of a stage with staged changes or recorded edits | It recomputes a keep tree that the keep ref already holds |

The cost of one process, one call in flight, on this macOS machine (12 cores, Git 2.54, Apple Git-157):

| Program | Time per call |
| --- | --- |
| `/usr/bin/git rev-parse --git-dir` (the shim) | 6.8 ms (median of 50); 9.4 ms (mean of 200 calls from a shell loop) |
| `$(git --exec-path)/git rev-parse --git-dir` | 3.4 ms (median of 50); 3.4 ms (mean of 200) |
| `/usr/bin/true` | 1.7 ms (mean of 200) |

## Goals / Non-Goals

**Goals:**
- Cheaper `git` processes on macOS, and fewer `git` processes in every command, with the same outputs, exit codes, commits, refs, and indexes.

**Non-Goals:**
- A Go Git library. See the decision below.
- Code that reads or writes Git objects, indexes, or refs.
- Running Git's own binary on Linux or Windows.

## Decisions

- **Run Git's own binary in place of the Xcode shim.** `Open` asks for the Git binary once for each `Repo`. On macOS, when `exec.LookPath("git")` is `/usr/bin/git` and `GIT_EXEC_PATH` is not set, it runs `/usr/bin/git --exec-path` and uses `<exec-path>/git` when that is an executable regular file. Otherwise, and in a `Repo` built without `Open`, calls run the `git` on `PATH`.
  - The binary in the exec path belongs to the Git that the shim runs: Git reports its own exec path. With `GIT_EXEC_PATH` set, the exec path is the variable's value and may hold another Git, so the shim stays.
  - The version check runs the binary that internal calls run.
  - Calls with `Opt.User` still run the `git` on `PATH`: hooks, through `git hook run`, and the commands whose output goes to the user. The shim adds `SDKROOT`, `CPATH`, `LIBRARY_PATH`, and `MANPATH` to the environment of the Git that it runs (observed with the Command Line Tools), so a hook keeps the environment that `git commit` gives it.
  - `Open` now runs `--exec-path` first, then the setup `rev-parse` beside the version check. This adds one process and about 3.4 ms to `Open`. Each later call saves about 3.4 to 6 ms.
  - On Linux, `/usr/bin/git` is Git itself. On Windows, `cmd\git.exe` is a launcher that sets up the environment for Git's shell and tools, and this change did not measure it.
  - Rejected: finding the developer directory without a process, from `DEVELOPER_DIR` or `/var/db/xcode_select_link`. That repeats the lookup rules of `xcrun`. Rejected: caching the path on disk, for the reason that the design of `skip-known-git-calls` gives against a cached version check.
- **Read the tip and HEAD with one `rev-parse`.** `git rev-parse <branch>^{commit} --symbolic-full-name HEAD` prints the tip of the branch, then the full name of the ref that HEAD points to, or `HEAD` for a detached HEAD. `--symbolic-full-name` applies only to the arguments after it, so the order matters. The call exits with code 128 when the branch does not exist or HEAD's branch is unborn; then the two reads run separately, as now. A symbolic ref that points to another symbolic ref gives the final ref, as `symbolic-ref -q HEAD` does, and a file named `HEAD` in the current directory changes nothing (checked with Git 2.54).
  - `refresh` and `prepareCommit` read the stage's branch this way. `checkTarget` splits into the read of HEAD and a check of a known HEAD, so `prepareCommit` checks the HEAD of the joint read. `land` still reads HEAD alone before `update-ref`.
  - The index sync after a commit reads the stage's branch this way. When HEAD points to another branch, it reads that branch's tip separately, as now. `syncIndexLocked` takes the tip from its caller, and `SyncIndex` and `doctorIndex` read it just before the call, as now.
  - The index sync now reads the tip before the default index's tree, not after. It holds `index.lock`, which every stage commit takes before it moves the branch, so only a tool that moves the branch without `index.lock` can move it between the two reads, as before.
  - Not joined: stage creation, which must read HEAD before it knows the branch. `HEAD^{commit}` with the name of HEAD could pair a tip and a name from two different moments if HEAD changes branch.
- **Clean the message in the process.** `cleanMessage` follows Git's `strbuf_stripspace` without a comment prefix. Each line loses its trailing spaces, tabs, CRs, and LF; Git's `isspace` excludes `\v` and `\f`. Empty lines at the start and at the end are dropped, runs of empty lines inside become one, and each kept line ends with LF. A unit test compares it with `git stripspace` on a table of inputs.
- **Record the keep tree's input in the keep commit.** The keep commit's message gains a paragraph `keep-input <digest>`, where the digest is the SHA-256 of the `mktree` input. `protect` reads the keep commit's tree, parent, and message with its one `show` call. When the parent is the stage's base and the digest equals the digest of the current input, `protect` does nothing. Otherwise it runs `mktree`, `commit-tree`, and `update-ref`, as now.
  - The digest covers text that git-multistage writes. The tree ID still comes from `mktree`, so no code computes a Git object ID.
  - One `commit-tree` call writes the tree and the digest of the same input, so the pair stays true. A keep commit of an older version has no digest, so the next `protect` writes it again once.
- **No Go Git library.** A check of go-git v5.19.3 and v6.0.0-beta.1 against Git 2.54 repositories gave these results:

  | Check | go-git v5.19.3 | go-git v6.0.0-beta.1 |
  | --- | --- | --- |
  | Modules in the build list | 49 | 35 (no stable v6 release) |
  | Binary size of a program that opens a repository | 9.0 MB (hello-world: 2.4 MB) | 7.4 MB |
  | Read `refs/worktree/multistage/<id>` from a linked worktree | `reference not found` | `reference not found` |
  | Follow `include` and `includeIf` in the config | No | Not tested |
  | Reftable repository | Does not open | Does not open |
  | Split index, sparse index | Index read fails | Index read fails |
  | SHA-256 repository | Does not open | Opens |
  | Repository with `extensions.worktreeConfig` | Does not open | Opens |
  | Blob ID of a CRLF file with `* text=auto` | Hashes the raw bytes, unlike `git hash-object --path` | Not tested |
  | Time of the reads in the process | 0.23 ms | Not measured |

  - The reads that a library could take over are the tip, HEAD, the keep ref, a commit's tree, and the settings. The keep refs live in `refs/worktree/`, so go-git cannot read them, and its settings miss included files.
  - The writes, the index reads and writes, the diffs with filters, `merge-tree`, and the hooks stay `git` processes either way.
  - After this change, the reads that a library could take over cost one `rev-parse` of about 3.4 ms in each stage command on macOS. The rest run beside other calls.

## Risks / Trade-offs

- [A clean filter or a `gpg.program` that an internal call runs gets no `SDKROOT`, `CPATH`, `LIBRARY_PATH`, or `MANPATH` from the shim] → These variables tell compilers where the SDK is, and Git's own calls do not use them. Internal calls already run with a changed environment, such as `LC_ALL=C`.
- [A later Git prints the joint `rev-parse` differently] → The output is used only when the call succeeds with exactly two non-empty lines; otherwise the separate reads run.
- [The message cleanup drifts from Git's] → `strbuf_stripspace` has kept these rules for years, and the unit test compares the two in every CI run.
- [A keep ref that a person rewrote with the same message] → Only git-multistage writes `refs/worktree/multistage/`, and the crash-recovery tests cover the keep ref.

## Migration Plan

None. A keep commit of an older version gets rewritten with the digest by the next `protect`. Older versions ignore the digest and still compare the tree.
