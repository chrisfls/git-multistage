# Proposal

## Why

The editor hooks run before and after every agent edit. After `fewer-git-processes`, one hooked edit still starts 22 `git` processes, and 6 of them store a snapshot of the file. Each snapshot adds the file to a temporary index: one `ls-files` reads the file's entry in the stage index, one `update-index` adds the file after that entry, and one `ls-files` reads the blob ID back. The entry matters only for Git's rule that keeps CRLF line endings when the index blob already has CR. A file without any CR byte gets the same blob whatever the index holds, and most source files have no CR.

## What Changes

- A snapshot of a file, or of an Edit's content, that holds no CR byte is stored with one `git hash-object -w --path=<path>`, which applies the same attributes and filters.
- A file with CR keeps the temporary-index path, so the CRLF rule still sees the index entry.
- No behavior changes. Outputs, exit codes, and files on disk stay the same.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The change alters no behavior, so `.openspec.yaml` sets `skip_specs: true`.

## Impact

- `internal/ms/snapshot.go`: `snapshotBlob` and `cleanContent`.
- `test/hook_test.go`: a hooked edit through a clean filter.
- Each hooked edit, and `add`, `mv`, and `rm` paths that snapshot a file, start fewer `git` processes. The measurements before and after go in `tasks.md`.
