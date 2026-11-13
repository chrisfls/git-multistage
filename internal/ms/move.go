package ms

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// `rm` and `mv` are the only commands that write the working tree. Each
// adopts the stage's index and metadata first, with the disk change in
// Meta.PendingDisk, and changes the disk last, so a crash between the two
// leaves a stage that a second run of the same command finishes.

const (
	cmdRm = "rm"
	cmdMv = "mv"
)

// diskPath returns the working-tree path of the repository path p.
func (r *Repo) diskPath(p string) string { return filepath.Join(r.Top, filepath.FromSlash(p)) }

// RemovePaths stages the deletion of paths, forgets their recorded edits,
// then deletes the files from the working tree. A file that differs from the
// stage's index needs force.
func (l *Stage) RemovePaths(paths []string, force bool) error {
	if l.pendingDisk(cmdRm, paths) {
		return l.finishDisk()
	}
	if err := l.canonicalPaths(paths); err != nil {
		return err
	}
	blobs, err := l.checkRemove(paths, force)
	if err != nil {
		return err
	}
	moves, pending := l.M.Moves, l.M.PendingDisk
	l.M.Moves = dropMoves(moves, paths)
	l.M.PendingDisk = &DiskChange{Command: cmdRm, Paths: paths, Blobs: blobs}
	err = l.withNewIndex(func(idx string) error {
		_, err := l.r.Run(gitx.Opt{Index: idx}, append([]string{"update-index", "--force-remove", "--"}, paths...)...)
		return err
	})
	if err != nil {
		l.M.Moves, l.M.PendingDisk = moves, pending
		return err
	}
	return l.finishDisk()
}

// pendingDisk reports whether the stage's pending disk change is the one of
// the command with paths.
func (l *Stage) pendingDisk(command string, paths []string) bool {
	d := l.M.PendingDisk
	return d != nil && d.Command == command && slices.Equal(d.Paths, paths)
}

// finishDisk makes the stage's pending disk change, then forgets it. It
// changes only a path that still holds the expected blob, so it never
// deletes or overwrites what another writer wrote since.
func (l *Stage) finishDisk() error {
	d := l.M.PendingDisk
	var err error
	if d.Command == cmdRm {
		_, err = l.DropEdits(d.Paths)
	} else {
		err = l.moveRecords(d.Paths[0], d.Paths[1])
	}
	if err != nil {
		return err
	}
	fault("before-disk-write")
	var kept []string
	if d.Command == cmdRm {
		kept, err = l.removeFiles(d)
	} else {
		kept, err = l.moveFile(d)
	}
	if err != nil {
		return err
	}
	if len(kept) > 0 {
		l.warn("left %s on disk: another writer changed it since the stage changed", strings.Join(kept, ", "))
	}
	l.M.PendingDisk = nil
	return l.save()
}

