package ms

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// `rm` and `mv` are the only commands that write the working tree. Each
// adopts the stage's index and metadata first and changes the disk last, so
// a crash between the two leaves a stage that a second run finishes.

// diskPath returns the working-tree path of the repository path p.
func (r *Repo) diskPath(p string) string { return filepath.Join(r.Top, filepath.FromSlash(p)) }

// RemovePaths stages the deletion of paths, then deletes the files from the
// working tree. A file that differs from the stage's index needs force.
func (l *Stage) RemovePaths(paths []string, force bool) error {
	if err := l.canonicalPaths(paths); err != nil {
		return err
	}
	stage, err := l.checkRemove(paths, force)
	if err != nil {
		return err
	}
	if len(stage) > 0 {
		moves := l.M.Moves
		l.M.Moves = dropMoves(moves, stage)
		err := l.withNewIndex(func(idx string) error {
			_, err := l.r.Run(gitx.Opt{Index: idx}, append([]string{"update-index", "--force-remove", "--"}, stage...)...)
			return err
		})
		if err != nil {
			l.M.Moves = moves
			return err
		}
	}
	fault("before-disk-write")
	for _, p := range paths {
		if err := os.Remove(l.r.diskPath(p)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// checkRemove checks that rm may delete paths, and returns the paths whose
// deletion is not staged yet.
func (l *Stage) checkRemove(paths []string, force bool) ([]string, error) {
	var stage, untracked, changed []string
	for _, p := range paths {
		entry, ok, err := l.r.lsIndex(l.IndexPath(), p)
		if err != nil {
			return nil, err
		}
		if ok {
			stage = append(stage, p)
		} else if entry, ok, err = l.r.lsTree(l.M.Base, p); err != nil {
			// A second run after a crash finds the deletion already staged.
			return nil, err
		}
		if !ok {
			untracked = append(untracked, p)
			continue
		}
		differs, err := l.r.diskDiffers(l.IndexPath(), p, entry)
		if err != nil {
			return nil, err
		}
		if differs {
			changed = append(changed, p)
		}
	}
	if len(untracked) > 0 {
		return nil, &Error{
			Code: ExitError, Kind: "untracked-path",
			Message: "the stage's index does not hold these paths; nothing was deleted", Paths: untracked,
		}
	}
	if len(changed) > 0 && !force {
		return nil, &Error{
			Code: ExitRefused, Kind: "unstaged-changes",
			Message: "these files differ from the stage's index and may hold other agents' edits; nothing was deleted",
			Paths:   changed, Hint: "pass -f to delete them anyway",
		}
	}
	return stage, nil
}

// diskDiffers reports whether the working-tree file p exists and differs
// from entry. A directory at p is a usage error.
func (r *Repo) diskDiffers(idx, p string, entry indexEntry) (bool, error) {
	info, err := os.Lstat(r.diskPath(p))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.IsDir() {
		return false, usageErr("path %s is a directory; pass files", p)
	}
	blob, err := r.snapshotBlob(idx, p)
	if err != nil {
		return false, err
	}
	if blob != entry.oid {
		return true, nil
	}
	if entry.mode == "100644" || entry.mode == "100755" {
		return info.Mode().IsRegular() && r.newFileMode(info) != entry.mode, nil
	}
	return false, nil
}

// MovePath stages the move of oldp to newp with the content that the stage
// holds for oldp, makes the recorded edits of oldp follow it, then moves the
// file in the working tree. When the disk already shows the move, the disk
// is left alone.
func (l *Stage) MovePath(oldp, newp string) error {
	var err error
	if oldp, err = l.canonical(oldp); err != nil {
		return err
	}
	if oldp == newp {
		return usageErr("cannot move %s onto itself", oldp)
	}
	entry, staged, err := l.checkMoveIndex(oldp, newp)
	if err != nil {
		return err
	}
	oldOnDisk, err := l.r.checkMoveDisk(oldp, newp)
	if err != nil {
		return err
	}
	if !staged {
		moves := l.M.Moves
		l.M.Moves = addMove(moves, oldp, newp)
		err := l.withNewIndex(func(idx string) error {
			if _, err := l.r.Run(gitx.Opt{Index: idx}, "update-index", "--force-remove", "--", oldp); err != nil {
				return err
			}
			return l.r.setIndexBlob(idx, newp, entry.mode, entry.oid)
		})
		if err != nil {
			l.M.Moves = moves
			return err
		}
	}
	if err := l.moveRecords(oldp, newp); err != nil {
		return err
	}
	fault("before-disk-write")
	if !oldOnDisk {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.r.diskPath(newp)), 0o777); err != nil {
		return err
	}
	return os.Rename(l.r.diskPath(oldp), l.r.diskPath(newp))
}

// checkMoveIndex returns the stage's entry of oldp, and whether the stage
// already holds the move, as after a crash of an earlier run.
func (l *Stage) checkMoveIndex(oldp, newp string) (indexEntry, bool, error) {
	entry, inIndex, err := l.r.lsIndex(l.IndexPath(), oldp)
	if err != nil {
		return entry, false, err
	}
	_, newInIndex, err := l.r.lsIndex(l.IndexPath(), newp)
	if err != nil {
		return entry, false, err
	}
	switch {
	case !inIndex && newInIndex && slices.Contains(l.M.Moves, Move{oldp, newp}):
		return entry, true, nil
	case !inIndex:
		return entry, false, &Error{
			Code: ExitError, Kind: "untracked-path",
			Message: "the stage's index does not hold " + oldp + "; nothing was moved", Paths: []string{oldp},
		}
	case newInIndex:
		return entry, false, &Error{
			Code: ExitRefused, Kind: "path-exists",
			Message: "the stage's index already holds " + newp + "; nothing was moved", Paths: []string{newp},
		}
	}
	return entry, false, nil
}

// checkMoveDisk checks that the disk holds exactly one of oldp and newp,
// and reports whether it is oldp.
func (r *Repo) checkMoveDisk(oldp, newp string) (bool, error) {
	oldOnDisk, newOnDisk := exists(r.diskPath(oldp)), exists(r.diskPath(newp))
	switch {
	case oldOnDisk && newOnDisk:
		return false, &Error{
			Code: ExitRefused, Kind: "path-exists",
			Message: newp + " already exists in the working tree; nothing was moved", Paths: []string{newp},
		}
	case !oldOnDisk && !newOnDisk:
		return false, &Error{
			Code: ExitError, Kind: "missing-path",
			Message: "neither " + oldp + " nor " + newp + " exists in the working tree; nothing was moved",
			Paths:   []string{oldp, newp},
		}
	}
	if info, err := os.Lstat(r.diskPath(oldp)); err == nil && info.IsDir() {
		return false, usageErr("path %s is a directory; move its files one at a time", oldp)
	}
	return oldOnDisk, nil
}

// withNewIndex runs f on a copy of the stage's index, and adopts the copy,
// with the current metadata, when f succeeds.
func (l *Stage) withNewIndex(f func(idx string) error) error {
	name, gen := l.newIndexFile()
	idx := filepath.Join(l.Dir, name)
	if err := copyFile(l.IndexPath(), idx); err != nil {
		return err
	}
	if err := f(idx); err != nil {
		_ = os.Remove(idx)
		return err
	}
	if err := l.adoptIndex(name, gen); err != nil {
		_ = os.Remove(idx)
		return err
	}
	return nil
}

// addMove returns moves with the move of oldp to newp. A move of a file that
// an earlier move created extends that move, or cancels it when it returns
// the file to where it was.
func addMove(moves []Move, oldp, newp string) []Move {
	out := slices.Clone(moves)
	for i, m := range out {
		if m.New != oldp {
			continue
		}
		if m.Old == newp {
			return slices.Delete(out, i, i+1)
		}
		out[i].New = newp
		return out
	}
	return append(out, Move{oldp, newp})
}

// dropMoves returns the moves that name none of paths.
func dropMoves(moves []Move, paths []string) []Move {
	var out []Move
	for _, m := range moves {
		if !slices.Contains(paths, m.Old) && !slices.Contains(paths, m.New) {
			out = append(out, m)
		}
	}
	return out
}

// moveRecords makes the pending edit records of oldp name newp.
func (l *Stage) moveRecords(oldp, newp string) error {
	recs, err := l.records()
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if rec.Path != oldp {
			continue
		}
		rec.Path = newp
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if err := writeFileAtomic(filepath.Join(editsDir(l.Dir), rec.file), data); err != nil {
			return err
		}
	}
	return nil
}
