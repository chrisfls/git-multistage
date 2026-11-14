package ms

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/chrisfls/git-multistage/internal/gitx"
	"github.com/chrisfls/git-multistage/internal/hunks"
)

// RepoPath converts a path argument, relative to the current directory or
// absolute, into a slash-separated path relative to the worktree root.
// The worktree root itself is ".".
func (r *Repo) RepoPath(arg string) (string, error) {
	var rel string
	if filepath.IsAbs(arg) {
		p, err := r.relToTop(arg)
		if err != nil {
			return "", usageErr("path %s is outside the worktree %s", arg, r.Top)
		}
		rel = filepath.ToSlash(p)
	} else {
		rel = r.Prefix + filepath.ToSlash(arg)
	}
	rel = path.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", usageErr("path %s is outside the worktree %s", arg, r.Top)
	}
	if rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return "", usageErr("path %s is inside the Git directory", arg)
	}
	return rel, nil
}

// relToTop returns the absolute path arg relative to the worktree root.
func (r *Repo) relToTop(arg string) (string, error) {
	p, err := filepath.Rel(r.Top, arg)
	if err == nil && p != ".." && !strings.HasPrefix(p, ".."+string(filepath.Separator)) {
		return p, nil
	}
	// The argument may reach the worktree through a symbolic link.
	// Resolve its deepest existing directory; the rest may not exist yet.
	dir, rest := filepath.Dir(arg), filepath.Base(arg)
	for {
		if resolved, err2 := filepath.EvalSymlinks(dir); err2 == nil {
			return filepath.Rel(r.Top, filepath.Join(resolved, rest))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return p, err
		}
		dir, rest = parent, filepath.Join(filepath.Base(dir), rest)
	}
}

