# Design

## Context

Each `git` process costs 13 to 15 ms on macOS (measured as the time of a command divided by its `git` processes, one command in flight). A counting wrapper on `PATH` logged every call of one scenario: two `status`, two edits through the Claude Code hook, `add --edits`, `diff --cached`, `add`, and two `commit`. Over those commands, the most frequent calls were:

| Call | Count | Why it repeats |
| --- | --- | --- |
| `rev-parse <commit>^{tree}` | 21 | `TreeOf` runs for the base, the tip, and in `protect`, for the same commit |
| `write-tree` | 14 | Different indexes, and the stage index at the end of each command |
| `symbolic-ref -q HEAD` | 11 | The branch can move during a command |
| `rev-parse -q --verify <branch>^{commit}` | 11 | The tip can move during a command |
| `version` | 8 | Once per process, beside `rev-parse` |
| `rev-parse --git-path index` | 8 | Once per process, after the setup `rev-parse` |
| `config --type=bool --get` | 8 | One call for each setting read |

## Goals / Non-Goals

**Goals:**
- Fewer `git` processes in every command and in both edit hooks, with the same outputs, exit codes, and files.

**Non-Goals:**
- A Go Git library.
- Caching anything that another process can change during a command, such as a ref, `HEAD`, or an index.
- A long-running `git` process, such as `cat-file --batch`, that serves reads for a whole command.

## Decisions

- **No Go Git library.** A library such as go-git would answer the reads in the process: refs, commits, trees, settings, and index entries. It is rejected for these reasons:
  - The `record-language-choice` change weighs few third-party modules as criterion 5, because the hook runs inside every agent's edit loop. go-git brings about twenty modules. libgit2 needs cgo, which breaks the static builds for Windows, macOS, and Linux that the `releases` spec requires.
  - The `cli` spec requires a sparse index and a split index. From the go-git v5 documentation and issues, not tested in this change: go-git v5 does not read the `link` and `sdir` index extensions, does not read the reftable ref format, and does not run clean filters or line-ending conversion, which `add` and the snapshots rely on. Each gap would need a fallback to the `git` process.
  - The writes stay `git` processes either way: `update-index`, `write-tree`, `merge-tree`, `commit-tree`, `update-ref`, and the commit hooks. These are about half of the calls.
  - The reads that the library would replace are the same calls that the decisions below remove or keep for correctness.
- **Fold the index path into setup.** `rev-parse` takes `--git-path index` beside the options that `Open` already passes, so `OpenRepo` starts no second `rev-parse`. `--path-format=absolute` applies to it, as it does now.
- **Memoize the tree of a commit.** A commit's tree never changes, so `TreeOf` keeps a map from commit ID to tree ID in the `Repo`, behind a mutex. Refs and `HEAD` are not cached, because another agent can move the branch during a command.
- **Read settings once.** The first `configBool` runs `git config -z --list`, which applies include files as `git config --get` does, and keeps the last value of each key. Key names are case-insensitive in their section and variable parts, so the map keys are lower case. The value parse follows Git's boolean rules: a key without `=` is true; `true`, `yes`, `on`, and a nonzero integer are true; `false`, `no`, `off`, `0`, and the empty string are false. Any other value gives the default, as a failed `--get` does now. A `git config` error gives the default for every key, as now.
- **One `update-index` for a snapshot.** `cleanFile` passes `--cacheinfo <mode>,<oid>,<path>` and the path in one call. `update-index` applies its arguments in order, so the line-ending check still sees the index entry before it adds the file.

## Risks / Trade-offs

- [A setting changes during a command] → A command now sees the setting as it was at its first read. A command takes under a second, and the old code read each setting at a different moment anyway.
- [`config --list` and `--get` differ for an unusual key] → Section and variable names are case-insensitive and subsection names are case-sensitive; the map key keeps the subsection's case. The settings that the code reads have no subsection.
- [One `update-index` call orders its work differently] → The tests of CRLF snapshots and filters cover the line-ending and filter cases.

## Migration Plan

None. No behavior or file changes.
