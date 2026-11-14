# Proposal

## Why

The time of a `git-multistage` command is close to the number of `git` processes it starts times 13 to 15 ms on macOS. After `faster-commands-and-tests`, a `status` still starts 14 processes (about 190 ms), each edit hook 10 to 22 (150 to 330 ms), and a `commit` 43 (about 650 ms). The editor hooks run before and after every agent edit, so this cost lands on every edit. Many of the calls repeat a lookup whose answer cannot change during the command, such as the tree of one commit (3 to 4 times in each command), or read a value that an earlier call could also have returned.

A Go Git library could answer some reads inside the process. The design weighs that option and rejects it: the reads that a library would replace are the same ones that caching and folding remove, and a library adds dependencies to the hook and misses Git features that the specs require.

## What Changes

- Repository setup reads the default index path in its one `rev-parse` call.
- The tree of a commit is read once in each process and reused.
- Git settings are read with one `git config` call in each process, the first time that a command needs one.
- A file snapshot adds the file to its temporary index with one `update-index` call, not two.
- No behavior changes. Outputs, exit codes, and files on disk stay the same.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change alters no behavior, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `internal/gitx/gitx.go`: `Open`, `TreeOf`, and a cached settings reader.
- `internal/ms/stagearea.go`: `OpenRepo` and `configBool`.
- `internal/ms/snapshot.go`: `cleanFile`.
- Every command and both edit hooks start fewer `git` processes. The measurements before and after go in `tasks.md`.
