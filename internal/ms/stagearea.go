package ms

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/chrisfls/git-multistage/internal/filelock"
	"github.com/chrisfls/git-multistage/internal/gitx"
)

// Repo is a worktree plus the git-multistage storage inside its Git directory.
type Repo struct {
	*gitx.Repo
	Dir string // <per-worktree git dir>/multistage
}

// OpenRepo discovers the repository that contains dir.
func OpenRepo(dir string) (*Repo, error) {
	g, err := gitx.Open(dir, cacheDirName)
	if err != nil {
		return nil, err
	}
	msDir := filepath.Join(g.GitDir, cacheDirName)
	g.TreeCache = filepath.Join(msDir, "index-trees")
	return &Repo{Repo: g, Dir: msDir}, nil
}

// cacheDirName is the directory of git-multistage's files in the Git
// directory.
const cacheDirName = "multistage"

func (r *Repo) stagesDir() string { return filepath.Join(r.Dir, "stages") }

var (
	idRE       = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	reservedRE = regexp.MustCompile(`^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$`)
)

// ValidateStageID checks that id is usable as a directory name on every
// platform, including case-insensitive file systems, and as a ref name.
func ValidateStageID(id string) error {
	if id == "" {
		return &Error{
			Code: ExitUsage, Kind: "no-stage-id",
			Message: "no stage ID: pass --stage <id>, set GIT_MULTISTAGE_ID, or pass --agentic inside an agent",
			Hint:    "run `git multistage init` to create a stage with a new unique ID",
		}
	}
	if !idRE.MatchString(id) || strings.HasSuffix(id, ".") || strings.HasSuffix(id, ".lock") ||
		strings.Contains(id, "..") || reservedRE.MatchString(id) {
		return usageErr("invalid stage ID %q: use 1-64 lowercase letters, digits, '.', '_' or '-', starting with a letter or digit", id)
	}
	return nil
}

// sanitizeID turns an external identifier, such as an agent's session ID,
// into a valid stage ID.
func sanitizeID(s string) string {
	b := []byte(strings.ToLower(s))
	for i, c := range b {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			b[i] = '-'
		}
	}
	id := strings.Trim(string(b), "-_")
	if len(id) > 64 || id == "" {
		sum := sha256.Sum256([]byte(s))
		id = strings.TrimRight(id[:min(len(id), 51)], "-_") + "-" + hex.EncodeToString(sum[:])[:12]
		id = strings.TrimLeft(id, "-_")
	}
	return id
}

// NewStageID returns a random stage ID.
func NewStageID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "stage-" + hex.EncodeToString(b)
}

// Meta is the persistent state of one stage.
type Meta struct {
	Version int       `json:"version"`
	ID      string    `json:"stage"`
	Branch  string    `json:"branch"` // full ref name that commits go to
	Base    string    `json:"base"`   // commit the staged changes are relative to; "" for an unborn branch
	Index   string    `json:"index"`  // current index file name inside the stage directory
	Gen     int       `json:"gen"`
	Created time.Time `json:"created"`
	Pending *Pending  `json:"pending,omitempty"`
	Commits []string  `json:"commits,omitempty"` // commits made by this stage, oldest first
	// AppliedEdits names the edit records that the current index already
	// holds; they are deleted when the stage is next opened.
	AppliedEdits []string `json:"applied_edits,omitempty"`
	// Moves are the moves that the index stages, made with `mv`, oldest first.
	Moves []Move `json:"moves,omitempty"`
	// PendingDisk are the disk changes of the rm and mv commands whose
	// stage change is adopted, one per command; a rerun of the same command
	// finishes its change while the stage's index still shows it.
	PendingDisk []DiskChange `json:"pending_disk,omitempty"`
	// BaseTree is the tree of Base, when the process that saved the
	// metadata knew it.
	BaseTree string `json:"base_tree,omitempty"`
}

// DiskChange is the working-tree change of one rm or mv.
type DiskChange struct {
	Command string   `json:"command"`         // "rm" or "mv"
	Paths   []string `json:"paths"`           // rm: the paths to delete; mv: the old and the new path
	Blobs   []string `json:"blobs"`           // the raw blob (hash-object --no-filters) expected on disk at each path; "" for none
	Temp    string   `json:"temp,omitempty"`  // mv: the temporary name of a case-only move between its two renames
	Moves   []Move   `json:"moves,omitempty"` // rm: the stage's moves that the rm dropped, which the records it forgets follow
}

