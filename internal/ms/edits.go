package ms

import (
	"bytes"
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
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%020d-%s.json", rec.Time.UnixNano(), randomHex())
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
	reads := &indexReads{}
	apply, err := l.followRecords(chosen, recreate, reads)
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
	staged, err := l.r.applyRecords(idx, apply, force, reads)
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
func (l *Stage) followRecords(chosen []EditRecord, recreate []string, reads *indexReads) ([]EditRecord, error) {
	gone, err := l.gonePaths(chosen, reads)
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
// edits an existing file that the stage's index does not hold. Its one read
// covers the paths of every chosen record, so that applying the records
// reads no path again.
func (l *Stage) gonePaths(chosen []EditRecord, reads *indexReads) ([]EditRecord, error) {
	seen, read := map[string]bool{}, map[string]bool{}
	var oldest []EditRecord
	var paths []string
	for _, rec := range chosen {
		for _, p := range []string{rec.Path, rec.From} {
			if p != "" && !read[p] {
				read[p] = true
				paths = append(paths, p)
			}
		}
		if seen[rec.Path] {
			continue
		}
		seen[rec.Path] = true
		if rec.From != "" || rec.Before == "" {
			continue
		}
		oldest = append(oldest, rec)
	}
	held, err := reads.read(l.r, l.IndexPath(), paths)
	if err != nil {
		return nil, err
	}
	var gone []EditRecord
	for _, rec := range oldest {
		if _, ok := held[rec.Path]; !ok {
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
// The records' writes reach the index file together, after every record
// was tried.
func (r *Repo) applyRecords(idx string, chosen []EditRecord, force bool, reads *indexReads) (map[string]bool, error) {
	var failed []string
	staged := map[string]bool{}
	view, err := r.newIndexView(idx, chosen, reads)
	if err != nil {
		return nil, err
	}
	for _, rec := range chosen {
		if err := r.applyRecord(view, rec, force); err != nil {
			var e *Error
			if errors.As(err, &e) {
				failed = append(failed, rec.Path+": "+e.Message)
				continue
			}
			return nil, err
		}
		staged[rec.Path] = true
	}
	// The writes of the records that applied reach the file even when other
	// records failed, so that a Git failure outranks edit-conflict, as it did
	// when each record wrote. The caller discards the file on any error.
	if err := view.flush(); err != nil {
		return nil, err
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

// indexReads keeps the stage-0 entries that `add --edits` read from index
// files, and the paths that they do not hold, by the checksum of the index
// file. The index copy that the records apply to has the bytes of the stage's
// index, so its view reads no path that the stage's index read.
type indexReads struct {
	bySum map[string]map[string]*indexEntry // a nil entry is a path without a stage-0 entry
}

// read returns the stage-0 entries of paths in idx, as Repo.lsIndexMany does.
// It starts Git only for the paths that no read of an index with the same
// checksum covered. An index without a checksum is always read.
func (c *indexReads) read(r *Repo, idx string, paths []string) (map[string]indexEntry, error) {
	sum := r.IndexChecksum(idx)
	if sum == "" {
		return r.lsIndexMany(idx, paths)
	}
	known := c.bySum[sum]
	var missing []string
	for _, p := range paths {
		if _, ok := known[p]; !ok {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		got, err := r.lsIndexMany(idx, missing)
		if err != nil {
			return nil, err
		}
		if r.IndexChecksum(idx) != sum {
			// The file changed during the read: answer from this read only.
			return r.lsIndexMany(idx, paths)
		}
		known = c.remember(sum, missing, got)
	}
	out := make(map[string]indexEntry, len(paths))
	for _, p := range paths {
		if e := known[p]; e != nil {
			out[p] = *e
		}
	}
	return out, nil
}

// remember keeps the entries that a read found for paths, and the paths that
// it did not find, under the index checksum sum.
func (c *indexReads) remember(sum string, paths []string, found map[string]indexEntry) map[string]*indexEntry {
	if c.bySum == nil {
		c.bySum = map[string]map[string]*indexEntry{}
	}
	known := c.bySum[sum]
	if known == nil {
		known = map[string]*indexEntry{}
		c.bySum[sum] = known
	}
	for _, p := range paths {
		if e, ok := found[p]; ok {
			known[p] = &e
		} else {
			known[p] = nil
		}
	}
	return known
}

// maxIndexWrites is the number of writes that one update-index call takes,
// which keeps its command line short on Windows.
const maxIndexWrites = 100

// indexWrite is one pending change of the index entry at path: the entry
// that replaces it, or its removal.
type indexWrite struct {
	path   string
	entry  indexEntry
	remove bool
}

// indexView reads the entries of the index file idx that records name, and
// keeps the writes of the records that applied. One Git read answers every
// record, and a read of a path with a pending write answers from the write,
// so no record starts Git to read or write the index. flush writes the
// pending writes to the file in record order.
type indexView struct {
	r       *Repo
	idx     string
	entries map[string]indexEntry
	writes  []indexWrite
	last    map[string]int // path to the position in writes of its newest write
}

func (r *Repo) newIndexView(idx string, recs []EditRecord, reads *indexReads) (*indexView, error) {
	v := &indexView{r: r, idx: idx, last: map[string]int{}}
	seen := map[string]bool{}
	var paths []string
	for _, rec := range recs {
		for _, p := range []string{rec.Path, rec.From} {
			if p != "" && !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	var err error
	v.entries, err = reads.read(r, idx, paths)
	return v, err
}

// get returns the stage-0 entry of p, which a record names, as the index
// file holds it after the pending writes.
func (v *indexView) get(p string) (indexEntry, bool) {
	if i, ok := v.last[p]; ok {
		w := v.writes[i]
		return w.entry, !w.remove
	}
	e, ok := v.entries[p]
	return e, ok
}

// removed reports whether the newest pending write of p removes it.
func (v *indexView) removed(p string) bool {
	i, ok := v.last[p]
	return ok && v.writes[i].remove
}

func (v *indexView) set(p, mode, oid string) {
	if mode == "" {
		mode = "100644"
	}
	v.last[p] = len(v.writes)
	v.writes = append(v.writes, indexWrite{path: p, entry: indexEntry{mode, oid}})
}

func (v *indexView) remove(p string) {
	v.last[p] = len(v.writes)
	v.writes = append(v.writes, indexWrite{path: p, remove: true})
}

// flush writes the pending writes to the index file, in groups of
// maxIndexWrites, with one update-index call for each group. Git fails
// without writing the index when a write does not fit, such as a file where
// the index holds a directory, as the call for each record did.
func (v *indexView) flush() error {
	for start := 0; start < len(v.writes); start += maxIndexWrites {
		args := []string{"update-index", "--add"}
		for _, w := range v.writes[start:min(start+maxIndexWrites, len(v.writes))] {
			if w.remove {
				// The ./ keeps a path that starts with - from reading as an option.
				args = append(args, "--force-remove", "./"+w.path)
			} else {
				args = append(args, "--cacheinfo", w.entry.mode+","+w.entry.oid+","+w.path)
			}
		}
		if _, err := v.r.Run(gitx.Opt{Index: v.idx}, args...); err != nil {
			return err
		}
	}
	return nil
}

// applyRecord replays one recorded edit onto the index entry of its path.
func (r *Repo) applyRecord(v *indexView, rec EditRecord, force bool) error {
	if rec.From != "" {
		return r.applyMove(v, rec)
	}
	cur, inIndex := v.get(rec.Path)
	switch {
	case rec.After == "": // deletion
		return applyDeletion(v, rec, cur, inIndex)
	case !inIndex && rec.Before == "": // a new file
		return r.applyNewFile(v, rec, force)
	case !inIndex:
		return errorf(ExitConflict, "edit-conflict", "the file is not in the stage; it was created by another writer")
	}
	if cur.mode == "120000" || cur.mode == "160000" {
		return errorf(ExitConflict, "edit-conflict", "the stage holds a symbolic link or submodule at this path")
	}
	if cur.oid == rec.Before {
		v.set(rec.Path, cur.mode, rec.After)
		return nil
	}
	out, err := r.replayRecord(rec, cur.oid)
	if err != nil {
		return err
	}
	oid, err := r.Line(gitx.Opt{Stdin: bytes.NewReader(out)}, "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	v.set(rec.Path, cur.mode, oid)
	return nil
}

// applyDeletion applies a recorded deletion to the index entry cur.
func applyDeletion(v *indexView, rec EditRecord, cur indexEntry, inIndex bool) error {
	if !inIndex {
		return nil
	}
	if cur.oid != rec.Before {
		return errorf(ExitConflict, "edit-conflict", "the deleted file differs in the stage from what was deleted")
	}
	v.remove(rec.Path)
	return nil
}

// applyMove moves the index entry of rec.From to rec.Path, as `mv` does.
func (r *Repo) applyMove(v *indexView, rec EditRecord) error {
	cur, ok := v.get(rec.From)
	if !ok {
		return errorf(ExitConflict, "edit-conflict", "the moved file %s is not in the stage", rec.From)
	}
	if _, taken := v.get(rec.Path); taken {
		return errorf(ExitConflict, "edit-conflict", "the stage already holds the file that the move creates")
	}
	v.remove(rec.From)
	v.set(rec.Path, cur.mode, cur.oid)
	return nil
}

// applyNewFile stages a file that the recorded edit created.
func (r *Repo) applyNewFile(v *indexView, rec EditRecord, force bool) error {
	if !force {
		// Like `git add`, an ignored file needs --force.
		args := []string{"check-ignore", "-q"}
		if v.removed(rec.Path) {
			// The index file still tracks the path that an earlier record deleted.
			args = append(args, "--no-index")
		}
		_, err := r.Run(gitx.Opt{Index: v.idx, Env: []string{envLiteralPathspecs}}, append(args, "--", rec.Path)...)
		if err == nil {
			return errorf(ExitConflict, "edit-conflict", "the file is ignored by .gitignore; pass -f to stage it")
		}
		if gitx.ExitCode(err) != 1 {
			return err
		}
	}
	v.set(rec.Path, rec.Mode, rec.After)
	return nil
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
