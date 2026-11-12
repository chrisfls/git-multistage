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
	Dir       string // <per-worktree git dir>/multistage
	IndexPath string // the worktree's default index
}

// OpenRepo discovers the repository that contains dir.
func OpenRepo(dir string) (*Repo, error) {
	g, err := gitx.Open(dir)
	if err != nil {
		return nil, err
	}
	idx, err := g.Line(gitx.Opt{}, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return nil, err
	}
	return &Repo{Repo: g, Dir: filepath.Join(g.GitDir, "multistage"), IndexPath: filepath.Clean(idx)}, nil
}

func (r *Repo) stagesDir() string { return filepath.Join(r.Dir, "stages") }

// checkSupported refuses repository layouts whose index semantics the
// stages cannot reproduce.
func (r *Repo) checkSupported() error {
	for _, key := range []string{"core.sparseCheckout", "core.splitIndex", "index.sparse"} {
		v, _ := r.Line(gitx.Opt{}, "config", "--type=bool", "--get", key)
		if v == "true" {
			return errorf(ExitRefused, "unsupported", "%s is enabled; git-multistage does not support it", key)
		}
	}
	return nil
}

// configBool reads a boolean Git setting.
func (r *Repo) configBool(key string, def bool) bool {
	v, err := r.Line(gitx.Opt{}, "config", "--type=bool", "--get", key)
	if err != nil || v == "" {
		return def
	}
	return v == "true"
}

var idRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
var reservedRE = regexp.MustCompile(`^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$`)

// ValidateStageID checks that id is usable as a directory name on every
// platform, including case-insensitive file systems, and as a ref name.
func ValidateStageID(id string) error {
	if id == "" {
		return &Error{Code: ExitUsage, Kind: "no-stage-id",
			Message: "no stage ID: pass --stage <id>, set GIT_MULTISTAGE_ID, or pass --agentic inside an agent",
			Hint:    "run `git multistage init` to create a stage with a new unique ID"}
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
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
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
	if err := ValidateStageID(id); err != nil {
		return nil, err
	}
	if err := r.checkSupported(); err != nil {
		return nil, err
	}
	dir := filepath.Join(r.stagesDir(), id)
	if err := os.MkdirAll(r.stagesDir(), 0o777); err != nil {
		return nil, err
	}
	if exclusive {
		if err := os.Mkdir(dir, 0o777); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return nil, &Error{Code: ExitRefused, Kind: "stage-exists",
					Message: fmt.Sprintf("stage %q already exists", id),
					Hint:    "if this stage is yours, keep using it; otherwise choose another ID"}
			}
			return nil, err
		}
	} else if err := os.MkdirAll(dir, 0o777); err != nil {
		return nil, err
	}
	lk, err := filelock.Acquire(filepath.Join(dir, "lock"), stageLockTimeout)
	if err != nil {
		if errors.Is(err, filelock.ErrBusy) {
			return nil, errorf(ExitBusy, "busy", "stage %q is busy: another git-multistage process is using it", id)
		}
		return nil, err
	}
	l := &Stage{r: r, ID: id, Dir: dir, lock: lk}
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := l.create(); err != nil {
			lk.Release()
			return nil, err
		}
	case err != nil:
		lk.Release()
		return nil, err
	default:
		if err := json.Unmarshal(data, &l.M); err != nil {
			lk.Release()
			return nil, fmt.Errorf("stage %q has unreadable metadata %s: %w", id, filepath.Join(dir, "meta.json"), err)
		}
	}
	l.removeLeftovers()
	if _, err := os.Stat(l.IndexPath()); err != nil {
		l.Damaged = true
		return l, nil
	}
	if err := l.recover(); err != nil {
		lk.Release()
		return nil, err
	}
	return l, nil
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
	return &Error{Code: ExitError, Kind: "damaged-stage",
		Message: fmt.Sprintf("stage %s is damaged: its index file %s is missing", l.ID, l.M.Index),
		Hint:    "run `git multistage drop --force`; " + l.keepRef() + " holds the last protected staged tree, if any"}
}

