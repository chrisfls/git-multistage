package ms

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/chrisfls/git-multistage/internal/gitx"
	"github.com/chrisfls/git-multistage/internal/hunks"
)

// An edit record is one file edit that an agent's editor hook observed: the
// file's content right before and right after the edit. `add --edits`
// replays exactly these changes into the stage, so the stage receives its
// own edits and nothing that other writers changed in the same file.

// EditRecord is one recorded edit. An empty Before means the file did not
// exist; an empty After means the edit deleted it.
type EditRecord struct {
	Path      string    `json:"path"`
	Before    string    `json:"before,omitempty"`
	After     string    `json:"after,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	Tool      string    `json:"tool,omitempty"`
	Uncertain bool      `json:"uncertain,omitempty"` // another writer may have changed the file during the edit
	Time      time.Time `json:"time"`

	file string
}

func editsDir(stageDir string) string { return filepath.Join(stageDir, "edits") }

// writeRecord stores rec as a new file, so concurrent hooks never collide.
func writeRecord(stageDir string, rec EditRecord) error {
	dir := editsDir(stageDir)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return err
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%020d-%s.json", rec.Time.UnixNano(), hex.EncodeToString(b))
	return writeFileAtomic(filepath.Join(dir, name), data)
}

// readRecords returns the stage's recorded edits, oldest first.
func readRecords(stageDir string) ([]EditRecord, error) {
	entries, err := os.ReadDir(editsDir(stageDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []EditRecord
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".json") || strings.HasPrefix(n, "pending-") || strings.Contains(n, ".tmp-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(editsDir(stageDir), n))
		if err != nil {
			return nil, err
		}
		var rec EditRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("edit record %s: %w", n, err)
		}
		rec.file = n
		recs = append(recs, rec)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].file < recs[j].file })
	return recs, nil
}

// records returns the stage's recorded edits that the index does not hold
// yet, oldest first. Records that the metadata names as applied stay on disk
// only while they cannot be deleted.
func (l *Stage) records() ([]EditRecord, error) {
	recs, err := readRecords(l.Dir)
	if err != nil || len(l.M.AppliedEdits) == 0 {
		return recs, err
	}
	var out []EditRecord
	for _, r := range recs {
		if !slices.Contains(l.M.AppliedEdits, r.file) {
			out = append(out, r)
		}
	}
	return out, nil
}

// RecordedPaths lists the paths that have recorded edits.
func (l *Stage) RecordedPaths() ([]string, error) {
	recs, err := l.records()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var paths []string
	for _, r := range recs {
		if !seen[r.Path] {
			seen[r.Path] = true
			paths = append(paths, r.Path)
		}
	}
	return paths, nil
}

// AddEdits stages the recorded edits of the given paths (all when none),
// and removes the records it staged.
func (l *Stage) AddEdits(paths []string, force bool) ([]string, error) {
	recs, err := l.records()
	if err != nil {
		return nil, err
	}
	chosen := chooseRecords(recs, paths)
	if len(chosen) == 0 {
		return nil, errorf(ExitError, "no-edits", "no recorded edits to stage")
	}
	if err := l.checkRecordsSafe(chosen); err != nil {
		return nil, err
	}

	// Apply the records to a copy of the index that becomes current only
	// when every record applies.
	name, gen := l.newIndexFile()
	idx := filepath.Join(l.Dir, name)
	if err := copyFile(l.IndexPath(), idx); err != nil {
		return nil, err
	}
	adopted := false
	defer func() {
		if !adopted {
			_ = os.Remove(idx)
		}
	}()
	staged, err := l.r.applyRecords(idx, chosen, force)
	if err != nil {
		return nil, err
	}
	// The metadata save that adopts the index also names the applied
	// records, so a crash before their deletion cannot apply them twice.
	// Names of earlier records that could not be deleted stay.
	applied := l.M.AppliedEdits
	for _, r := range chosen {
		l.M.AppliedEdits = append(l.M.AppliedEdits, r.file)
	}
	if err := l.adoptIndex(name, gen); err != nil {
		l.M.AppliedEdits = applied
		return nil, err
	}
	adopted = true
	fault("after-adopt-edits")
	if err := l.purgeAppliedEdits(); err != nil {
		l.warn("cannot delete the staged edit records yet; the next command deletes them: %v", err)
	}
	var out []string
	for p := range staged {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// chooseRecords returns the records of the given paths (all when none).
func chooseRecords(recs []EditRecord, paths []string) []EditRecord {
	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
	}
	var chosen []EditRecord
	for _, r := range recs {
		if len(want) == 0 || want[r.Path] {
			chosen = append(chosen, r)
		}
	}
	return chosen
}

// checkRecordsSafe fails with edit-lost when the newest record of a path is
// no longer on disk, or when a record is uncertain.
func (l *Stage) checkRecordsSafe(chosen []EditRecord) error {
	last := map[string]EditRecord{}
	for _, r := range chosen {
		last[r.Path] = r
	}
	var lost, uncertain []string
	for p, r := range last {
		ok, err := l.r.editOnDisk(l.IndexPath(), r)
		if err != nil {
			return err
		}
		if !ok {
			lost = append(lost, p)
		}
	}
	for _, r := range chosen {
		if r.Uncertain {
			uncertain = append(uncertain, r.Path)
		}
	}
	if len(lost) == 0 && len(uncertain) == 0 {
		return nil
	}
	sort.Strings(lost)
	e := &Error{
		Code: ExitConflict, Kind: "edit-lost",
		Message: "some recorded edits cannot be staged safely; nothing was staged",
		Hint:    "redo the edit, or stage the file with `hunks` and `add --hunk`; `git multistage drop --edits <path>` forgets the records",
	}
	for _, p := range lost {
		e.Details = append(e.Details, p+": the file no longer holds the edit (another writer changed those lines)")
	}
	for _, p := range uncertain {
		e.Details = append(e.Details, p+": another writer may have changed the file during the edit")
	}
	return e
}

// applyRecords applies the records to the index file idx, and returns the
// paths it staged. It fails with edit-conflict when any record does not fit.
func (r *Repo) applyRecords(idx string, chosen []EditRecord, force bool) (map[string]bool, error) {
	var failed []string
	staged := map[string]bool{}
	for _, rec := range chosen {
		if err := r.applyRecord(idx, rec, force); err != nil {
			var e *Error
			if errors.As(err, &e) {
				failed = append(failed, rec.Path+": "+e.Message)
				continue
			}
			return nil, err
		}
		staged[rec.Path] = true
	}
	if len(failed) > 0 {
		return nil, &Error{
			Code: ExitConflict, Kind: "edit-conflict",
			Message: "some recorded edits do not fit the stage; nothing was staged", Details: failed,
			Hint: "stage those files with `hunks` and `add --hunk`, or forget the records with `git multistage drop --edits <path>`",
		}
	}
	return staged, nil
}

// purgeAppliedEdits deletes the edit records that the current index already
// holds, then forgets their names.
func (l *Stage) purgeAppliedEdits() error {
	if len(l.M.AppliedEdits) == 0 {
		return nil
	}
	for _, n := range l.M.AppliedEdits {
		if err := os.Remove(filepath.Join(editsDir(l.Dir), n)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	l.M.AppliedEdits = nil
	return l.save()
}

// DropEdits forgets the recorded edits of the given paths (all when none).
func (l *Stage) DropEdits(paths []string) (int, error) {
	recs, err := l.records()
	if err != nil {
		return 0, err
	}
	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
	}
	n := 0
	for _, r := range recs {
		if len(want) == 0 || want[r.Path] {
			_ = os.Remove(filepath.Join(editsDir(l.Dir), r.file))
			n++
		}
	}
	return n, nil
}

func (r *Repo) blobContent(oid string) ([]byte, error) {
	if oid == "" {
		return nil, nil
	}
	return r.catBlob(oid)
}

// applyRecord replays one recorded edit onto the index entry of its path.
func (r *Repo) applyRecord(idx string, rec EditRecord, force bool) error {
	cur, inIndex, err := r.lsIndex(idx, rec.Path)
	if err != nil {
		return err
	}
	switch {
	case rec.After == "": // deletion
		return r.applyDeletion(idx, rec, cur, inIndex)
	case !inIndex && rec.Before == "": // a new file
		return r.applyNewFile(idx, rec, force)
	case !inIndex:
		return errorf(ExitConflict, "edit-conflict", "the file is not in the stage; it was created by another writer")
	}
	if cur.mode == "120000" || cur.mode == "160000" {
		return errorf(ExitConflict, "edit-conflict", "the stage holds a symbolic link or submodule at this path")
	}
	if cur.oid == rec.Before {
		return r.setIndexBlob(idx, rec.Path, cur.mode, rec.After)
	}
	out, err := r.replayRecord(rec, cur.oid)
	if err != nil {
		return err
	}
	oid, err := r.Line(gitx.Opt{Stdin: strings.NewReader(string(out))}, "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	return r.setIndexBlob(idx, rec.Path, cur.mode, oid)
}

// applyDeletion applies a recorded deletion to the index entry cur.
func (r *Repo) applyDeletion(idx string, rec EditRecord, cur indexEntry, inIndex bool) error {
	if !inIndex {
		return nil
	}
	if cur.oid != rec.Before {
		return errorf(ExitConflict, "edit-conflict", "the deleted file differs in the stage from what was deleted")
	}
	_, err := r.Run(gitx.Opt{Index: idx}, "update-index", "--force-remove", "--", rec.Path)
	return err
}

// applyNewFile stages a file that the recorded edit created.
func (r *Repo) applyNewFile(idx string, rec EditRecord, force bool) error {
	if !force {
		// Like `git add`, an ignored file needs --force.
		_, err := r.Run(gitx.Opt{Index: idx, Env: []string{"GIT_LITERAL_PATHSPECS=0"}}, "check-ignore", "-q", "--", rec.Path)
		if err == nil {
			return errorf(ExitConflict, "edit-conflict", "the file is ignored by .gitignore; pass -f to stage it")
		}
		if gitx.ExitCode(err) != 1 {
			return err
		}
	}
	return r.setIndexBlob(idx, rec.Path, rec.Mode, rec.After)
}

// replayRecord places the lines of the recorded edit into the stage's
// blob cur, which differs from the record's before-content.
func (r *Repo) replayRecord(rec EditRecord, cur string) ([]byte, error) {
	before := rec.Before
	if before == "" {
		before = r.EmptyBlob
	}
	mine, err := r.diffBlobs(rec.Path, before, rec.After)
	if err != nil {
		return nil, errorf(ExitConflict, "edit-conflict", "binary content cannot be replayed")
	}
	theirs, err := r.diffBlobs(rec.Path, before, cur)
	if err != nil {
		return nil, errorf(ExitConflict, "edit-conflict", "binary content cannot be replayed")
	}
	beforeContent, err := r.blobContent(rec.Before)
	if err != nil {
		return nil, err
	}
	curContent, err := r.catBlob(cur)
	if err != nil {
		return nil, err
	}
	out, ok := replayLines(beforeContent, curContent, mine, theirs)
	if !ok {
		return nil, errorf(ExitConflict, "edit-conflict", "the edit overlaps lines that differ between the stage and the file")
	}
	return out, nil
}

// addsBack reports whether later adds any line that the deletion removed.
// A line at the end of the file can lose or gain its newline in the change.
func addsBack(later, deletion hunks.Hunk) bool {
	removed := map[string]bool{}
	for _, l := range deletion.Removed {
		removed[strings.TrimSuffix(l, "\n")] = true
	}
	return slices.ContainsFunc(later.Added, func(l string) bool { return removed[strings.TrimSuffix(l, "\n")] })
}

func (r *Repo) setIndexBlob(idx, p, mode, oid string) error {
	if mode == "" {
		mode = "100644"
	}
	_, err := r.Run(gitx.Opt{Index: idx}, "update-index", "--add", "--cacheinfo", mode, oid, p)
	return err
}

// editOnDisk reports whether the working tree still holds the lines that
// rec added: no later change on disk touched them. idx is the stage index,
// which decides how the file's line endings are cleaned.
func (r *Repo) editOnDisk(idx string, rec EditRecord) (bool, error) {
	full := filepath.Join(r.Top, filepath.FromSlash(rec.Path))
	if rec.After == "" {
		_, err := os.Lstat(full)
		return errors.Is(err, fs.ErrNotExist), nil
	}
	if _, err := os.Lstat(full); err != nil {
		return false, nil //nolint:nilerr // a path that cannot be read no longer holds the edit
	}
	disk, err := r.snapshotBlob(idx, rec.Path)
	if err != nil {
		return false, err
	}
	if disk == rec.After {
		return true, nil
	}
	before := rec.Before
	if before == "" {
		before = r.EmptyBlob
	}
	mine, err := r.diffBlobs(rec.Path, before, rec.After)
	if err != nil {
		return false, nil //nolint:nilerr // hunks that cannot be compared, such as binary content, cannot show that the edit survived
	}
	later, err := r.diffBlobs(rec.Path, rec.After, disk)
	if err != nil {
		return false, nil //nolint:nilerr // hunks that cannot be compared, such as binary content, cannot show that the edit survived
	}
	for _, h := range mine {
		for _, d := range later {
			if overwrites(d, h) {
				return false, nil
			}
		}
	}
	return true, nil
}

// overwrites reports whether the later change d undid the recorded hunk h.
// h's added lines occupy [NewStart-1, NewStart-1+NewLines) of After; a later
// change that removes any of them, or inserts inside them, overwrote the
// edit. A deletion adds no lines: it sits between lines NewStart and
// NewStart+1 of After. A later change there puts the deletion back when it
// adds any of the deleted lines: an insertion after line NewStart, or a
// replacement whose old lines include line NewStart or NewStart+1. A change
// of a neighbouring line alone adds none of them.
func overwrites(d, h hunks.Hunk) bool {
	if h.NewLines == 0 {
		if d.NewLines == 0 {
			return false
		}
		atPoint := d.OldLines == 0 && d.OldStart == h.NewStart ||
			d.OldLines > 0 && d.OldStart <= h.NewStart+1 && h.NewStart <= d.OldStart+d.OldLines-1
		return atPoint && addsBack(d, h)
	}
	lo, hi := h.NewStart-1, h.NewStart-1+h.NewLines
	if d.OldLines > 0 {
		dlo, dhi := d.OldStart-1, d.OldStart-1+d.OldLines
		return dlo < hi && lo < dhi
	}
	return lo < d.OldStart && d.OldStart < hi
}
