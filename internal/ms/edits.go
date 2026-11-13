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
	Tip       string    `json:"tip,omitempty"`       // the tip of the stage's branch when the edit was made
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

// followRecords returns the records to apply. Where each path that the
// stage's index no longer holds went is worked out once, from its oldest
// record, and applies to every record of the path: a moved path is applied
// at its new path, a path that the stage itself deleted drops its records,
// a path that a commit deleted fails with path-deleted, and a path that no
// commit deleted fails with edit-conflict. The records
// of a deleted path of recreate become one record that creates the file
// with the content of the newest of them.
func (l *Stage) followRecords(chosen []EditRecord, recreate []string) ([]EditRecord, error) {
	gone, err := l.gonePaths(chosen)
	if err != nil {
		return nil, err
	}
	moved := map[string]string{}
	g := &goneRecords{newest: map[string]EditRecord{}}
	recreated, dropped := map[string]bool{}, map[string]bool{}
	for _, rec := range gone {
		n, ok, deleting, err := l.followPath(rec)
		switch {
		case err != nil:
			return nil, err
		case ok:
			moved[rec.Path] = n
		case deleting == "" && n == "":
			dropped[rec.Path] = true
		case deleting == "":
			g.unheld = append(g.unheld, rec.Path+": the stage does not hold this file, and no commit on the branch deleted it")
		case slices.Contains(recreate, rec.Path):
			recreated[rec.Path] = true
		default:
			g.paths = append(g.paths, rec.Path)
			g.details = append(g.details, rec.Path+" was deleted by "+deleting)
		}
	}
	var out []EditRecord
	for _, rec := range chosen {
		if dropped[rec.Path] {
			continue
		}
		if n, ok := moved[rec.Path]; ok {
			rec.Path = n
		}
		if recreated[rec.Path] {
			g.newest[rec.Path] = rec
			continue
		}
		out = append(out, rec)
	}
	return g.finish(out, recreate)
}

// gonePaths returns the oldest record of each path whose oldest record
// edits an existing file that the stage's index does not hold.
func (l *Stage) gonePaths(chosen []EditRecord) ([]EditRecord, error) {
	seen := map[string]bool{}
	var gone []EditRecord
	for _, rec := range chosen {
		if seen[rec.Path] {
			continue
		}
		seen[rec.Path] = true
		if rec.From != "" || rec.Before == "" {
			continue
		}
		_, ok, err := l.r.lsIndex(l.IndexPath(), rec.Path)
		if err != nil {
			return nil, err
		}
		if !ok {
			gone = append(gone, rec)
		}
	}
	return gone, nil
}

// followPath follows rec.Path through the commits that the branch reached
// after the record, then through the stage's own moves. It returns the final
// path when the stage's index holds it. Otherwise it returns the commit that
// deleted the path; or "" and "" when the stage deleted a followed path that
// its base holds; or the path and "" when no commit removed the path.
func (l *Stage) followPath(rec EditRecord) (string, bool, string, error) {
	p, deleting, err := l.followCommits(rec)
	if err != nil || deleting != "" {
		return "", false, deleting, err
	}
	p = followStageMoves(p, l.M.Moves)
	if p == rec.Path {
		return p, false, "", nil
	}
	_, held, err := l.r.lsIndex(l.IndexPath(), p)
	if err != nil || held {
		return p, held, "", err
	}
	_, onBase, err := l.r.lsTree(l.M.Base, p)
	if err != nil || onBase {
		return "", false, "", err
	}
	return p, false, "", nil
}

// followStageMoves returns the path where the stage's moves put p.
func followStageMoves(p string, moves []Move) string {
	for _, m := range moves {
		if m.Old == p {
			return m.New
		}
	}
	return p
}