func (l *Stage) create() error {
	branch, err := l.r.SymbolicHead()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(branch, "refs/heads/") {
		return errorf(ExitRefused, "detached-head", "HEAD is detached; check out a branch before using git-multistage")
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
	return l.setIndexTree(tree, base)
}

// setIndexTree makes a fresh index generation that holds tree, with base as
// the stage's base. The metadata rename is the commit point: until it
// happens, the old index generation stays current.
func (l *Stage) setIndexTree(tree, base string) error {
	name, gen := l.newIndexFile()
	path := filepath.Join(l.Dir, name)
	if err := l.r.ReadTree(path, tree); err != nil {
		return err
	}
	l.M.Base = base
	return l.adoptIndex(name, gen)
}

// newIndexFile returns the name and generation of the next index file, after
// removing any leftover file of that name.
func (l *Stage) newIndexFile() (string, int) {
	gen := l.M.Gen + 1
	name := fmt.Sprintf("index-%d", gen)
	os.Remove(filepath.Join(l.Dir, name))
	os.Remove(filepath.Join(l.Dir, name+".lock"))
	return name, gen
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
		os.Remove(filepath.Join(l.Dir, old))
	}
	return nil
}

// keepRef is the per-worktree ref that keeps a stage's staged objects
// reachable, so that `git gc` does not prune them.
func (l *Stage) keepRef() string { return "refs/worktree/multistage/" + l.ID }

// protect points keepRef at a commit of the staged tree while the stage has
// staged changes, and deletes it otherwise.
func (l *Stage) protect() error {
	if l.Damaged {
		return nil
	}
	tree, err := l.r.WriteTree(l.IndexPath())
	if err != nil {
		return err
	}
	base, err := l.r.TreeOf(l.M.Base)
	if err != nil {
		return err
	}
	cur, _ := l.r.Line(gitx.Opt{}, "show", "-s", "--format=%T %P", l.keepRef(), "--")
	if tree == base {
		if cur != "" {
			_, err = l.r.Run(gitx.Opt{}, "update-ref", "-d", l.keepRef())
		}
		return err
	}
	want := strings.TrimSpace(tree + " " + l.M.Base)
	if cur == want {
		return nil
	}
	args := []string{"commit-tree", tree, "-m", "git-multistage stage " + l.ID}
	if l.M.Base != "" {
		args = append(args, "-p", l.M.Base)
	}
	id := []string{"GIT_AUTHOR_NAME=git-multistage", "GIT_AUTHOR_EMAIL=git-multistage@localhost",
		"GIT_COMMITTER_NAME=git-multistage", "GIT_COMMITTER_EMAIL=git-multistage@localhost"}
	c, err := l.r.Line(gitx.Opt{Env: id}, args...)
	if err != nil {
		return err
	}
	_, err = l.r.Run(gitx.Opt{}, "update-ref", l.keepRef(), c)
	return err
}

func (l *Stage) save() error {
	data, err := json.MarshalIndent(l.M, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(l.Dir, "meta.json"), append(data, '\n'))
}

// removeLeftovers deletes files that an interrupted process left behind.
// The stage lock guarantees that no other git-multistage process uses them.
func (l *Stage) removeLeftovers() {
	entries, _ := os.ReadDir(l.Dir)
	for _, e := range entries {
		n := e.Name()
		stale := strings.HasSuffix(n, ".lock") ||
			strings.Contains(n, ".tmp-") ||
			(strings.HasPrefix(n, "index-") && n != l.M.Index) ||
			n == "hook-index"
		if stale {
			os.Remove(filepath.Join(l.Dir, n))
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
	if res, err := l.r.syncAfterCommit(p.Parent); err != nil {
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
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := rename(tmp, path); err != nil {
		os.Remove(tmp)
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