// removeFiles deletes the files of d that hold their expected blob, and
// returns the paths that it kept.
func (l *Stage) removeFiles(d *DiskChange) ([]string, error) {
	var kept []string
	for i, p := range d.Paths {
		blob, err := l.diskBlob(p)
		switch {
		case err != nil:
			return nil, err
		case blob == "":
		case blob != d.Blobs[i]:
			kept = append(kept, p)
		default:
			if err := os.Remove(l.r.diskPath(p)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
	}
	return kept, nil
}

// moveFile moves the file of d when the old path holds its expected blob,
// and returns the old path when another writer changed it.
func (l *Stage) moveFile(d *DiskChange) ([]string, error) {
	oldp, newp := d.Paths[0], d.Paths[1]
	if d.Blobs[0] == "" {
		return nil, nil // the disk showed the move already
	}
	blob, err := l.diskBlob(oldp)
	if err != nil || blob == "" {
		return nil, err
	}
	if blob != d.Blobs[0] {
		return []string{oldp}, nil
	}
	if err := os.MkdirAll(filepath.Dir(l.r.diskPath(newp)), 0o777); err != nil {
		return nil, err
	}
	return nil, os.Rename(l.r.diskPath(oldp), l.r.diskPath(newp))
}

// diskBlob returns the blob of the working-tree file p, or "" when it does
// not exist.
func (l *Stage) diskBlob(p string) (string, error) {
	if !exists(l.r.diskPath(p)) {
		return "", nil
	}
	return l.r.snapshotBlob(l.IndexPath(), p)
}

// checkRemove checks that rm may delete paths, and returns the blob that
// the disk holds at each.
func (l *Stage) checkRemove(paths []string, force bool) ([]string, error) {
	var blobs, untracked, changed []string
	for _, p := range paths {
		entry, ok, err := l.r.lsIndex(l.IndexPath(), p)
		if err != nil {
			return nil, err
		}
		if !ok {
			untracked = append(untracked, p)
			continue
		}
		blob, differs, err := l.r.diskDiffers(l.IndexPath(), p, entry)
		if err != nil {
			return nil, err
		}
		blobs = append(blobs, blob)
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
	return blobs, nil
}

// diskDiffers returns the blob of the working-tree file p, "" when it does
// not exist, and whether it differs from entry. A directory at p is a usage
// error.
func (r *Repo) diskDiffers(idx, p string, entry indexEntry) (string, bool, error) {
	info, err := os.Lstat(r.diskPath(p))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.IsDir() {
		return "", false, usageErr("path %s is a directory; pass files", p)
	}
	blob, err := r.snapshotBlob(idx, p)
	if err != nil {
		return "", false, err
	}
	if blob != entry.oid {
		return blob, true, nil
	}
	if entry.mode == "100644" || entry.mode == "100755" {
		return blob, info.Mode().IsRegular() && r.newFileMode(info) != entry.mode, nil
	}
	return blob, false, nil
}

// MovePath stages the move of oldp to newp with the content that the stage
// holds for oldp, makes the recorded edits of oldp follow it, then moves the
// file in the working tree. When the disk already shows the move, the disk
// is left alone.
func (l *Stage) MovePath(oldp, newp string) error {
	if l.pendingDisk(cmdMv, []string{oldp, newp}) {
		return l.finishDisk()
	}
	var err error
	if oldp, err = l.canonical(oldp); err != nil {
		return err
	}
	if oldp == newp {
		return usageErr("cannot move %s onto itself", oldp)
	}
	entry, err := l.checkMoveIndex(oldp, newp)
	if err != nil {
		return err
	}
	oldOnDisk, err := l.r.checkMoveDisk(oldp, newp)
	if err != nil {
		return err
	}
	blob := ""
	if oldOnDisk {
		if blob, err = l.diskBlob(oldp); err != nil {
			return err
		}
	}
	moves, pending := l.M.Moves, l.M.PendingDisk
	l.M.Moves = addMove(moves, oldp, newp)
	l.M.PendingDisk = &DiskChange{Command: cmdMv, Paths: []string{oldp, newp}, Blobs: []string{blob, ""}}
	err = l.withNewIndex(func(idx string) error {
		if _, err := l.r.Run(gitx.Opt{Index: idx}, "update-index", "--force-remove", "--", oldp); err != nil {
			return err
		}
		return l.r.setIndexBlob(idx, newp, entry.mode, entry.oid)
	})
	if err != nil {
		l.M.Moves, l.M.PendingDisk = moves, pending
		return err
	}
	return l.finishDisk()
}

// checkMoveIndex returns the stage's entry of oldp, and checks that the
// stage does not hold newp.
func (l *Stage) checkMoveIndex(oldp, newp string) (indexEntry, error) {
	entry, inIndex, err := l.r.lsIndex(l.IndexPath(), oldp)
	if err != nil {
		return entry, err
	}
	_, newInIndex, err := l.r.lsIndex(l.IndexPath(), newp)
	if err != nil {
		return entry, err
	}
	switch {
	case !inIndex:
		return entry, &Error{
			Code: ExitError, Kind: "untracked-path",
			Message: "the stage's index does not hold " + oldp + "; nothing was moved", Paths: []string{oldp},
		}
	case newInIndex:
		return entry, &Error{
			Code: ExitRefused, Kind: "path-exists",
			Message: "the stage's index already holds " + newp + "; nothing was moved", Paths: []string{newp},
		}
	}
	return entry, nil
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