func (r *Repo) repoPaths(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for _, a := range args {
		p, err := r.RepoPath(a)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// gitErr turns a failed Git command into an *Error that shows Git's message.
func gitErr(err error, code int, kind string) error {
	var ee *gitx.ExitError
	if errors.As(err, &ee) {
		msg := strings.TrimSpace(ee.Stderr)
		msg = strings.TrimPrefix(msg, "fatal: ")
		msg = strings.TrimPrefix(msg, "error: ")
		if msg == "" {
			msg = ee.Error()
		}
		return &Error{Code: code, Kind: kind, Message: msg}
	}
	return err
}

// Change is one path in a name-status listing.
type Change struct {
	Status  string `json:"status"`
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
}

func parseNameStatus(out []byte) []Change {
	f := strings.Split(string(out), "\x00")
	var cs []Change
	for i := 0; i+1 < len(f); i += 2 {
		c := Change{Status: f[i][:1], Path: f[i+1]}
		if (c.Status == "R" || c.Status == "C") && i+2 < len(f) {
			c.OldPath, c.Path = f[i+1], f[i+2]
			i++
		}
		cs = append(cs, c)
	}
	return cs
}

// stagedChanges lists the stage's staged changes relative to its base.
func (l *Stage) stagedChanges() ([]Change, error) {
	base, err := l.r.TreeOf(l.M.Base)
	if err != nil {
		return nil, err
	}
	out, err := l.r.Run(gitx.Opt{Index: l.IndexPath()}, "diff-index", flagCached, "-z", "--name-status", "-M", base)
	if err != nil {
		return nil, err
	}
	return parseNameStatus(out), nil
}

// otherStagesStaged returns, for each path that another stage of this worktree
// has staged, the stages that staged it.
func (l *Stage) otherStagesStaged() (map[string][]string, error) {
	res := map[string][]string{}
	entries, err := os.ReadDir(l.r.stagesDir())
	if err != nil {
		return res, nil //nolint:nilerr // an unreadable stages directory holds no other stages
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != l.ID && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	// Each stage is read by its own Git process, so they run at once.
	staged := make([][]string, len(names))
	reads := make([]func() error, len(names))
	for i, name := range names {
		reads[i] = func() error {
			// A stage that cannot be read has nothing to compare.
			staged[i], _ = l.r.stageStagedPaths(filepath.Join(l.r.stagesDir(), name))
			return nil
		}
	}
	_ = parallel(reads...)
	for i, name := range names {
		for _, p := range staged[i] {
			res[p] = append(res[p], name)
		}
	}
	return res, nil
}

// stageStagedPaths reads another stage's staged paths without locking it or
// writing its index. A stage that replaces its index meanwhile is read again.
func (r *Repo) stageStagedPaths(dir string) ([]string, error) {
	for range 5 {
		m, err := readMeta(dir)
		if err != nil {
			return nil, err
		}
		idx := filepath.Join(dir, m.Index)
		if _, err := os.Stat(idx); err != nil {
			continue
		}
		r.RememberTree(m.Base, m.BaseTree)
		base, err := r.TreeOf(m.Base)
		if err != nil {
			return nil, err
		}
		out, err := r.Run(gitx.Opt{Index: idx}, "diff-index", flagCached, "-z", "--name-only", "--no-renames", base)
		if m2, err2 := readMeta(dir); err2 != nil || m2.Index != m.Index {
			continue
		}
		if err != nil {
			return nil, err
		}
		var paths []string
		for _, p := range strings.Split(string(out), "\x00") {
			if p != "" {
				paths = append(paths, p)
			}
		}
		return paths, nil
	}
	return nil, fmt.Errorf("stage %s keeps changing", filepath.Base(dir))
}

// AddPaths stages whole files, like `git add`.
func (l *Stage) AddPaths(paths []string, force, allowShared bool) ([]gitx.Entry, error) {
	if err := l.canonicalPaths(paths); err != nil {
		return nil, err
	}
	if missing := l.r.missingOnDisk(paths); len(missing) > 0 {
		return nil, errMissingPaths(missing)
	}
	before, err := l.r.WriteTree(l.IndexPath())
	if err != nil {
		return nil, err
	}
	// Stage into a copy that becomes current only after the checks pass, so
	// a refusal or a crash leaves the previous index in place.
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
	args := []string{cmdAdd}
	if force {
		args = append(args, "-f")
	}
	if _, err := l.r.Run(gitx.Opt{Index: idx}, append(append(args, "--"), paths...)...); err != nil {
		return nil, gitErr(err, ExitError, "add-failed")
	}
	fault("after-add")
	after, err := l.r.WriteTree(idx)
	if err != nil {
		return nil, err
	}
	changed, err := l.r.DiffTrees(before, after, false)
	if err != nil {
		return nil, err
	}
	if err := l.checkAdded(changed, allowShared); err != nil {
		return nil, err
	}
	if err := l.adoptIndex(name, gen); err != nil {
		return nil, err
	}
	adopted = true
	return changed, nil
}

// checkAdded refuses the changes that a whole-file add staged when they
// delete a file, or, unless allowShared, touch a file of another stage.
func (l *Stage) checkAdded(changed []gitx.Entry, allowShared bool) error {
	if deleted := deletedEntries(changed); len(deleted) > 0 {
		// A directory argument stages the deletions of the files missing inside it.
		return errMissingPaths(deleted)
	}
	if allowShared || len(changed) == 0 {
		return nil
	}
	return l.refuseShared(changed)
}

// missingOnDisk returns the paths that are missing from the working tree.
func (r *Repo) missingOnDisk(paths []string) []string {
	var missing []string
	for _, p := range paths {
		if _, err := os.Lstat(r.diskPath(p)); errors.Is(err, fs.ErrNotExist) {
			missing = append(missing, p)
		}
	}
	return missing
}

// deletedEntries returns the paths that changed deletes.
func deletedEntries(changed []gitx.Entry) []string {
	var deleted []string
	for _, c := range changed {
		if c.Status == 'D' {
			deleted = append(deleted, c.Path)
		}
	}
	return deleted
}

// errMissingPaths refuses to stage the deletion of paths that are missing
// from the working tree: another agent may have deleted or moved them.
func errMissingPaths(paths []string) error {
	return &Error{
		Code: ExitError, Kind: "missing-path",
		Message: "these paths are missing from the working tree, and add stages no deletions or moves; nothing was staged",
		Paths:   paths,
		Hint:    "delete a file with `git multistage rm <path>`, and move one with `git multistage mv <old> <new>`",
	}
}

// refuseShared fails with shared-path when another stage has staged changes
// in a changed file.
func (l *Stage) refuseShared(changed []gitx.Entry) error {
	others, err := l.otherStagesStaged()
	if err != nil {
		return err
	}
	var shared, details []string
	for _, c := range changed {
		if ids := others[c.Path]; len(ids) > 0 {
			shared = append(shared, c.Path)
			details = append(details, fmt.Sprintf("%s is staged in %s", c.Path, strings.Join(ids, ", ")))
		}
	}
	if len(shared) == 0 {
		return nil
	}
	return &Error{
		Code: ExitRefused, Kind: "shared-path",
		Message: "another stage has staged changes in these files, so the whole file may hold its changes too; nothing was staged",
		Paths:   shared, Details: details,
		Hint: "stage only your hunks: `git multistage hunks <path>`, then `git multistage add <path> --hunk <id>`; " +
			"or pass --allow-shared to stage the whole file anyway",
	}
}

// FileHunks is the hunk listing of one file.
type FileHunks struct {
	Path   string     `json:"path"`
	Status string     `json:"status"` // modified, added, deleted
	Binary bool       `json:"binary,omitempty"`
	Note   string     `json:"note,omitempty"` // why the file has no selectable hunks
	Hunks  []HunkInfo `json:"hunks"`

	mode    string // index mode to stage with
	oldBlob string
}

// HunkInfo is one selectable hunk.
type HunkInfo struct {
	hunks.Hunk
	Lines    []string `json:"lines"`
	StagedBy []string `json:"staged_by,omitempty"`
}

type indexEntry struct{ mode, oid string }

// lsIndex returns the stage-0 index entry of path in idx.
func (r *Repo) lsIndex(idx, p string) (indexEntry, bool, error) {
	out, err := r.Run(gitx.Opt{Index: idx}, "ls-files", "-s", "-z", "--", p)
	if err != nil {
		return indexEntry{}, false, err
	}
	for _, rec := range strings.Split(string(out), "\x00") {
		meta, name, ok := strings.Cut(rec, "\t")
		f := strings.Fields(meta)
		if ok && name == p && len(f) == 3 && f[2] == "0" {
			return indexEntry{f[0], f[1]}, true, nil
		}
	}
	return indexEntry{}, false, nil
}

// lsTree returns the entry of path in commit.
func (r *Repo) lsTree(commit, p string) (indexEntry, bool, error) {
	if commit == "" {
		return indexEntry{}, false, nil
	}
	out, err := r.Run(gitx.Opt{}, "ls-tree", "-z", "--full-tree", commit, "--", p)
	if err != nil {
		return indexEntry{}, false, err
	}
	meta, name, ok := strings.Cut(strings.TrimSuffix(string(out), "\x00"), "\t")
	f := strings.Fields(meta)
	if ok && name == p && len(f) == 3 && f[1] == "blob" {
		return indexEntry{f[0], f[2]}, true, nil
	}
	return indexEntry{}, false, nil
}

// lsIndexMany returns the stage-0 index entries of paths in idx, by path,
// with one Git call for each 200 paths. A path that has no stage-0 entry is
// not in the result.
func (r *Repo) lsIndexMany(idx string, paths []string) (map[string]indexEntry, error) {
	current, _, err := r.readIndexEntries(idx, paths)
	if err != nil {
		return nil, err
	}
	entries := make(map[string]indexEntry, len(current))
	for p, e := range current {
		entries[p] = indexEntry(e)
	}
	return entries, nil
}

// lsTreeMany returns the blob entries of paths in commit, by path, with one
// Git call for each 200 paths. A path that is not a blob there is not in the
// result.
func (r *Repo) lsTreeMany(commit string, paths []string) (map[string]indexEntry, error) {
	entries := map[string]indexEntry{}
	if commit == "" {
		return entries, nil
	}
	for start := 0; start < len(paths); start += 200 {
		end := min(start+200, len(paths))
		out, err := r.Run(gitx.Opt{}, append([]string{"ls-tree", "-z", "--full-tree", commit, "--"}, paths[start:end]...)...)
		if err != nil {
			return nil, err
		}
		for _, rec := range strings.Split(string(out), "\x00") {
			meta, name, ok := strings.Cut(rec, "\t")
			f := strings.Fields(meta)
			if ok && len(f) == 3 && f[1] == "blob" {
				entries[name] = indexEntry{f[0], f[2]}
			}
		}
	}
	return entries, nil
}

// diffBlobs returns the hunks between two blobs of p. Their IDs are for
// internal use: identical copies get IDs that hash no versions.
func (r *Repo) diffBlobs(p, oldBlob, newBlob string) ([]hunks.Hunk, error) {
	return r.diffBlobsWithIDs(p, oldBlob, newBlob, "", hunks.NewSide)
}

// diffBlobsWithIDs returns the hunks between two blobs of p. The IDs of
// identical copies hash versions and their start lines on side; see
// hunks.Parse.
func (r *Repo) diffBlobsWithIDs(p, oldBlob, newBlob, versions string, side hunks.Side) ([]hunks.Hunk, error) {
	out, err := r.Run(gitx.Opt{}, cmdDiff, "--no-color", "--no-ext-diff", "--no-textconv",
		"-U0", "--inter-hunk-context=0", "--diff-algorithm=myers", oldBlob, newBlob)
	if err != nil && gitx.ExitCode(err) != 1 {
		return nil, err
	}
	return hunks.Parse(p, out, versions, side)
}

// copyVersions names the versions that the IDs of identical hunks depend on:
// the stage's base commit and the blob whose lines place the copies. An
// unstaged listing places copies in the working-tree snapshot, which staging
// a hunk leaves alone. A staged listing places them in the base blob, which
// restoring a hunk leaves alone. So the other copies keep their IDs; any
// other change to the placing blob, or a new base, makes them stale.
func (l *Stage) copyVersions(blob string) string {
	return l.M.Base + "\x00" + blob
}

func (r *Repo) catBlob(oid string) ([]byte, error) {
	return r.Run(gitx.Opt{}, "cat-file", "blob", oid)
}

// catBlobs returns the content of the blobs oids, in order, with one Git
// call. It fails when an object is missing or is not a blob.
func (r *Repo) catBlobs(oids ...string) ([][]byte, error) {
	out, err := r.Run(gitx.Opt{Stdin: strings.NewReader(strings.Join(oids, "\n") + "\n")}, "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	blobs := make([][]byte, 0, len(oids))
	for _, oid := range oids {
		// <oid> SP <type> SP <size> LF <content> LF
		header, rest, ok := bytes.Cut(out, []byte("\n"))
		f := strings.Fields(string(header))
		if !ok || len(f) != 3 || f[0] != oid || f[1] != "blob" {
			return nil, fmt.Errorf("git cat-file --batch: no blob %s: %s", oid, header)
		}
		size, err := strconv.Atoi(f[2])
		if err != nil || size+1 > len(rest) {
			return nil, fmt.Errorf("git cat-file --batch: bad header %q", header)
		}
		blobs = append(blobs, rest[:size])
		out = rest[size+1:]
	}
	return blobs, nil
}

// hasFilter reports whether a clean/smudge filter (such as Git LFS) applies to p.
func (r *Repo) hasFilter(p string) (string, error) {
	out, err := r.Run(gitx.Opt{}, "check-attr", "-z", "filter", "--", p)
	if err != nil {
		return "", err
	}
	f := strings.Split(string(out), "\x00")
	if len(f) >= 3 && f[2] != "unspecified" && f[2] != "unset" {
		return f[2], nil
	}
	return "", nil
}

// canonicalPaths replaces each path with its spelling in the stage index.
// One Git call finds the paths that the index holds as spelled.
func (l *Stage) canonicalPaths(paths []string) error {
	held, err := l.r.lsIndexMany(l.IndexPath(), paths)
	if err != nil {
		return err
	}
	for i, p := range paths {
		if _, ok := held[p]; ok {
			continue
		}
		if paths[i], err = l.canonicalUnheld(p); err != nil {
			return err
		}
	}
	return nil
}

// canonical returns the spelling of p in the stage index. On a
// case-insensitive file system a path argument may differ in case from the
// tracked path.
func (l *Stage) canonical(p string) (string, error) {
	if _, ok, err := l.r.lsIndex(l.IndexPath(), p); err != nil || ok {
		return p, err
	}
	return l.canonicalUnheld(p)
}

// canonicalUnheld returns the spelling in the stage index of a path that the
// index does not hold as spelled.
func (l *Stage) canonicalUnheld(p string) (string, error) {
	if !l.r.ConfigBool("core.ignoreCase", false) {
		return p, nil
	}
	dir := path.Dir(p)
	out, err := l.r.Run(gitx.Opt{Index: l.IndexPath(), Env: []string{envLiteralPathspecs, "GIT_ICASE_PATHSPECS=1"}},
		"ls-files", "-z", "--", dir)
	if err != nil {
		return p, err
	}
	for _, name := range strings.Split(string(out), "\x00") {
		if strings.EqualFold(name, p) {
			return name, nil
		}
	}
	return p, nil
}

// unstagedHunks lists the hunks between the stage index and the working tree.
func (l *Stage) unstagedHunks(p string) (FileHunks, error) {
	fh := FileHunks{Path: p, Status: "modified", Hunks: []HunkInfo{}}
	old, tracked, err := l.r.lsIndex(l.IndexPath(), p)
	if err != nil {
		return fh, err
	}
	info, err := os.Lstat(filepath.Join(l.r.Top, filepath.FromSlash(p)))
	if errors.Is(err, fs.ErrNotExist) {
		if !tracked {
			return fh, usageErr("path %s is neither in the working tree nor in the stage's index", p)
		}
		fh.Status, fh.Note = "deleted", "stage the deletion with `git multistage rm "+p+"`"
		return fh, nil
	}
	if err != nil {
		return fh, err
	}
	if info.IsDir() {
		return fh, usageErr("path %s is a directory; pass files", p)
	}
	fh.mode, fh.oldBlob = old.mode, old.oid
	if !tracked {
		fh.Status, fh.mode = "added", l.r.newFileMode(info)
		if fh.oldBlob, err = l.r.EnsureEmptyBlob(); err != nil {
			return fh, err
		}
	}
	if note, err := l.r.wholeFileNote(p, info, fh.mode); err != nil || note != "" {
		fh.Note = note
		return fh, err
	}
	// One snapshot of the file: every hunk comes from this blob, so a
	// concurrent write to the file cannot mix two versions.
	newBlob, err := l.r.snapshotBlob(l.IndexPath(), p)
	if err != nil {
		return fh, err
	}
	hs, err := l.r.diffBlobsWithIDs(p, fh.oldBlob, newBlob, l.copyVersions(newBlob), hunks.NewSide)
	return fh, fh.fill(hs, err, "binary file; stage it whole with `git multistage add "+p+"`")
}

// newFileMode returns the index mode that `git add` gives the new file info.
func (r *Repo) newFileMode(info fs.FileInfo) string {
	if info.Mode()&0o111 != 0 && r.ConfigBool("core.fileMode", true) {
		return "100755"
	}
	return "100644"
}

// wholeFileNote returns why the file p, with the index mode mode, has no
// hunks and must be staged whole, or "".
func (r *Repo) wholeFileNote(p string, info fs.FileInfo, mode string) (string, error) {
	if !info.Mode().IsRegular() || mode == "120000" || mode == "160000" {
		return "not a regular file; stage it whole with `git multistage add " + p + "`", nil
	}
	f, err := r.hasFilter(p)
	if err != nil || f == "" {
		return "", err
	}
	return fmt.Sprintf("uses the %q filter; stage it whole with `git multistage add %s`", f, p), nil
}

// fill adds the hunks of a diff to fh. A binary file gets binaryNote and
// no hunks.
func (fh *FileHunks) fill(hs []hunks.Hunk, err error, binaryNote string) error {
	if errors.Is(err, hunks.ErrBinary) {
		fh.Binary, fh.Note = true, binaryNote
		return nil
	}
	if err != nil {
		return err
	}
	for _, h := range hs {
		fh.Hunks = append(fh.Hunks, HunkInfo{Hunk: h, Lines: h.Lines()})
	}
	return nil
}

// stagedHunks lists the hunks between the stage's base and its index.
func (l *Stage) stagedHunks(p string) (FileHunks, error) {
	fh := FileHunks{Path: p, Status: "modified", Hunks: []HunkInfo{}}
	base, inBase, err := l.r.lsTree(l.M.Base, p)
	if err != nil {
		return fh, err
	}
	cur, inIndex, err := l.r.lsIndex(l.IndexPath(), p)
	if err != nil {
		return fh, err
	}
	switch {
	case !inBase && !inIndex:
		return fh, usageErr("path %s has no staged changes", p)
	case !inIndex:
		fh.Status, fh.Note = "deleted", "unstage the deletion with `git multistage restore --staged "+p+"`"
		return fh, nil
	case !inBase:
		fh.Status = "added"
		if base.oid, err = l.r.EnsureEmptyBlob(); err != nil {
			return fh, err
		}
	}
	fh.mode, fh.oldBlob = cur.mode, base.oid
	if inBase && base.mode != cur.mode || cur.mode == "120000" || cur.mode == "160000" {
		fh.Note = "mode or type change; unstage it whole with `git multistage restore --staged " + p + "`"
		return fh, nil
	}
	hs, err := l.r.diffBlobsWithIDs(p, base.oid, cur.oid, l.copyVersions(base.oid), hunks.OldSide)
	return fh, fh.fill(hs, err, "binary file; unstage it whole with `git multistage restore --staged "+p+"`")
}

// Hunks lists hunks of the given paths. With no paths it lists every file
// that has changes: unstaged tracked files, or staged files when cached.
func (l *Stage) Hunks(paths []string, cached bool) ([]FileHunks, error) {
	if len(paths) == 0 {
		var err error
		if paths, err = l.changedPaths(cached); err != nil {
			return nil, err
		}
	}
	var others map[string][]string
	if !cached {
		var err error
		if others, err = l.otherStagesStaged(); err != nil {
			return nil, err
		}
	}
	var res []FileHunks
	for _, p := range paths {
		p, err := l.canonical(p)
		if err != nil {
			return nil, err
		}
		var fh FileHunks
		if cached {
			fh, err = l.stagedHunks(p)
		} else {
			fh, err = l.unstagedHunks(p)
		}
		if err != nil {
			return nil, err
		}
		if len(others[p]) > 0 {
			l.markStagedBy(&fh, others[p])
		}
		res = append(res, fh)
	}
	return res, nil
}

// markStagedBy notes which other stages have staged a hunk identical to each
// listed hunk. The ID of a hunk without copies depends only on the path and
// the changed lines, so an equal ID means an equal change. The ID of a copy
// also hashes versions of its own listing, so a copy matches by its changed
// lines and its start line in the old content instead.
func (l *Stage) markStagedBy(fh *FileHunks, ids []string) {
	for _, id := range ids {
		dir := filepath.Join(l.r.stagesDir(), id)
		m, err := readMeta(dir)
		if err != nil {
			continue
		}
		other := &Stage{r: l.r, ID: id, Dir: dir, M: m}
		staged, err := other.stagedHunks(fh.Path)
		if err != nil {
			continue
		}
		ids, places := map[string]bool{}, map[string]bool{}
		for _, h := range staged.Hunks {
			ids[h.ID] = true
			places[placeKey(h.Hunk)] = true
		}
		for i := range fh.Hunks {
			if ids[fh.Hunks[i].ID] || places[placeKey(fh.Hunks[i].Hunk)] {
				fh.Hunks[i].StagedBy = append(fh.Hunks[i].StagedBy, id)
			}
		}
	}
}

// placeKey names a hunk by its start line in the old content and its
// changed lines.
func placeKey(h hunks.Hunk) string {
	return strconv.Itoa(h.OldStart) + "\x00" + strings.Join(h.Lines(), "\n")
}

// changedPaths lists tracked files that differ between the stage index and
// the working tree, or, when cached, between the base and the stage index.
func (l *Stage) changedPaths(cached bool) ([]string, error) {
	var out []byte
	var err error
	if cached {
		base, err2 := l.r.TreeOf(l.M.Base)
		if err2 != nil {
			return nil, err2
		}
		out, err = l.r.Run(gitx.Opt{Index: l.IndexPath()}, "diff-index", flagCached, "-z", "--name-only", "--no-renames", base)
	} else {
		out, err = l.r.Run(gitx.Opt{Index: l.IndexPath()}, cmdDiff, "-z", "--name-only", "--no-renames")
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// selectHunks finds each requested ID in the listing. Every ID must match
// exactly one hunk of the listed files.
func selectHunks(files []FileHunks, ids []string) (map[string][]hunks.Hunk, error) {
	seen, err := checkHunkIDs(files, ids)
	if err != nil {
		return nil, err
	}
	chosen := map[string][]hunks.Hunk{}
	for _, f := range files {
		for _, h := range f.Hunks {
			if seen[h.ID] || seen[h.CopyID] {
				chosen[f.Path] = append(chosen[f.Path], h.Hunk)
			}
		}
	}
	return chosen, nil
}

// checkHunkIDs returns the set of ids. Each ID must match exactly one hunk
// of the listed files: its ID, or its copy ID when its other copies were
// staged since the listing.
func checkHunkIDs(files []FileHunks, ids []string) (map[string]bool, error) {
	byID := map[string][]string{}
	for _, f := range files {
		for _, h := range f.Hunks {
			byID[h.ID] = append(byID[h.ID], f.Path)
			if h.CopyID != h.ID {
				byID[h.CopyID] = append(byID[h.CopyID], f.Path)
			}
		}
	}
	seen := map[string]bool{}
	var unknown, ambiguous []string
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		switch len(byID[id]) {
		case 0:
			unknown = append(unknown, id)
		case 1:
		default:
			ambiguous = append(ambiguous, id)
		}
	}
	if len(unknown) == 0 && len(ambiguous) == 0 {
		return seen, nil
	}
	e := &Error{
		Code: ExitStale, Kind: "stale-hunk",
		Message: "hunk IDs do not match the current diff; nothing was changed",
		Hint:    "the file changed since the hunks were listed; run `git multistage hunks <path>` again",
	}
	for _, id := range unknown {
		e.Details = append(e.Details, "unknown hunk "+id)
	}
	for _, id := range ambiguous {
		e.Details = append(e.Details, "hunk "+id+" matches several files; pass one path")
	}
	return nil, e
}

// refuseIgnored fails, like `git add`, for an untracked path that a
// .gitignore file ignores.
func (l *Stage) refuseIgnored(p string) error {
	// check-ignore takes plain paths and rejects literal pathspec mode.
	_, err := l.r.Run(gitx.Opt{Index: l.IndexPath(), Env: []string{envLiteralPathspecs}}, "check-ignore", "-q", "--", p)
	if err != nil && gitx.ExitCode(err) != 1 {
		return err
	}
	if err == nil {
		return &Error{
			Code: ExitError, Kind: "ignored",
			Message: "the path is ignored by a .gitignore file; nothing was staged", Paths: []string{p},
			Hint: "pass -f to stage it anyway",
		}
	}
	return nil
}

// AddHunks stages the chosen hunks of the given files. Every check runs
// before the stage index changes, and all files change in one new index
// generation, so a refusal stages nothing.
func (l *Stage) AddHunks(paths, ids []string, force bool) ([]string, error) {
	files, err := l.Hunks(paths, false)
	if err != nil {
		return nil, err
	}
	chosen, err := selectHunks(files, ids)
	if err != nil {
		return nil, err
	}
	var writes []blobWrite
	for _, f := range files {
		hs := chosen[f.Path]
		if len(hs) == 0 {
			continue
		}
		if f.Status == "added" && !force {
			if err := l.refuseIgnored(f.Path); err != nil {
				return nil, err
			}
		}
		old, err := l.r.catBlob(f.oldBlob)
		if err != nil {
			return nil, err
		}
		content, err := hunks.Apply(old, hs)
		if err != nil {
			return nil, err
		}
		writes = append(writes, blobWrite{path: f.Path, mode: f.mode, content: content})
	}
	if err := l.writeBlobs(writes); err != nil {
		return nil, err
	}
	staged := make([]string, 0, len(writes))
	for _, w := range writes {
		staged = append(staged, w.path)
	}
	return staged, nil
}

// blobWrite is one change of a stage index entry: new content for path, or,
// when restore is set, the entry of the base.
type blobWrite struct {
	path, mode string
	content    []byte
	restore    bool
}

// writeBlobs applies writes to a copy of the stage index and adopts the copy,
// so that the stage index changes once, or not at all.
func (l *Stage) writeBlobs(writes []blobWrite) error {
	if len(writes) == 0 {
		return nil
	}
	name, gen := l.newIndexFile()
	idx := filepath.Join(l.Dir, name)
	if err := copyFile(l.IndexPath(), idx); err != nil {
		return err
	}
	adopted := false
	defer func() {
		if !adopted {
			_ = os.Remove(idx)
		}
	}()
	var restore []string
	for _, w := range writes {
		if w.restore {
			restore = append(restore, w.path)
			continue
		}
		if err := l.setBlob(idx, w.path, w.mode, w.content); err != nil {
			return err
		}
	}
	if len(restore) > 0 {
		if err := l.restorePaths(idx, restore); err != nil {
			return err
		}
	}
	if err := l.adoptIndex(name, gen); err != nil {
		return err
	}
	adopted = true
	return nil
}

// setBlob stores content as the entry of path in the index file idx.
func (l *Stage) setBlob(idx, p, mode string, content []byte) error {
	oid, err := l.r.Line(gitx.Opt{Stdin: bytes.NewReader(content)}, "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	_, err = l.r.Run(gitx.Opt{Index: idx}, "update-index", "--add", "--cacheinfo", mode, oid, p)
	return err
}

// RestorePaths unstages whole files: their stage index entries return to the
// base, and the staged moves of those files are forgotten.
func (l *Stage) RestorePaths(paths []string) error {
	if err := l.restorePaths(l.IndexPath(), paths); err != nil {
		return err
	}
	if moves, dropped := dropMoves(l.M.Moves, paths); len(dropped) > 0 {
		l.M.Moves = moves
		return l.save()
	}
	return nil
}

// restorePaths returns the entries of paths in the index file idx to the base.
func (l *Stage) restorePaths(idx string, paths []string) error {
	var err error
	if l.M.Base == "" {
		_, err = l.r.Run(gitx.Opt{Index: idx}, append([]string{"rm", flagCached, "-r", "-q", "--ignore-unmatch", "--"}, paths...)...)
	} else {
		_, err = l.r.Run(gitx.Opt{Index: idx}, append([]string{"reset", "-q", l.M.Base, "--"}, paths...)...)
	}
	if err != nil {
		return gitErr(err, ExitError, "restore-failed")
	}
	return nil
}

// RestoreHunks unstages the chosen staged hunks: the stage index keeps every
// other staged hunk of the file. All files change in one new index
// generation.
func (l *Stage) RestoreHunks(paths, ids []string) ([]string, error) {
	files, err := l.Hunks(paths, true)
	if err != nil {
		return nil, err
	}
	chosen, err := selectHunks(files, ids)
	if err != nil {
		return nil, err
	}
	var writes []blobWrite
	for _, f := range files {
		drop := map[string]bool{}
		for _, h := range chosen[f.Path] {
			drop[h.ID] = true
		}
		if len(drop) == 0 {
			continue
		}
		var keep []hunks.Hunk
		for _, h := range f.Hunks {
			if !drop[h.ID] {
				keep = append(keep, h.Hunk)
			}
		}
		if len(keep) == 0 {
			writes = append(writes, blobWrite{path: f.Path, restore: true})
			continue
		}
		old, err := l.r.catBlob(f.oldBlob)
		if err != nil {
			return nil, err
		}
		content, err := hunks.Apply(old, keep)
		if err != nil {
			return nil, err
		}
		writes = append(writes, blobWrite{path: f.Path, mode: f.mode, content: content})
	}
	if err := l.writeBlobs(writes); err != nil {
		return nil, err
	}
	restored := make([]string, 0, len(writes))
	for _, w := range writes {
		restored = append(restored, w.path)
	}
	sort.Strings(restored)
	return restored, nil
}

// ApplyPatch applies a patch to the stage index only, like `git apply --cached`.
func (l *Stage) ApplyPatch(patch io.Reader, reverse bool) error {
	args := []string{"apply", flagCached, "--whitespace=nowarn"}
	if reverse {
		args = append(args, "-R")
	}
	_, err := l.r.Run(gitx.Opt{Index: l.IndexPath(), Stdin: patch}, append(args, "-")...)
	if err != nil {
		return gitErr(err, ExitConflict, "patch-failed")
	}
	return nil
}
