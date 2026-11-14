package gitx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// headRef is the ref of the worktree's current branch or commit.
const headRef = "HEAD"

// maxSymrefDepth is how many refs Git reads to resolve one name, following
// symbolic refs (SYMREF_MAXDEPTH in Git's refs.c).
const maxSymrefDepth = 5

// gitSpace is the white space of Git's isspace, which leaves out \v and \f.
const gitSpace = " \t\n\r"

// readRefFiles reads ref from the files ref storage, as Git's files backend
// does. It follows symbolic refs and returns the name of the last ref and its
// object ID; oid is "" when that ref does not exist. A detached HEAD names
// itself. It reads only HEAD and the branches under refs/heads/; ok is false
// when only Git can tell, such as for the reftable storage, a symbolic link,
// or a file that it cannot parse.
func (r *Repo) readRefFiles(ref string) (name, oid string, ok bool) {
	if r.RefFormat != "files" {
		return "", "", false
	}
	name = ref
	for range maxSymrefDepth {
		if name != headRef && (!strings.HasPrefix(name, "refs/heads/") || !validRefName(name)) {
			return "", "", false
		}
		data, found, ok := r.looseRef(name)
		if !ok || (!found && name == headRef) {
			return "", "", false
		}
		if !found {
			oid, ok = r.packedRef(name)
			return name, oid, ok
		}
		content := strings.TrimRight(data, gitSpace)
		if target, sym := strings.CutPrefix(content, "ref:"); sym {
			name = strings.TrimLeft(target, gitSpace)
			continue
		}
		if !r.isObjectID(content) || content == r.ZeroOID {
			return "", "", false
		}
		return name, content, true
	}
	return "", "", false
}

// looseRef reads the loose file of a ref: HEAD in the Git directory, a
// branch in the common directory. A missing file, a path through a file, and
// a directory are no loose ref (found is false), as Git reads them. ok is
// false for a symbolic link, another kind of file, and a failed read.
func (r *Repo) looseRef(name string) (data string, found, ok bool) {
	dir := r.CommonDir
	if name == headRef {
		dir = r.GitDir
	}
	p := filepath.Join(dir, filepath.FromSlash(name))
	info, err := os.Lstat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
		return "", false, true
	case err != nil || !info.Mode().IsRegular() && !info.IsDir():
		return "", false, false
	case info.IsDir():
		return "", false, true
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		// Deleted or packed since the Lstat.
		return "", false, true
	}
	if err != nil {
		return "", false, false
	}
	return string(b), true, true
}

// packedRef looks a ref up in packed-refs: each line is "<oid> <name>", a
// "^<oid>" line peels the tag above it, and a "#" line is the header. oid is
// "" when the ref is not there. ok is false for a failed read and a line that
// is none of these.
func (r *Repo) packedRef(name string) (oid string, ok bool) {
	data, err := os.ReadFile(filepath.Join(r.CommonDir, "packed-refs"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", true
	}
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || line[0] == '#' || line[0] == '^' {
			continue
		}
		id, ref, found := strings.Cut(line, " ")
		if !found || !r.isObjectID(id) {
			return "", false
		}
		if ref == name {
			return id, true
		}
	}
	return "", true
}

// validRefName reports whether name passes Git's ref name rules
// (check_refname_format): every component passes validRefComponent, and the
// name has no ".." and no "@{" and does not end with "." or "/".
func validRefName(name string) bool {
	if name == "" || name == "@" || strings.HasSuffix(name, ".") ||
		strings.Contains(name, "..") || strings.Contains(name, "@{") {
		return false
	}
	for _, c := range strings.Split(name, "/") {
		if !validRefComponent(c) {
			return false
		}
	}
	return true
}

// validRefComponent reports whether c, one component of a ref name, is not
// empty, does not start with "." or end with ".lock", and holds no control
// character and none of ` ~^:?*[\`.
func validRefComponent(c string) bool {
	if c == "" || c[0] == '.' || strings.HasSuffix(c, ".lock") {
		return false
	}
	for i := range len(c) {
		if b := c[i]; b < 0x20 || b == 0x7f || strings.IndexByte(" ~^:?*[\\", b) >= 0 {
			return false
		}
	}
	return true
}
