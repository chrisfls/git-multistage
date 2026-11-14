# Design

## Context

`snapshotBlob` (`internal/ms/snapshot.go`) stores a working-tree file as the blob that `git add` would store against an index, and `cleanContent` stores given content the same way for an Edit's own change. Both build a temporary index that holds only the file's entry from the stage index, then run `update-index --add` and read the blob back with `ls-files -s`.

In Git's `convert.c`, `crlf_to_git` reads the index only through `has_crlf_in_index`, and only to stop a CRLF-to-LF conversion. When the content holds no CRLF, there is nothing to convert, and the round-trip check of `core.safecrlf` sees the same counts either way. The clean filter, `ident`, and `working-tree-encoding` do not read the index.

The full suite logged 9,126 `git` processes in one run. `git-multistage` started 8,529 of them and the test helpers 579, so the second pass cuts the tool's processes, not the tests'.

## Goals / Non-Goals

**Goals:**
- Fewer `git` processes for each snapshot, with the same blob IDs.

**Non-Goals:**
- Writing blobs in Go. The tool implements no Git format of its own.
- Changing the snapshot of a file with CR.

## Decisions

- **Scan for CR, then `hash-object`.** `snapshotBlob` reads a regular file in 64 KiB blocks and stops at the first CR byte. A file without CR is stored with `hash-object -w --path=<path> -- <path>`, which reads the file again. The scan holds no file in memory. A writer can add CR between the scan and the hash; the old code also read the file at one moment of a concurrent change, and the temporary-index path covers the file again on the next snapshot.
- **`cleanContent` uses `--stdin`.** The content is already in memory, so `hash-object -w --stdin --path=<path>` reads it from the pipe and reads the attributes of `<path>` from the real work tree, the same `.gitattributes` files that the temporary work tree copies.
- **`hash-object` reads no index.** A check in a repository whose `.gitattributes` exists only in the default index showed that `hash-object --path` gives the same blob with the default index and with a missing one. The temporary index of the old code holds no `.gitattributes` either.
- **Rejected: memoize `write-tree` by the index file's stat data.** It saves one process in `add` and `commit`. The stage's hook copy reuses one path, and on a file system with coarse modification times a recycled inode can match stale stat data, which would commit a wrong tree.

Checks against Git 2.54 (Apple Git-157), in a repository with `core.autocrlf=true`, a committed CRLF blob, and a clean filter: for an LF file and a filtered file, `update-index` after the entry and `hash-object --path` gave the same blob; for a CRLF file they differed, which is why it keeps the old path. With `core.safecrlf=true`, both failed with the same message.

## Risks / Trade-offs

- [Another Git version reads the index in `hash-object`] → The test of a filtered edit and the CRLF tests run against the Git on `PATH`; the CI workflow runs them on Linux and Windows.
- [An edit of `.gitattributes` itself] → `cleanContent` copies the real `.gitattributes` over the content when the path is a `.gitattributes` file, so the old path stored the file on disk rather than the Edit's content. The new path stores the content for a file without CR. The old path keeps this for a file with CR; it is not changed here.

## Migration Plan

None. No behavior or file changes.