// Move is one staged move of a file.
type Move struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// Pending records a commit whose branch update may or may not have happened.
type Pending struct {
	Commit string `json:"commit"`
	Parent string `json:"parent"`
}

// Stage is one staging area, locked by this process.
type Stage struct {
	r        *Repo
	ID       string
	Dir      string
	M        Meta
	lock     *filelock.Lock
	Created  bool
	Damaged  bool // the current index file is missing
	Warnings []string
}

const stageLockTimeout = 60 * time.Second

func (l *Stage) warn(format string, args ...any) {
	l.Warnings = append(l.Warnings, fmt.Sprintf(format, args...))
}

// IndexPath returns the path of the stage's current index file.
func (l *Stage) IndexPath() string { return filepath.Join(l.Dir, l.M.Index) }

// OpenStage locks the stage id, creating it when it does not exist, and
// finishes any interrupted commit. When exclusive is set, an existing stage is
// an error.
func (r *Repo) OpenStage(id string, exclusive bool) (*Stage, error) {
	return r.openStage(id, exclusive, testLockTimeout(stageLockTimeout))
}

func (r *Repo) openStage(id string, exclusive bool, timeout time.Duration) (*Stage, error) {
	if err := ValidateStageID(id); err != nil {
		return nil, err
	}
	dir := filepath.Join(r.stagesDir(), id)
	if err := r.makeStageDir(dir, id, exclusive); err != nil {
		return nil, err
	}
	testPause("before-stage-lock")
	lk, err := lockStageDir(dir, id, timeout)
	if err != nil {
		return nil, err
	}
	testPause("after-stage-lock")
	l := &Stage{r: r, ID: id, Dir: dir, lock: lk}
	if err := l.loadMeta(); err != nil {
		lk.Release()
		return nil, err
	}
	l.removeLeftovers()
	if err := l.purgeAppliedEdits(); err != nil {
		// The metadata keeps the records' names, so they are never applied
		// again; a later command deletes them.
		l.warn("cannot delete the staged edit records yet; the next command deletes them: %v", err)
	}
	if _, err := os.Stat(l.IndexPath()); err != nil {
		l.Damaged = true
		return l, nil //nolint:nilerr // a stage without its index is returned as damaged, for drop --force
	}
	if err := l.leaveLockedIndex(); err != nil {
		lk.Release()
		return nil, err
	}
	if err := l.recover(); err != nil {
		lk.Release()
		return nil, err
	}
	return l, nil
}

// makeStageDir creates the directory of stage id. When exclusive, the stage
// must not exist yet.
func (r *Repo) makeStageDir(dir, id string, exclusive bool) error {
	if err := os.MkdirAll(r.stagesDir(), 0o777); err != nil {
		return err
	}
	if !exclusive {
		return os.MkdirAll(dir, 0o777)
	}
	err := os.Mkdir(dir, 0o777)
	if errors.Is(err, fs.ErrExist) {
		return &Error{
			Code: ExitRefused, Kind: "stage-exists",
			Message: fmt.Sprintf("stage %q already exists", id),
			Hint:    "if this stage is yours, keep using it; otherwise choose another ID",
		}
	}
	return err
}

// lockStageDir takes the lock of the stage directory dir.
func lockStageDir(dir, id string, timeout time.Duration) (*filelock.Lock, error) {
	lk, err := filelock.Acquire(filepath.Join(dir, "lock"), timeout)
	// A drop that renamed the stage directory away while this process waited
	// leaves no lock file to lock; the stage is then created again.
	for i := 0; i < 5 && errors.Is(err, fs.ErrNotExist); i++ {
		testPause("recreate-dropped-stage")
		if err = os.MkdirAll(dir, 0o777); err == nil {
			lk, err = filelock.Acquire(filepath.Join(dir, "lock"), timeout)
		}
	}
	if errors.Is(err, filelock.ErrBusy) {
		e := errorf(ExitBusy, "busy", "stage %q is busy: another git-multistage process is using it", id)
		e.Hint = forAgent("", hintRetry)
		return nil, e
	}
	return lk, err
}

