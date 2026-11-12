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
	"sort"
	"strings"
	"time"

	"github.com/chrisfls/git-multistage/internal/gitx"
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

// RecordedPaths lists the paths that have recorded edits.
func (l *Stage) RecordedPaths() ([]string, error) {
	recs, err := readRecords(l.Dir)
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
	recs, err := readRecords(l.Dir)
	if err != nil {
		return nil, err
	}
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
	if len(chosen) == 0 {
		return nil, errorf(ExitError, "no-edits", "no recorded edits to stage")
	}
	// The newest record of each path must still be on disk.
	last := map[string]EditRecord{}
	for _, r := range chosen {
		last[r.Path] = r
	}
	var lost, uncertain []string
	for p, r := range last {
		ok, err := l.r.editOnDisk(l.IndexPath(), r)
		if err != nil {
			return nil, err
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
	if len(lost) > 0 || len(uncertain) > 0 {
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
		return nil, e
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
	var failed []string
	staged := map[string]bool{}
	for _, r := range chosen {
		if err := l.r.applyRecord(idx, r, force); err != nil {
			var e *Error
			if errors.As(err, &e) {
				failed = append(failed, r.Path+": "+e.Message)
				continue
			}
			return nil, err
		}
		staged[r.Path] = true
	}
	if len(failed) > 0 {
		return nil, &Error{
			Code: ExitConflict, Kind: "edit-conflict",
			Message: "some recorded edits do not fit the stage; nothing was staged", Details: failed,
			Hint: "stage those files with `hunks` and `add --hunk`, or forget the records with `git multistage drop --edits <path>`",
		}
	}
	if err := l.adoptIndex(name, gen); err != nil {
		return nil, err
	}
	adopted = true
	for _, r := range chosen {
		_ = os.Remove(filepath.Join(editsDir(l.Dir), r.file))
	}
	var out []string
	for p := range staged {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// DropEdits forgets the recorded edits of the given paths (all when none).
func (l *Stage) DropEdits(paths []string) (int, error) {
	recs, err := readRecords(l.Dir)
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
		if !inIndex {
			return nil
		}
		if cur.oid != rec.Before {
			return errorf(ExitConflict, "edit-conflict", "the deleted file differs in the stage from what was deleted")
		}
		_, err := r.Run(gitx.Opt{Index: idx}, "update-index", "--force-remove", "--", rec.Path)
		return err
	case !inIndex && rec.Before == "": // a new file
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
	case !inIndex:
		return errorf(ExitConflict, "edit-conflict", "the file is not in the stage; it was created by another writer")
	}
	if cur.mode == "120000" || cur.mode == "160000" {
		return errorf(ExitConflict, "edit-conflict", "the stage holds a symbolic link or submodule at this path")
	}
	if cur.oid == rec.Before {
		return r.setIndexBlob(idx, rec.Path, cur.mode, rec.After)
	}
	before := rec.Before
	if before == "" {
		before = r.EmptyBlob
	}
	mine, err := r.diffBlobs(rec.Path, before, rec.After)
	if err != nil {
		return errorf(ExitConflict, "edit-conflict", "binary content cannot be replayed")
	}
	theirs, err := r.diffBlobs(rec.Path, before, cur.oid)
	if err != nil {
		return errorf(ExitConflict, "edit-conflict", "binary content cannot be replayed")
	}
	beforeContent, err := r.blobContent(rec.Before)
	if err != nil {
		return err
	}
	curContent, err := r.catBlob(cur.oid)
	if err != nil {
		return err
	}
	out, ok := replayLines(beforeContent, curContent, mine, theirs)
	if !ok {
		return errorf(ExitConflict, "edit-conflict", "the edit overlaps lines that differ between the stage and the file")
	}
	oid, err := r.Line(gitx.Opt{Stdin: strings.NewReader(string(out))}, "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	return r.setIndexBlob(idx, rec.Path, cur.mode, oid)
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
	// mine's added lines occupy [NewStart-1, NewStart-1+NewLines) of After;
	// a later change that removes any of them, or inserts inside them,
	// overwrote the edit.
	for _, h := range mine {
		lo, hi := h.NewStart-1, h.NewStart-1+h.NewLines
		for _, d := range later {
			if d.OldLines > 0 {
				dlo, dhi := d.OldStart-1, d.OldStart-1+d.OldLines
				if dlo < hi && lo < dhi {
					return false, nil
				}
			} else if lo < d.OldStart && d.OldStart < hi {
				return false, nil
			}
		}
	}
	return true, nil
}
