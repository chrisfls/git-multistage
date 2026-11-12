package ms

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// snapshotBlob stores the working-tree file at path as a blob and returns its
// ID. It cleans the file as `git add` does against indexFile: when the entry
// of path in indexFile holds CRLF line endings, core.autocrlf and text=auto
// keep them, as Git keeps them for a file whose index blob already has CR.
// indexFile may be "" or missing, which cleans the file as a new file.
//
// The file is added to a temporary index that holds only the entry of path,
// without stat data, so Git always reads the file again.
func (r *Repo) snapshotBlob(indexFile, path string) (string, error) {
	full := filepath.Join(r.Top, filepath.FromSlash(path))
	if info, err := os.Lstat(full); err == nil && !info.Mode().IsRegular() {
		// An index stores a symbolic link as its target name; keep the content.
		return r.Line(gitx.Opt{}, "hash-object", "-w", "--path="+path, "--", path)
	}
	var entry indexEntry
	var ok bool
	if indexFile != "" {
		if _, err := os.Stat(indexFile); err == nil {
			if entry, ok, err = r.lsIndex(indexFile, path); err != nil {
				return "", err
			}
		}
	}
	tmpDir := filepath.Join(r.Dir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o777); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(tmpDir, "snapshot-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	idx := filepath.Join(dir, "index")
	if ok && entry.mode != "120000" && entry.mode != "160000" {
		if _, err := r.Run(gitx.Opt{Index: idx}, "update-index", "--add", "--cacheinfo", entry.mode, entry.oid, path); err != nil {
			return "", err
		}
	}
	if _, err := r.Run(gitx.Opt{Index: idx}, "update-index", "--add", "--", path); err != nil {
		return "", err
	}
	got, found, err := r.lsIndex(idx, path)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("cannot snapshot %s", path)
	}
	return got.oid, nil
}