// followCommits follows rec.Path through the commits that the branch
// reached after the record, as followRemoval does. A record without a tip,
// written by an earlier version, follows the commits dated after the record.
func (l *Stage) followCommits(rec EditRecord) (string, string, error) {
	if rec.Tip == "" {
		tip, err := l.r.ResolveRef(l.M.Branch)
		if err != nil || tip == "" {
			return rec.Path, "", err // an unborn branch has no commits
		}
		return l.r.followRemoval(rec.Path, "", fmt.Sprintf("--since=%d", rec.Time.Unix()-1), l.M.Branch)
	}
	return l.r.followRemoval(rec.Path, rec.Tip, "", l.M.Branch)
}

// followRemoval follows p through the commits of from..branch, oldest
// first: from the oldest commit that moved the path, by its
// Multistage-Moved trailers or else Git's rename detection on that commit,
// it goes on at the new path. It returns the path where the commits left
// it, or, when commits removed it and none moved it, the "<short>
// <subject>" of the newest commit that removed it. A since option, when
// set, limits the commits by date.
func (r *Repo) followRemoval(p, from, since, branch string) (string, string, error) {
	for {
		n, c, desc, err := r.removal(p, from, since, branch)
		switch {
		case err != nil:
			return "", "", err
		case n != "":
			p, from, since = n, c, ""
		case desc == "":
			return p, "", nil
		default:
			return "", desc, nil
		}
	}
}

// removal returns where the commits of from..branch (of branch when from
// is "") moved p: the new path and the oldest commit that moved it. When
// none moved it, it returns "" and the "<short> <subject>" of the newest
// commit that removed p, or "" when none removed it. A since option, when
// set, limits the commits by date.
func (r *Repo) removal(p, from, since, branch string) (newp, commit, desc string, err error) {
	revs := branch
	if from != "" {
		revs = from + ".." + branch
	}
	args := []string{cmdLog, "--reverse", "--no-renames", "--diff-filter=D", "--format=%H %h %s"}
	if since != "" {
		args = append(args, since)
	}
	out, err := r.Run(gitx.Opt{}, append(args, revs, "--", p)...)
	if err != nil {
		return "", "", "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		c, d, _ := strings.Cut(line, " ")
		if c == "" {
			continue
		}
		n, err := r.movedIn(c, p)
		if err != nil || n != "" {
			return n, c, "", err
		}
		desc = d
	}
	return "", "", desc, nil
}

// movedIn returns the path where commit c moved p: the path that its
// Multistage-Moved trailers name, or else the one that Git's rename
// detection finds; "" when c deleted p.
func (r *Repo) movedIn(c, p string) (string, error) {
	moves, err := r.trailerMoves(c + "^!")
	if err != nil {
		return "", err
	}
	if n, ok := moves[p]; ok {
		return n, nil
	}
	entries, err := r.DiffTrees(c+"^", c, true)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Status == 'R' && e.OldPath == p {
			return e.Path, nil
		}
	}
	return "", nil
}

// goneRecords collects the records of files that a commit deleted, and of
// files that no commit deleted and the stage does not hold.
type goneRecords struct {
	paths, details []string
	unheld         []string              // the details of the files that no commit deleted
	newest         map[string]EditRecord // the newest record of each path to recreate
}

// finish fails with edit-conflict for a file that no commit deleted and the
// stage does not hold, then with path-deleted when a deleted path is not
// recreated, and otherwise adds to out one record that creates each
// recreated file.
func (g *goneRecords) finish(out []EditRecord, recreate []string) ([]EditRecord, error) {
	if len(g.unheld) > 0 {
		return nil, editConflict(g.unheld)
	}
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
		return nil, editConflict(failed)
	}
	return staged, nil
}

// editConflict is the error of recorded edits that do not fit the stage.
func editConflict(details []string) *Error {
	return &Error{
		Code: ExitConflict, Kind: "edit-conflict",
		Message: "some recorded edits do not fit the stage; nothing was staged", Details: details,
		Hint: "an edit that changes lines another stage has not committed yet fits once that stage commits; " +
			"or stage those files with `hunks` and `add --hunk`, or forget the records with `git multistage drop --edits <path>`",
	}
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
		var err error
		if before, err = r.EnsureEmptyBlob(); err != nil {
			return nil, err
		}
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