// loadMeta reads the stage's metadata, or creates the stage when it has none.
func (l *Stage) loadMeta() error {
	data, err := os.ReadFile(filepath.Join(l.Dir, "meta.json"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return l.create()
	case err != nil:
		return err
	}
	if err := json.Unmarshal(data, &l.M); err != nil {
		return fmt.Errorf("stage %q has unreadable metadata %s: %w", l.ID, filepath.Join(l.Dir, "meta.json"), err)
	}
	l.r.RememberTree(l.M.Base, l.M.BaseTree)
	return nil
}

// Close protects the staged objects from garbage collection and releases
// the stage lock.
func (l *Stage) Close() {
	if l.lock == nil {
		return
	}
	if err := l.protect(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot protect the staged objects of %s from garbage collection: %v\n", l.ID, err)
	}
	l.lock.Release()
	l.lock = nil
}

// errDamaged reports a stage whose index file is missing.
func (l *Stage) errDamaged() error {
	return &Error{
		Code: ExitError, Kind: "damaged-stage",
		Message: fmt.Sprintf("stage %s is damaged: its index file %s is missing", l.ID, l.M.Index),
		Hint:    "run `git multistage drop --force`; " + l.keepRef() + " holds the last protected staged tree, if any",
	}
}

func (l *Stage) create() error {
	// A stage targets the branch that HEAD points to, or a detached HEAD itself.
	branch, err := l.r.headTarget()
	if err != nil {
		return err
	}
	base, err := l.r.ResolveRef(branch)
	if err != nil {
		return err
	}
	l.M = Meta{Version: 1, ID: l.ID, Branch: branch, Created: time.Now().UTC()}
	if err := l.setIndex(base); err != nil {
		return err
	}
	l.Created = true
	return l.r.ensureSyncRecord(base)
}

// setIndex makes a fresh index generation that holds the tree of base and
// records base as the stage's base.
func (l *Stage) setIndex(base string) error {
	tree, err := l.r.TreeOf(base)
	if err != nil {
		return err
	}
	// An index that holds the base stages no moves.
	l.M.Moves = nil
	return l.setIndexTree(tree, base)
}

// setIndexTree makes a fresh index generation that holds tree, with base as
// the stage's base. The metadata rename is the commit point: until it
// happens, the old index generation stays current.
func (l *Stage) setIndexTree(tree, base string) error {
	name, gen := l.newIndexFile()
	path := filepath.Join(l.Dir, name)
	if l.M.Index != "" && l.r.KnownIndexTree(l.IndexPath()) == tree {
		// The current index holds tree already, as after a commit whose tree
		// is the staged tree: a copy needs only the refresh.
		if err := copyFile(l.IndexPath(), path); err != nil {
			return err
		}
		if err := l.r.RefreshIndex(path); err != nil {
			return err
		}
	} else if err := l.r.ReadTree(path, tree); err != nil {
		return err
	}
	if err := l.r.copySkipWorktree(path, nil, false); err != nil {
		return err
	}
	l.r.RememberIndexTree(path, tree)
	l.M.Base = base
	return l.adoptIndex(name, gen)
}

// newIndexFile returns the name and generation of the next index file. It
// skips every generation whose index file or lock file exists: a Git child
// of a killed process may still hold such a lock file and later rename it,
// so the name is never reused while the file is there.
func (l *Stage) newIndexFile() (string, int) {
	gen := l.M.Gen + 1
	for {
		name := fmt.Sprintf("index-%d", gen)
		if !exists(filepath.Join(l.Dir, name)) && !exists(filepath.Join(l.Dir, name+".lock")) {
			return name, gen
		}
		gen++
	}
}

// leaveLockedIndex moves the stage to a copy of its index in a new
// generation when the current index has a lock file. Git commands lock the
// current index in place, so the lock file of a Git child that outlived a
// killed process would make them fail until removeLeftovers deletes it. The
// orphan's late rename then replaces a file that is no longer current.
func (l *Stage) leaveLockedIndex() error {
	if !exists(l.IndexPath() + ".lock") {
		return nil
	}
	name, gen := l.newIndexFile()
	if err := copyFile(l.IndexPath(), filepath.Join(l.Dir, name)); err != nil {
		return err
	}
	return l.adoptIndex(name, gen)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// adoptIndex makes the index file name current. The metadata rename is the
// commit point; the previous index file is deleted afterwards.
func (l *Stage) adoptIndex(name string, gen int) error {
	old := l.M.Index
	l.M.Index, l.M.Gen = name, gen
	if err := l.save(); err != nil {
		return err
	}
	if old != "" && old != name {
		_ = os.Remove(filepath.Join(l.Dir, old))
	}
	return nil
}

// keepRef is the per-worktree ref that keeps a stage's staged objects
// reachable, so that `git gc` does not prune them.
func (l *Stage) keepRef() string { return "refs/worktree/multistage/" + l.ID }

// protect points keepRef at a commit whose tree holds the staged tree and
// the blobs of recorded edits, while the stage has either; otherwise it
// deletes the ref.
// sparseState returns, from the default index, the paths that a sparse
// checkout leaves out of the working tree and the set of all its paths; nil
// maps without a sparse checkout.
func (r *Repo) sparseState() (skip, known map[string]bool, err error) {
	if !r.ConfigBool("core.sparseCheckout", false) {
		return nil, nil, nil
	}
	out, err := r.Run(gitx.Opt{}, "ls-files", "-t", "-z")
	if err != nil {
		return nil, nil, err
	}
	skip, known = map[string]bool{}, map[string]bool{}
	for _, rec := range strings.Split(string(out), "\x00") {
		if len(rec) > 2 {
			known[rec[2:]] = true
			if rec[0] == 'S' {
				skip[rec[2:]] = true
			}
		}
	}
	return skip, known, nil
}

// copySkipWorktree marks entries of index skip-worktree as a sparse checkout
// needs: as the default index marks them, and, for paths that the default
// index does not have, when the sparse-checkout rules exclude them. When
// only is set, it limits the marking to those paths.
func (r *Repo) copySkipWorktree(index string, only []string, keepFormat bool) error {
	skip, known, err := r.sparseState()
	if err != nil || known == nil {
		return err
	}
	candidates := only
	if only == nil {
		out, err := r.Run(gitx.Opt{Index: index}, "ls-files", "-z")
		if err != nil {
			return err
		}
		candidates = strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	}
	var paths, unknown []string
	for _, p := range candidates {
		switch {
		case p == "":
		case skip[p]:
			paths = append(paths, p)
		case !known[p]:
			unknown = append(unknown, p)
		}
	}
	outside, err := r.outsideSparse(unknown)
	if err != nil {
		return err
	}
	paths = append(paths, outside...)
	if len(paths) == 0 {
		return nil
	}
	_, err = r.Run(gitx.Opt{Index: index, KeepFormat: keepFormat, Stdin: strings.NewReader(strings.Join(paths, "\x00") + "\x00")},
		"update-index", "--skip-worktree", "-z", "--stdin")
	return err
}

// outsideSparse returns the paths that the sparse-checkout rules exclude.
func (r *Repo) outsideSparse(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	out, err := r.Run(gitx.Opt{Stdin: strings.NewReader(strings.Join(paths, "\x00") + "\x00")},
		"sparse-checkout", "check-rules", "-z")
	if err != nil {
		return nil, err
	}
	inside := map[string]bool{}
	for _, p := range strings.Split(string(out), "\x00") {
		inside[p] = true
	}
	var outside []string
	for _, p := range paths {
		if !inside[p] {
			outside = append(outside, p)
		}
	}
	return outside, nil
}

// editBlobEntries returns the mktree entries that name the blobs of recs.
func editBlobEntries(recs []EditRecord) string {
	var edits strings.Builder
	for i, rec := range recs {
		for _, b := range []struct{ name, oid string }{{"before", rec.Before}, {"after", rec.After}} {
			if b.oid != "" {
				fmt.Fprintf(&edits, "100644 blob %s\t%d-%s\n", b.oid, i, b.name)
			}
		}
	}
	return edits.String()
}

// keepInput returns the mktree input of the tree that the keep ref holds:
// the staged tree, and beside it the edit blobs that edits names.
func keepInput(staged, edits string) string {
	return fmt.Sprintf("040000 tree %s\tstaged\n", staged) + edits
}

// keepDigestPrefix starts the line of the keep commit's message that holds
// the SHA-256 of the input of the commit's tree.
const keepDigestPrefix = "keep-input "

// keepDigest returns the line of the keep commit's message for input.
func keepDigest(input string) string {
	sum := sha256.Sum256([]byte(input))
	return keepDigestPrefix + hex.EncodeToString(sum[:])
}

// keepCommit holds what protect needs of a keep commit: its ID, its parents
// as `%P` prints them, and the digest line of its message. The commit is
// immutable, so the facts never change; the ref says which commit is current.
type keepCommit struct{ commit, parents, digest string }

// parseKeep reads the output of `show -s --format=%H %P%n%B` of the keep
// ref: the commit, its parents, and the digest line of its message.
func parseKeep(out string) keepCommit {
	first, msg, _ := strings.Cut(out, "\n")
	commit, parents, _ := strings.Cut(first, " ")
	k := keepCommit{commit: commit, parents: strings.TrimSpace(parents)}
	for _, line := range strings.Split(msg, "\n") {
		if strings.HasPrefix(line, keepDigestPrefix) {
			k.digest = line
		}
	}
	return k
}

// keepRecordPath is the file of the keep record: the commit, its parents, and
// its digest line, one for each line. The stage writes it after each keep
// commit it makes, so that the next protect needs no `show`.
func (l *Stage) keepRecordPath() string { return filepath.Join(l.Dir, "keep") }

// readKeepRecord returns the keep record, or false when the file is missing
// or damaged.
func (l *Stage) readKeepRecord() (keepCommit, bool) {
	data, err := os.ReadFile(l.keepRecordPath())
	if err != nil {
		return keepCommit{}, false
	}
	f := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(f) != 3 || len(f[0]) != len(l.r.EmptyTree) {
		return keepCommit{}, false
	}
	return keepCommit{f[0], f[1], f[2]}, true
}

// writeKeepRecord replaces the file through a temporary file and a rename,
// without a flush. The file only saves a `show`, so a failed write is ignored.
func (l *Stage) writeKeepRecord(k keepCommit) {
	path := l.keepRecordPath()
	tmp := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	data := k.commit + "\n" + k.parents + "\n" + k.digest + "\n"
	if os.WriteFile(tmp, []byte(data), 0o666) != nil || rename(tmp, path) != nil {
		_ = os.Remove(tmp)
	}
}

// readKeep returns the commit that the keep ref names, with its parents and
// digest; commit is "" when the ref does not exist. With the files ref
// storage it reads the ref's loose file, which Git never packs for a
// per-worktree ref, and a file that names the commit of the keep record gives
// the rest without `show`. A missing file means no keep ref; any other case
// runs `show`.
func (l *Stage) readKeep() keepCommit {
	files := l.r.RefFormat == "files"
	if files {
		data, err := os.ReadFile(filepath.Join(l.r.GitDir, "refs", "worktree", "multistage", l.ID))
		if errors.Is(err, fs.ErrNotExist) {
			return keepCommit{}
		}
		if rec, ok := l.readKeepRecord(); err == nil && ok && strings.TrimSpace(string(data)) == rec.commit {
			return rec
		}
	}
	out, err := l.r.Run(gitx.Opt{}, "show", "-s", "--format=%H %P%n%B", l.keepRef(), "--")
	if err != nil {
		return keepCommit{}
	}
	k := parseKeep(string(out))
	if files && k.commit != "" {
		l.writeKeepRecord(k)
	}
	return k
}

func (l *Stage) protect() error {
	if l.Damaged {
		return nil
	}
	// The staged tree, the base tree, and the keep ref are read at once.
	var tree, base string
	var cur keepCommit
	err := parallel(
		func() (err error) {
			tree, err = l.r.WriteTree(l.IndexPath())
			return err
		},
		func() (err error) {
			base, err = l.r.TreeOf(l.M.Base)
			return err
		},
		func() error {
			cur = l.readKeep()
			return nil
		},
	)
	if err != nil {
		return err
	}
	// Blobs that recorded edits refer to need the same protection.
	recs, err := readRecords(l.Dir)
	if err != nil {
		return err
	}
	edits := editBlobEntries(recs)
	if tree == base && edits == "" {
		if cur.commit != "" {
			_, err = l.r.Run(gitx.Opt{}, "update-ref", "-d", l.keepRef())
			_ = os.Remove(l.keepRecordPath())
		}
		return err
	}
	// The keep commit records the digest of its tree's input, so an
	// unchanged stage needs no mktree.
	input := keepInput(tree, edits)
	digest := keepDigest(input)
	if cur.parents == l.M.Base && cur.digest == digest {
		return nil
	}
	keepTree, err := l.r.Line(gitx.Opt{Stdin: strings.NewReader(input)}, "mktree")
	if err != nil {
		return err
	}
	args := []string{"commit-tree", keepTree, "-m", "git-multistage stage " + l.ID, "-m", digest}
	if l.M.Base != "" {
		args = append(args, "-p", l.M.Base)
	}
	id := []string{
		"GIT_AUTHOR_NAME=git-multistage", "GIT_AUTHOR_EMAIL=git-multistage@localhost",
		"GIT_COMMITTER_NAME=git-multistage", "GIT_COMMITTER_EMAIL=git-multistage@localhost",
	}
	c, err := l.r.Line(gitx.Opt{Env: id}, args...)
	if err != nil {
		return err
	}
	if _, err = l.r.Run(gitx.Opt{}, "update-ref", l.keepRef(), c); err != nil {
		return err
	}
	l.writeKeepRecord(keepCommit{c, l.M.Base, digest})
	return nil
}

func (l *Stage) save() error {
	l.M.BaseTree = l.r.KnownTree(l.M.Base)
	data, err := json.MarshalIndent(l.M, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(l.Dir, "meta.json"), append(data, '\n'))
}

// leftoverLockAge is the age after which a lock file is surely abandoned: a
// Git child of a killed process may hold a younger one.
const leftoverLockAge = time.Hour

// removeLeftovers deletes files that an interrupted process left behind.
// The stage lock guarantees that no other git-multistage process uses them,
// but a Git child of a killed process may still hold a lock file and later
// rename it, so a lock file is deleted only when it is old; newIndexFile
// never reuses a generation whose files exist.
func (l *Stage) removeLeftovers() {
	entries, _ := os.ReadDir(l.Dir)
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".lock") {
			if info, err := e.Info(); err != nil || time.Since(info.ModTime()) < leftoverLockAge {
				continue
			}
		}
		stale := strings.HasSuffix(n, ".lock") ||
			strings.Contains(n, ".tmp-") ||
			(strings.HasPrefix(n, "index-") && n != l.M.Index) ||
			strings.HasPrefix(n, "hook-index")
		if stale {
			_ = os.Remove(filepath.Join(l.Dir, n))
		}
	}
}

