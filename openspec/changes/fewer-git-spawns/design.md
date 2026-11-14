# Design

## Context

Git's trace2 `start` events listed every `git` process of the scenario of `reuse-git-results` on master `33fbf02`: five other stages with one staged file each, then in stage `claude-w` two `status`, two hooked edits, `add --edits`, `status`, `diff --cached`, `commit`, `add`, and `commit`. The scenario starts 115 processes. The count includes the `git multistage` process that runs each command. The calls that this change removes:

| Calls | Where | Count in the scenario |
| --- | --- | --- |
| `version` | `Open` of every command, beside the setup `rev-parse` | 12 |
| `rev-parse <branch>^{commit} --symbolic-full-name HEAD --` | `refresh` of every stage command, `prepareCommit` | 9 |
| `rev-parse -q --verify <branch>^{commit}` | the PreToolUse hook's branch tip, stage creation, the commit turn | 5 |
| `symbolic-ref -q HEAD` | stage creation, `land` | 3 |
| `ls-files -z -- <path>` with `GIT_ICASE_PATHSPECS=1` | both edit hooks, when `core.ignoreCase` is true (the default of `git init` on macOS) | 4 |
| `ls-files -s -z -- <paths>` | `add --edits`, `add`, the index sync after a commit | 4 |

Under `git multistage`, Git sets `GIT_EXEC_PATH` and puts its exec path first on `PATH`, so `ownGit` starts no `--exec-path` (checked with trace2: no `--exec-path` through `git multistage status`, one through `git-multistage status`). Every plugin runs `git multistage hook`.

## Goals / Non-Goals

**Goals:**
- Fewer `git` processes in every command and in both edit hooks, with the same outputs, exit codes, commits, refs, and indexes.
- A `git` fallback for every case that the new readers do not handle, so that no repository loses support.

**Non-Goals:**
- A Go Git library. See the decision below.
- Reading Git settings in the process. Includes and conditional includes decide them.
- Repository discovery in the process. `safe.directory`, `GIT_DIR`, and `core.worktree` decide it.
- Code that writes Git objects, refs, or indexes, or that computes an object ID.
- The `--exec-path` call of a direct `git-multistage` run, which only the tests make. The design of `cheaper-git-calls` rejected caching that lookup.
- Plugin hooks that run `git-multistage` directly. That saves the dispatching `git`, but on macOS it adds the `--exec-path` call.

## Decisions

- **Remember a passed version check.** `Open` takes the name of a cache directory under the Git directory: `ms` passes `multistage`, and `""` keeps nothing on disk. The setup `rev-parse` now runs first:
  - When it fails, the version check runs, and its error comes first, as now: an old Git can fail `rev-parse` for a reason that the check explains.
  - When it succeeds, `Open` looks up the binary of internal calls in `<git-dir>/multistage/git-version`. The key is the binary's path after `exec.LookPath` and `filepath.EvalSymlinks`, its size, and its modification time in nanoseconds. A match skips `git version`. Otherwise the check runs, and a passed check writes the file, by a rename of a temporary file, without a flush. A failed write is ignored.
  - The file holds one line, `<size> <mtime-ns> <path>`. Only the last binary is kept, because one worktree runs one Git.
  - On a miss, `version` runs after `rev-parse` instead of beside it. That adds one process time to the first command of each worktree and of each new Git.
  - The key changes when Git is upgraded: a package manager writes a new file, and Homebrew also changes the resolved path. Under `git multistage`, the `git` on `PATH` is Git's own binary in the exec path, so the Xcode shim's unchanging `/usr/bin/git` is not the key; without `git multistage`, `ownGit` already resolves the shim to Git's binary.
  - Rejected: a cache in the user's cache directory. It would keep the check beside `rev-parse` on a miss, but it writes outside the repository, and the tests would share the user's file.
  - Rejected: a feature probe in the setup `rev-parse`. No `rev-parse` option marks Git 2.45, the version of the three-tree `merge-tree`.
- **Read HEAD and branches from the ref files.** With `RefFormat` `files`, `ResolveRef`, `SymbolicHead`, and `TipAndHead` read `HEAD` from the Git directory and `refs/heads/<name>` from the common directory, as Git's files backend does:
  - A file that holds `ref: <name>` names another ref, which is followed, at most five times. A file that holds an object ID of the repository's length, after trailing white space is removed, gives the ID. `HEAD` with an ID is detached.
  - A missing loose file, a path through a file, and a directory read as no loose ref; then `packed-refs` in the common directory gives the ref, or the ref does not exist. The `^` lines of peeled tags and the `#` header are skipped.
  - Git refuses to write an object other than a commit to `HEAD` or to a branch (checked with Git 2.54: `update-ref` fails with "trying to write non-commit object ... to branch"). So the ID in the file is the commit that `<ref>^{commit}` gives.
  - Every other case runs `git` as now: the `reftable` storage; a ref other than `HEAD` or `refs/heads/<name>`; a name that fails Git's ref-name rules; a symbolic link, such as the `HEAD` of `core.preferSymlinkRefs`; a file that is neither a regular file nor a directory; a read error other than a missing file; malformed content or the zero ID; a malformed line of `packed-refs`; a chain of more than five names.
  - Git writes a loose ref and `packed-refs` by a rename of a lock file, and `pack-refs` writes `packed-refs` before it deletes the loose files, so a read sees the old or the new value, as Git's own read does. `land` still moves the branch with a compare-and-swap `update-ref`.
  - `readKeep` already reads the keep ref's loose file this way; the new reader covers the refs that every command reads.
