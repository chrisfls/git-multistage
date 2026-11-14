package ms

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// fillIndex writes an index that holds tree at path. An index read from a
// tree has no stat data, so its refresh hashes every file of the worktree.
// fillIndex starts from a copy of a stage index with stat data when it can,
// so that the refresh hashes only the files whose entries differ from it.
func (l *Stage) fillIndex(path, tree string) error {
	if l.M.Index != "" && l.r.KnownIndexTree(l.IndexPath()) == tree {
		// The current index holds tree already, as after a commit whose tree
		// is the staged tree: a copy needs only the refresh.
		if err := copyFile(l.IndexPath(), path); err != nil {
			return err
		}
		return l.r.RefreshIndex(path)
	}
	if seed := l.statSeed(); seed != "" {
		if l.r.resetCopy(seed, path, tree) == nil {
			return l.r.RefreshIndex(path)
		}
		// The seed is gone, as when its stage replaced its index meanwhile,
		// or it may hold a bit that an index read from tree lacks.
		_ = os.Remove(path)
	}
	return l.r.ReadTree(path, tree)
}

// statSeed returns the index file that a new index generation of the stage
// starts from: the stage's current index, or for a new stage the current
// index of the other stage of the worktree that changed last. It returns ""
// when there is none, or with core.ignoreStat, under which Git marks the
// entries that it updates assume-unchanged. The default index never serves,
// because it can hold a user's assume-unchanged and skip-worktree bits.
func (l *Stage) statSeed() string {
	if l.r.ConfigBool("core.ignoreStat", false) {
		return ""
	}
	if l.M.Index != "" {
		return l.IndexPath()
	}
	return l.r.newestStageIndex(l.ID)
}

// newestStageIndex returns the current index of the stage other than id
// whose directory changed last, or "". Every metadata write renames
// meta.json into the stage directory. It reads the metadata without the
// stage's lock.
func (r *Repo) newestStageIndex(id string) string {
	entries, err := os.ReadDir(r.stagesDir())
	if err != nil {
		return ""
	}
	var newest string
	var changed time.Time
	for _, e := range entries {
		if !e.IsDir() || e.Name() == id || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(changed) {
			newest, changed = e.Name(), info.ModTime()
		}
	}
	if newest == "" {
		return ""
	}
	dir := filepath.Join(r.stagesDir(), newest)
	m, err := readMeta(dir)
	if err != nil || m.Index == "" {
		return ""
	}
	return filepath.Join(dir, m.Index)
}

// resetCopy copies the index file seed to path and makes the copy hold tree.
// read-tree --reset keeps the entries whose mode and object ID match tree,
// with their stat data and their bits, so the copy must be a version 2
// index, which holds no skip-worktree and no intent-to-add bit. The copy
// keeps the modification time of seed, which Git's checks of racily clean
// entries rely on.
func (r *Repo) resetCopy(seed, path, tree string) error {
	if err := copyFile(seed, path); err != nil {
		return err
	}
	if !indexV2(path) {
		return errNotIndexV2
	}
	if r.KnownIndexTree(path) == tree {
		return nil
	}
	_, err := r.Run(gitx.Opt{Index: path}, "read-tree", "--reset", tree)
	return err
}

var errNotIndexV2 = errors.New("not a version 2 index")

// indexV2 reports whether the index file at path has format version 2.
func indexV2(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	var header [8]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return false
	}
	return string(header[:4]) == "DIRC" && binary.BigEndian.Uint32(header[4:]) == 2
}
