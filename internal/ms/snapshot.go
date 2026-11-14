package ms

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// entryIfIndex returns the entry of path in indexFile, when indexFile is
// set and exists.
func (r *Repo) entryIfIndex(indexFile, path string) (indexEntry, bool, error) {
	if indexFile == "" {
		return indexEntry{}, false, nil
	}
	if _, err := os.Stat(indexFile); err != nil {
		return indexEntry{}, false, nil //nolint:nilerr // a missing index cleans the file as a new file
	}
	return r.lsIndex(indexFile, path)
}

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
	if info, err := os.Lstat(full); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		// An index stores a symbolic link as its target name, which
		// hash-object would follow.
		target, err := os.Readlink(full)
		if err != nil {
			return "", err
		}
		return r.Line(gitx.Opt{Stdin: strings.NewReader(target)}, "hash-object", "-w", "--stdin")
	} else if err == nil && !info.Mode().IsRegular() {
		return r.Line(gitx.Opt{}, "hash-object", "-w", "--path="+path, "--", path)
	} else if err == nil {
		if id, ok, err := r.hashIfNoCR(full, path); ok || err != nil {
			return id, err
		}
	}
	dir, err := r.tempDir("snapshot-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	return r.cleanFile(gitx.Opt{}, indexFile, filepath.Join(dir, "index"), path)
}

// cleanContent stores content as the blob that snapshotBlob would store if
// the working-tree file at path held it. The content is written to a
// temporary work tree that holds the .gitattributes files of path's
// directories, so attributes and filters apply as in the real work tree.
func (r *Repo) cleanContent(indexFile, path string, content []byte) (string, error) {
	if !bytes.Contains(content, []byte{'\r'}) {
		// As in hashIfNoCR, content without CR needs no index. hash-object
		// reads the attributes of path from the real work tree.
		return r.Line(gitx.Opt{Stdin: bytes.NewReader(content)}, "hash-object", "-w", "--stdin", "--path="+path)
	}
	dir, err := r.tempDir("clean-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	wt := filepath.Join(dir, "wt")
	file := filepath.Join(wt, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(file), 0o777); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, content, 0o666); err != nil {
		return "", err
	}
	for d := filepath.Dir(filepath.FromSlash(path)); ; d = filepath.Dir(d) {
		if attrs, err := os.ReadFile(filepath.Join(r.Top, d, ".gitattributes")); err == nil {
			if err := os.WriteFile(filepath.Join(wt, d, ".gitattributes"), attrs, 0o666); err != nil {
				return "", err
			}
		}
		if d == "." || d == string(filepath.Separator) {
			break
		}
	}
	o := gitx.Opt{Dir: wt, Env: []string{"GIT_DIR=" + r.GitDir, "GIT_WORK_TREE=" + wt}}
	return r.cleanFile(o, indexFile, filepath.Join(dir, "index"), path)
}

// hashIfNoCR stores the file full as the blob of path when the file holds
// no CR. Without CR, the line-ending rules give the same blob whatever an
// index holds, so hash-object, which reads no index, stores the blob that
// cleanFile would. ok is false for a file with CR.
func (r *Repo) hashIfNoCR(full, path string) (id string, ok bool, err error) {
	f, err := os.Open(full)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 64*1024)
	for {
		n, err := f.Read(buf)
		if bytes.IndexByte(buf[:n], '\r') >= 0 {
			return "", false, nil
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", false, err
		}
	}
	id, err = r.Line(gitx.Opt{}, "hash-object", "-w", "--path="+path, "--", path)
	return id, true, err
}

// tempDir creates a temporary directory in the repository's tmp directory.
func (r *Repo) tempDir(prefix string) (string, error) {
	tmpDir := filepath.Join(r.Dir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o777); err != nil {
		return "", err
	}
	return os.MkdirTemp(tmpDir, prefix)
}

// cleanFile adds the file at path of o's work tree to the temporary index
// idx, after the entry of path in indexFile, and returns the blob ID.
func (r *Repo) cleanFile(o gitx.Opt, indexFile, idx, path string) (string, error) {
	entry, ok, err := r.entryIfIndex(indexFile, path)
	if err != nil {
		return "", err
	}
	o.Index = idx
	// update-index applies its arguments in order, so the line-ending check
	// of the file sees the entry from indexFile.
	args := []string{cmdUpdateIndex, "--add"}
	if ok && entry.mode != "120000" && entry.mode != "160000" {
		args = append(args, "--cacheinfo", entry.mode, entry.oid, path)
	}
	if _, err := r.Run(o, append(args, "--", path)...); err != nil {
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