- **Read index entries from the index file.** `gitx.ReadIndex(path, match)` reads an index of version 2, 3, or 4 and returns the mode, object ID, stage, and path of each entry whose path `match` accepts, in index order:
  - Version 4 removes a prefix of the previous path, given as Git's offset varint, and adds a NUL-ended suffix. Versions 2 and 3 pad each entry with NULs to a multiple of 8 bytes, and version 3 adds two bytes of extended flags to an entry with the extended bit.
  - It returns no result, so that `ls-files` runs, for a split index (the `link` extension), a sparse index (the `sdir` extension or a directory entry), any other required extension (a signature that does not start with an upper-case letter), another version, a SHA length that does not match the repository, a read error other than a missing file, and a file that ends early. A missing index has no entries, as Git reads it.
  - The callers keep the pathspec rules of `ls-files` with `GIT_LITERAL_PATHSPECS=1`: a path matches itself and every path below it. A path that is empty, ends with `/`, or has an empty, `.`, or `..` component runs `ls-files`.
  - `lsIndex` and `readIndexEntries` call it for the stage indexes and the copies of the default index, whose writes `-c core.splitIndex=false -c index.sparse=false` keep full; a sparse or split default index still runs `ls-files`.
  - The index spelling of a path on a case-insensitive file system reads the index file too: `newEditHook` spells the edited path, and `canonicalUnheld` spells a path of `add`, `hunks`, or `mv` that the stage index does not hold as given. Both took the first name of `ls-files -z -- <pathspec>` with `GIT_ICASE_PATHSPECS=1` that equals the path by `strings.EqualFold`; the pathspec is the path in the hook and the path's directory in `canonicalUnheld`. That pathspec matches a path that equals it without ASCII case (Git compares in the C locale), the paths below such a path, and with `.` every path, so the reader applies the same two tests in index order. A pathspec with a glob character, a backslash, or a leading `:` runs `ls-files`, because with `GIT_LITERAL_PATHSPECS=0` Git can read it as a pattern or as magic.
  - The index checksum is not checked. Git does not check it on a normal read either; the file is read with one read, and Git replaces an index by a rename, so the read sees one complete file.
- **No Go Git library.** The design of `cheaper-git-calls` checked go-git v5.19.3 and v6.0.0-beta.1 against Git 2.54 repositories earlier today: it cannot read `refs/worktree/` refs from a linked worktree, does not follow config includes, fails on split and sparse indexes and on reftable, hashes CRLF files unlike `git hash-object --path`, and adds 49 modules (35 for v6). The readers of this change cover the same three reads that a library would take over in these commands, with the standard library, and they fall back to `git` where go-git fails. libgit2 needs cgo, which the release builds do without.
- **Rejected: `git fast-import` for the keep commit.** It would write the keep tree, the commit, and the ref in one process in place of `mktree`, `commit-tree`, and `update-ref`. Under `fastimport.unpackLimit` it starts `git unpack-objects` to loosen its pack (checked with Git 2.54), so it saves one process at the cost of a new write path.
- **Rejected: one transaction for the branch and the keep ref in `commit`.** The keep ref is deleted after the stage index is reset, and whether it is deleted or rewritten depends on the recorded edits that remain. Moving it into `land` changes what the crash-recovery spec keeps reachable.

## Risks / Trade-offs

- [A ref file names a missing object, so `rev-parse --verify` would have failed and the reader returns the ID] → Only a damaged repository has a branch whose commit is missing; the next `git` call that uses the tip fails, as it would for the merge base or the commit's parent.
- [A future Git writes a ref or an index form that the readers do not know] → Each reader accepts only the forms above and runs `git` for anything else; a new required index extension starts with a lower-case letter, as Git's format requires.
- [A large index] → The reader allocates only the matching entries. `ls-files` also reads the whole file.
- [The same Git binary at the same path, size, and modification time gets older] → No package manager does this; the check exists to explain an old Git, not to guard against tampering.
- [Readers and Git disagree on an edge case] → Unit tests compare each reader with `git` on the same files: `rev-parse` and `symbolic-ref` for loose, packed, symbolic, detached, unborn, and linked-worktree refs; `ls-files -s` for index versions 2, 3, and 4, SHA-1 and SHA-256, unmerged and intent-to-add entries, and paths longer than 4095 bytes.

## Migration Plan

None. Older versions ignore `git-version`.