// recover finishes or discards a commit that a killed process left pending.
func (l *Stage) recover() error {
	p := l.M.Pending
	if p == nil {
		return nil
	}
	tip, err := l.r.ResolveRef(l.M.Branch)
	if err != nil {
		return err
	}
	landed := false
	// A pending commit that garbage collection pruned never landed.
	if _, err := l.r.Run(gitx.Opt{}, "cat-file", "-e", p.Commit+"^{commit}"); err == nil {
		if landed, err = l.r.IsAncestor(p.Commit, tip); err != nil {
			return err
		}
	}
	if !landed {
		l.M.Pending = nil
		l.warn("an interrupted commit (%s) did not reach %s; the staged changes are kept", short(p.Commit), l.M.Branch)
		return l.save()
	}
	l.warn("finished an interrupted commit %s on %s", short(p.Commit), l.M.Branch)
	if err := l.finishCommit(p.Commit); err != nil {
		return err
	}
	if !l.r.syncEnabled() {
		return nil
	}
	lk, err := l.r.lockIndex(syncTimeoutRecovery)
	if err != nil {
		l.warn("default index not synchronized: %v; then run `git multistage sync --from %s`", err, short(p.Parent))
		return nil
	}
	defer lk.release()
	if res, err := l.r.syncAfterCommit(l.M.Branch, p.Parent); err != nil {
		l.warn("default index not synchronized: %v", err)
	} else {
		l.Warnings = append(l.Warnings, res.Warnings...)
	}
	return nil
}

// finishCommit records commit as made by this stage and clears the staging area.
func (l *Stage) finishCommit(commit string) error {
	l.M.Pending = nil
	l.M.Commits = append(l.M.Commits, commit)
	if n := len(l.M.Commits); n > 500 {
		l.M.Commits = l.M.Commits[n-500:]
	}
	return l.setIndex(commit)
}

// writeFileAtomic replaces path with data through a temporary file and a rename.
func writeFileAtomic(path string, data []byte) error {
	tmp := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := syncFile(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// rename retries on Windows, where a reader that holds the target open makes
// the rename fail for a moment.
func rename(from, to string) error {
	err := os.Rename(from, to)
	for i := 0; err != nil && runtime.GOOS == "windows" && i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		err = os.Rename(from, to)
	}
	return err
}

// readMeta reads a stage's metadata without locking it.
func readMeta(dir string) (Meta, error) {
	var m Meta
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(data, &m)
	return m, err
}

func short(oid string) string {
	if len(oid) > 10 {
		return oid[:10]
	}
	if oid == "" {
		return "(none)"
	}
	return oid
}
