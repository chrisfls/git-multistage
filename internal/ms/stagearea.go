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

var agentRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
var reservedRE = regexp.MustCompile(`^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$`)

// ValidateAgent checks that id is usable as a stage directory name on every
// platform, including case-insensitive file systems.
func ValidateAgent(id string) error {
	if id == "" {
		return &Error{Code: ExitUsage, Kind: "no-agent",
			Message: "no agent ID: set GIT_MULTISTAGE_ID or pass --agent <id>",
			Hint:    "run `git multistage init` to create a stage with a new unique ID"}
	}
	if !agentRE.MatchString(id) || strings.HasSuffix(id, ".") || reservedRE.MatchString(id) {
		return usageErr("invalid agent ID %q: use 1-64 lowercase letters, digits, '.', '_' or '-', starting with a letter or digit", id)
	}
	return nil
}

// NewAgentID returns a random agent ID.
func NewAgentID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "agent-" + hex.EncodeToString(b)
}

// Meta is the persistent state of one stage.
type Meta struct {
	Version int       `json:"version"`
	Agent   string    `json:"agent"`
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

// Stage is an agent's staging area, locked by this process.
type Stage struct {
	r        *Repo
	Agent    string
	Dir      string
	M        Meta
	lock     *filelock.Lock
	Created  bool
	Warnings []string
}

const stageLockTimeout = 60 * time.Second

func (l *Stage) warn(format string, args ...any) {
	l.Warnings = append(l.Warnings, fmt.Sprintf(format, args...))
}

// IndexPath returns the path of the stage's current index file.
func (l *Stage) IndexPath() string { return filepath.Join(l.Dir, l.M.Index) }

// OpenStage locks the stage of agent, creating it when it does not exist, and
// finishes any interrupted commit. When exclusive is set, an existing stage is
// an error.
func (r *Repo) OpenStage(agent string, exclusive bool) (*Stage, error) {
	if err := ValidateAgent(agent); err != nil {
		return nil, err
	}
	if err := r.checkSupported(); err != nil {
		return nil, err
	}
	dir := filepath.Join(r.stagesDir(), agent)
	if err := os.MkdirAll(r.stagesDir(), 0o777); err != nil {
		return nil, err
	}
	if exclusive {
		if err := os.Mkdir(dir, 0o777); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return nil, &Error{Code: ExitRefused, Kind: "stage-exists",
					Message: fmt.Sprintf("stage %q already exists", agent),
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
			return nil, errorf(ExitBusy, "busy", "stage %q is busy: another git-multistage process for this agent is running", agent)
		}
		return nil, err
	}
	l := &Stage{r: r, Agent: agent, Dir: dir, lock: lk}
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
			return nil, fmt.Errorf("stage %q has unreadable metadata %s: %w", agent, filepath.Join(dir, "meta.json"), err)
		}
	}
	l.removeLeftovers()
	if err := l.recover(); err != nil {
		lk.Release()
		return nil, err
	}
	return l, nil
}

// Close releases the stage lock.
func (l *Stage) Close() { l.lock.Release() }

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
	l.M = Meta{Version: 1, Agent: l.Agent, Branch: branch, Created: time.Now().UTC()}
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
	gen := l.M.Gen + 1
	name := fmt.Sprintf("index-%d", gen)
	path := filepath.Join(l.Dir, name)
	os.Remove(path)
	os.Remove(path + ".lock")
	if err := l.r.ReadTree(path, tree); err != nil {
		return err
	}
	old := l.M.Index
	l.M.Index, l.M.Gen, l.M.Base = name, gen, base
	if err := l.save(); err != nil {
		return err
	}
	if old != "" && old != name {
		os.Remove(filepath.Join(l.Dir, old))
	}
	return nil
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
			n == "hook-index" || n == "stage-index"
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
	landed, err := l.r.IsAncestor(p.Commit, tip)
	if err != nil {
		return err
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
	if res, err := l.r.SyncIndex(p.Parent, syncTimeoutRecovery); err != nil {
		l.warn("default index not synchronized: %v; run `git multistage sync`", err)
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
