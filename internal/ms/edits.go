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
)

// An edit record is one file edit that an agent's editor hook observed: the
// file's content right before and right after the edit. `add --edits`
// replays exactly these changes into the stage, so the stage receives its
// own edits and nothing that other writers changed in the same file.

// EditRecord is one recorded edit. An empty Before means the file did not
// exist; an empty After means the edit deleted it. A record with From is a
// move of the file From to Path, with no change of its content.
type EditRecord struct {
	Path      string    `json:"path"`
	From      string    `json:"from,omitempty"`
	Before    string    `json:"before,omitempty"`
	After     string    `json:"after,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	Tool      string    `json:"tool,omitempty"`
	Uncertain bool      `json:"uncertain,omitempty"` // written by older versions; ignored
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
// and removes the records it staged. Each path of recreate that a commit
// deleted since its records is staged as a new file.
func (l *Stage) AddEdits(paths []string, force bool, recreate []string) ([]string, error) {
	recs, err := l.records()
	if err != nil {
		return nil, err
	}
	chosen := chooseRecords(recs, paths)
	if len(chosen) == 0 {
		return nil, errorf(ExitError, "no-edits", "no recorded edits to stage")
	}
	apply, err := l.followRecords(chosen, recreate)
	if err != nil {
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
	staged, err := l.r.applyRecords(idx, apply, force)
	if err != nil {
		return nil, err
	}
	// The metadata save that adopts the index also names the applied
	// records, so a crash before their deletion cannot apply them twice.
	// Names of earlier records that could not be deleted stay.
	applied, moves := l.M.AppliedEdits, l.M.Moves
	for _, r := range chosen {
		l.M.AppliedEdits = append(l.M.AppliedEdits, r.file)
	}
	for _, r := range apply {
		if r.From != "" {
			l.M.Moves = addMove(l.M.Moves, r.From, r.Path)
		}
	}
	if err := l.adoptIndex(name, gen); err != nil {
		l.M.AppliedEdits, l.M.Moves = applied, moves
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

// followRecords returns the records to apply. A record of a path that the
// stage's index no longer holds follows the Multistage-Moved trailers of the
// commits since the record, or fails with path-deleted when such a commit
// deleted the path. The records of a deleted path of recreate become one
// record that creates the file with the content of the newest of them.
func (l *Stage) followRecords(chosen []EditRecord, recreate []string) ([]EditRecord, error) {
	var out []EditRecord
	gone := &goneRecords{newest: map[string]EditRecord{}}
	created := map[string]bool{} // paths that earlier records create
	var moves map[string]string
	for _, rec := range chosen {
		missing, err := l.recordPathGone(rec, created)
		if err != nil {
			return nil, err
		}
		created[rec.Path] = rec.After != "" || rec.From != ""
		if !missing {
			out = append(out, rec)
			continue
		}
		since := fmt.Sprintf("--since=%d", rec.Time.Unix()-1)
		if moves == nil {
			if moves, err = l.r.trailerMoves(since, l.M.Branch); err != nil {
				return nil, err
			}
		}
		n, ok, err := l.movedTo(rec.Path, moves)
		switch {
		case err != nil:
			return nil, err
		case ok:
			rec.Path = n
			out = append(out, rec)
		case slices.Contains(recreate, rec.Path):
			gone.newest[rec.Path] = rec
		case !slices.Contains(gone.paths, rec.Path):
			gone.paths = append(gone.paths, rec.Path)
			gone.details = append(gone.details, rec.Path+" was deleted by "+l.r.deletingCommit(rec.Path, since, l.M.Branch))
		}
	}
	return gone.finish(out, recreate)
}

// goneRecords collects the records of files that a commit deleted.
type goneRecords struct {
	paths, details []string
	newest         map[string]EditRecord // the newest record of each path to recreate
}

// finish fails with path-deleted when a deleted path is not recreated, and
// otherwise adds to out one record that creates each recreated file.
func (g *goneRecords) finish(out []EditRecord, recreate []string) ([]EditRecord, error) {
	if len(g.paths) > 0 {
		return nil, &Error{
			Code: ExitConflict, Kind: "path-deleted",
			Message: "recorded edits change files that a commit on the branch deleted; nothing was staged",
			Paths:   g.paths, Details: g.details,
			Hint: "stage a file as new with `git multistage add --edits --recreate <path>`, " +
				"or forget its records with `git multistage drop --edits <path>`",
		}
	}
	for _, p := range recreate {
		if rec, ok := g.newest[p]; ok && rec.After != "" {
			out = append(out, EditRecord{Path: p, After: rec.After, Mode: rec.Mode, Tool: rec.Tool, Time: rec.Time})
		}
	}
	return out, nil
}

// movedTo returns the path where moves put p, when the stage's index holds it.
func (l *Stage) movedTo(p string, moves map[string]string) (string, bool, error) {
	n, ok := moves[p]
	if !ok {
		return "", false, nil
	}
	_, ok, err := l.r.lsIndex(l.IndexPath(), n)
	return n, ok, err
}

// recordPathGone reports whether rec edits an existing file that neither the
// stage's index nor an earlier record of created holds.
func (l *Stage) recordPathGone(rec EditRecord, created map[string]bool) (bool, error) {
	if rec.From != "" || rec.Before == "" || created[rec.Path] {
		return false, nil
	}
	_, ok, err := l.r.lsIndex(l.IndexPath(), rec.Path)
	return !ok, err
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
			Hint: "an edit that changes lines another stage has not committed yet fits once that stage commits; " +
				"or stage those files with `hunks` and `add --hunk`, or forget the records with `git multistage drop --edits <path>`",
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
	if rec.From != "" {
		return r.applyMove(idx, rec)
	}
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

// applyMove moves the index entry of rec.From to rec.Path, as `mv` does.
func (r *Repo) applyMove(idx string, rec EditRecord) error {
	cur, ok, err := r.lsIndex(idx, rec.From)
	if err != nil {
		return err
	}
	if !ok {
		return errorf(ExitConflict, "edit-conflict", "the moved file %s is not in the stage", rec.From)
	}
	if _, taken, err := r.lsIndex(idx, rec.Path); err != nil || taken {
		if err != nil {
			return err
		}
		return errorf(ExitConflict, "edit-conflict", "the stage already holds the file that the move creates")
	}
	if _, err := r.Run(gitx.Opt{Index: idx}, "update-index", "--force-remove", "--", rec.From); err != nil {
		return err
	}
	return r.setIndexBlob(idx, rec.Path, cur.mode, cur.oid)
}

// applyNewFile stages a file that the recorded edit created.
func (r *Repo) applyNewFile(idx string, rec EditRecord, force bool) error {
	if !force {
		// Like `git add`, an ignored file needs --force.
		_, err := r.Run(gitx.Opt{Index: idx, Env: []string{envLiteralPathspecs}}, "check-ignore", "-q", "--", rec.Path)
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

func (r *Repo) setIndexBlob(idx, p, mode, oid string) error {
	if mode == "" {
		mode = "100644"
	}
	_, err := r.Run(gitx.Opt{Index: idx}, "update-index", "--add", "--cacheinfo", mode, oid, p)
	return err
}
