package ms

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
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

	kindPathExists = "path-exists"
)

// diskPath returns the working-tree path of the repository path p.
func (r *Repo) diskPath(p string) string { return filepath.Join(r.Top, filepath.FromSlash(p)) }

// RemovePaths stages the deletion of paths, forgets their recorded edits,
// then deletes the files from the working tree. A file that differs from the
// stage's index needs force.
func (l *Stage) RemovePaths(paths []string, force bool) error {
	i, err := l.pendingDisk(cmdRm, paths)
	if err != nil || i >= 0 {
		if err != nil {
			return err
		}
		return l.finishPending(i)
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
	l.M.PendingDisk = append(slices.Clone(pending), DiskChange{Command: cmdRm, Paths: paths, Blobs: blobs})
	err = l.withNewIndex(func(idx string) error {
		_, err := l.r.Run(gitx.Opt{Index: idx}, append([]string{"update-index", "--force-remove", "--"}, paths...)...)
		return err
	})
	if err != nil {
		l.M.Moves, l.M.PendingDisk = moves, pending
		return err
	}
	return l.finishDisk(len(l.M.PendingDisk) - 1)
}

// pendingDisk drops the stage's pending disk changes whose change the
// stage's index no longer shows, and returns the position of the one of the
// command with paths; -1 when there is none.
func (l *Stage) pendingDisk(command string, paths []string) (int, error) {
	var kept []DiskChange
	found := -1
	for _, d := range l.M.PendingDisk {
		shown, err := l.showsChange(d)
		if err != nil {
			return -1, err
		}
		if !shown {
			if err := l.leaveTemp(d); err != nil {
				return -1, err
			}
			continue
		}
		if d.Command == command && l.samePaths(d.Paths, paths) {
			found = len(kept)
		}
		kept = append(kept, d)
	}
	l.M.PendingDisk = kept
	return found, nil
}

// samePaths reports whether a command with paths names the paths of a
// pending disk change, as the stage's index spells them. With
// core.ignoreCase, a path may differ in case, except in a case-only move,
// whose two paths differ only in case.
func (l *Stage) samePaths(pending, paths []string) bool {
	if slices.Equal(pending, paths) {
		return true
	}
	if len(pending) != len(paths) || len(paths) == 2 && strings.EqualFold(paths[0], paths[1]) {
		return false
	}
	for i := range paths {
		if !strings.EqualFold(pending[i], paths[i]) {
			return false
		}
	}
	return l.r.configBool("core.ignoreCase", false)
}

// leaveTemp renames the file of d that is at its temporary name back to
// the old path, so that d can be dropped. It fails when the old path is
// taken, and d stays.
func (l *Stage) leaveTemp(d DiskChange) error {
	if d.Temp == "" || !exists(l.r.diskPath(d.Temp)) {
		return nil
	}
	return renameNoReplace(l.r.diskPath(d.Temp), l.r.diskPath(d.Paths[0]))
}

// showsChange reports whether the stage's index still shows the change of
// d: for rm, each path absent; for mv, the old path absent and the new
// path present.
func (l *Stage) showsChange(d DiskChange) (bool, error) {
	for i, p := range d.Paths {
		_, held, err := l.r.lsIndex(l.IndexPath(), p)
		if err != nil {
			return false, err
		}
		if want := d.Command == cmdMv && i == 1; held != want {
			return false, nil
		}
	}
	return true, nil
}

// errTargetExists is the error of a move whose target appeared on disk
// after the check.
var errTargetExists = errors.New("the target of the move exists")

// finishPending finishes the pending disk change at position i for a rerun
// of its command. When the target of a move appeared since, the stage keeps
// the move.
func (l *Stage) finishPending(i int) error {
	newp := l.M.PendingDisk[i].Paths[len(l.M.PendingDisk[i].Paths)-1]
	err := l.finishDisk(i)
	if errors.Is(err, errTargetExists) {
		return &Error{
			Code: ExitRefused, Kind: kindPathExists,
			Message: newp + " already exists in the working tree; the file was not moved", Paths: []string{newp},
			Hint: "the stage holds the move; move or delete " + newp + ", then run the command again",
		}
	}
	return err
}

// finishDisk makes the pending disk change at position i, then forgets it.
// It changes only a path that still holds the expected bytes, so it never
// deletes or overwrites what another writer wrote since. A move makes the
// recorded edits of its old path follow it after the disk change, so a move
// whose target appeared changes no record.
func (l *Stage) finishDisk(i int) error {
	d := l.M.PendingDisk[i]
	if d.Command == cmdRm {
		if err := l.dropEditsOf(d.Paths); err != nil {
			return err
		}
	}
	fault("before-disk-write")
	var kept []string
	var err error
	if d.Command == cmdRm {
		kept, err = l.removeFiles(d)
	} else if kept, err = l.moveFile(&l.M.PendingDisk[i]); err == nil {
		err = l.moveRecords(d.Paths[0], d.Paths[1])
	}
	if err != nil {
		return err
	}
	if len(kept) > 0 {
		l.warn("left %s on disk: another writer changed it since the stage changed", strings.Join(kept, ", "))
	}
	l.M.PendingDisk = slices.Delete(l.M.PendingDisk, i, i+1)
	return l.save()
}

// removeFiles deletes the files of d that hold their expected blob, and
// returns the paths that it kept.
func (l *Stage) removeFiles(d DiskChange) ([]string, error) {
	var kept []string
	for i, p := range d.Paths {
		blob, err := l.r.rawBlob(p)
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

// moveFile moves the file of d when the old path holds its expected bytes,
// and returns the old path when another writer changed it. It fails with
// errTargetExists when the new path exists. A temporary name that d records
// is the old file after the first rename of a case-only move.
func (l *Stage) moveFile(d *DiskChange) ([]string, error) {
	oldp, newp := d.Paths[0], d.Paths[1]
	if d.Temp != "" && exists(l.r.diskPath(d.Temp)) {
		return nil, targetErr(renameNoReplace(l.r.diskPath(d.Temp), l.r.diskPath(newp)))
	}
	if d.Blobs[0] == "" {
		return nil, nil // the disk showed the move already
	}
	blob, err := l.r.rawBlob(oldp)
	if err != nil || blob == "" {
		return nil, err
	}
	if blob != d.Blobs[0] {
		return []string{oldp}, nil
	}
	if err := os.MkdirAll(filepath.Dir(l.r.diskPath(newp)), 0o777); err != nil {
		return nil, err
	}
	testPause("before-rename")
	if l.r.caseOnly(oldp, newp) && (!caseRenameInOneCall || testTwoStepRename()) {
		return nil, targetErr(l.renameCaseOnly(d))
	}
	return nil, targetErr(renameNoReplace(l.r.diskPath(oldp), l.r.diskPath(newp)))
}

// targetErr returns errTargetExists for an error of a rename onto an
// existing file, and err otherwise.
func targetErr(err error) error {
	if errors.Is(err, fs.ErrExist) {
		return errTargetExists
	}
	return err
}

// renameCaseOnly moves the file of d between two spellings of one path,
// through a temporary name that it first records in d, so that a rerun
// finishes the second rename.
func (l *Stage) renameCaseOnly(d *DiskChange) error {
	oldp, newp := d.Paths[0], d.Paths[1]
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	d.Temp = path.Join(path.Dir(oldp), ".git-multistage-"+hex.EncodeToString(b)+"-"+path.Base(oldp))
	if err := l.save(); err != nil {
		return err
	}
	from, tmp := l.r.diskPath(oldp), l.r.diskPath(d.Temp)
	if err := renameNoReplace(from, tmp); err != nil {
		return err
	}
	fault("between-case-renames")
	testPause("between-case-renames")
	if err := renameNoReplace(tmp, l.r.diskPath(newp)); err != nil {
		if rerr := renameNoReplace(tmp, from); rerr != nil {
			// The file keeps the temporary name, so d must stay: the
			// error does not wrap fs.ErrExist, which would roll d back.
			return fmt.Errorf("cannot rename %s to %s (%v), and the file stays at %s: %v", d.Temp, newp, err, d.Temp, rerr) //nolint:errorlint // wrapping err would make the caller drop d
		}
		d.Temp = ""
		return err
	}
	return nil
}

// caseOnly reports whether oldp and newp are two spellings of one existing
// file, as on a case-insensitive file system.
func (r *Repo) caseOnly(oldp, newp string) bool {
	if !strings.EqualFold(oldp, newp) {
		return false
	}
	a, err := os.Lstat(r.diskPath(oldp))
	if err != nil {
		return false
	}
	b, err := os.Lstat(r.diskPath(newp))
	return err == nil && os.SameFile(a, b)
}

// rawBlob returns the ID of the raw bytes of the working-tree file p,
// hashed without filters or line-ending conversion, or "" when it does not
// exist. It does not depend on the stage's index, so a check before and
// after an index change compares the same bytes.
func (r *Repo) rawBlob(p string) (string, error) {
	full := r.diskPath(p)
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(full)
		if err != nil {
			return "", err
		}
		return r.Line(gitx.Opt{Stdin: strings.NewReader(target)}, "hash-object", "--no-filters", "--stdin")
	}
	return r.Line(gitx.Opt{}, "hash-object", "--no-filters", "--", p)
}

// checkRemove checks that rm may delete paths, and returns the raw blob
// that the disk holds at each.
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
		differs, err := l.r.diskDiffers(l.IndexPath(), p, entry)
		if err != nil {
			return nil, err
		}
		blob, err := l.r.rawBlob(p)
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

// diskDiffers reports whether the working-tree file p differs from entry;
// false when it does not exist. A directory at p is a usage error.
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
	i, err := l.pendingDisk(cmdMv, []string{oldp, newp})
	if err != nil || i >= 0 {
		if err != nil {
			return err
		}
		return l.finishPending(i)
	}
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
		if blob, err = l.r.rawBlob(oldp); err != nil {
			return err
		}
	}
	moves, pending := l.M.Moves, l.M.PendingDisk
	l.M.Moves = addMove(moves, oldp, newp)
	l.M.PendingDisk = append(slices.Clone(pending), DiskChange{Command: cmdMv, Paths: []string{oldp, newp}, Blobs: []string{blob, ""}})
	if err := l.withNewIndex(func(idx string) error { return l.r.indexMove(idx, oldp, newp, entry) }); err != nil {
		l.M.Moves, l.M.PendingDisk = moves, pending
		return err
	}
	if err := l.finishDisk(len(l.M.PendingDisk) - 1); !errors.Is(err, errTargetExists) {
		return err
	}
	// A file appeared at newp after the check: restore the stage's previous
	// index and metadata, so the command changes nothing.
	l.M.Moves, l.M.PendingDisk = moves, pending
	if err := l.withNewIndex(func(idx string) error { return l.r.indexMove(idx, newp, oldp, entry) }); err != nil {
		return err
	}
	return &Error{
		Code: ExitRefused, Kind: kindPathExists,
		Message: newp + " already exists in the working tree; nothing was moved", Paths: []string{newp},
	}
}

// indexMove moves the entry of oldp in the index file idx to newp, with the
// mode and blob of entry.
func (r *Repo) indexMove(idx, oldp, newp string, entry indexEntry) error {
	if _, err := r.Run(gitx.Opt{Index: idx}, "update-index", "--force-remove", "--", oldp); err != nil {
		return err
	}
	return r.setIndexBlob(idx, newp, entry.mode, entry.oid)
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
			Code: ExitRefused, Kind: kindPathExists,
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
	case oldOnDisk && newOnDisk && !r.caseOnly(oldp, newp):
		return false, &Error{
			Code: ExitRefused, Kind: kindPathExists,
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

// dropEditsOf forgets the recorded edits of paths, and those of a path that
// the stage's index does not hold and that the branch's commits and the
// stage's moves took to one of paths.
func (l *Stage) dropEditsOf(paths []string) error {
	recs, err := l.records()
	if err != nil {
		return err
	}
	drop := map[string]bool{}
	for _, rec := range recs {
		d, seen := drop[rec.Path]
		if !seen {
			if d, err = l.followsTo(rec, paths); err != nil {
				return err
			}
			drop[rec.Path] = d
		}
		if d {
			if err := os.Remove(filepath.Join(editsDir(l.Dir), rec.file)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

// followsTo reports whether rec edits one of paths, directly or through the
// commits and the stage's moves that took its path, which the stage's index
// does not hold, to one of them.
func (l *Stage) followsTo(rec EditRecord, paths []string) (bool, error) {
	if slices.Contains(paths, rec.Path) {
		return true, nil
	}
	if _, held, err := l.r.lsIndex(l.IndexPath(), rec.Path); err != nil || held {
		return false, err
	}
	p, deleting, err := l.followCommits(rec)
	if err != nil || deleting != "" {
		return false, err
	}
	return slices.Contains(paths, followStageMoves(p, l.M.Moves)), nil
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
