package ms

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/chrisfls/git-multistage/internal/filelock"
	"github.com/chrisfls/git-multistage/internal/gitx"
)

// StatusInfo is the state of one stage and of the working tree around it.
type StatusInfo struct {
	ID        string       `json:"stage"`
	StageDir  string       `json:"stage_dir"`
	Fresh     Freshness    `json:"freshness"`
	Staged    []Change     `json:"staged"`
	Unstaged  []Change     `json:"unstaged"`
	Untracked []string     `json:"untracked"`
	Shared    []SharedPath `json:"shared,omitempty"`
	Recorded  []string     `json:"recorded_edits,omitempty"` // paths with edits that the editor hook recorded
}

// SharedPath is a path that this stage and other stages have staged.
type SharedPath struct {
	Path   string   `json:"path"`
	Stages []string `json:"stages"`
}

// Status reports the stage's staged, unstaged, and untracked changes.
func (l *Stage) Status(f Freshness) (*StatusInfo, error) {
	st := &StatusInfo{ID: l.ID, StageDir: l.Dir, Fresh: f, Staged: []Change{}, Unstaged: []Change{}, Untracked: []string{}}
	// The three reads leave the index alone, so they run at once.
	var unstaged, untracked []byte
	err := parallel(
		func() (err error) {
			st.Staged, err = l.stagedChanges()
			return err
		},
		func() (err error) {
			unstaged, err = l.r.Run(gitx.Opt{Index: l.IndexPath()}, cmdDiff, "-z", "--name-status", "--no-renames")
			return err
		},
		func() (err error) {
			untracked, err = l.r.Run(gitx.Opt{Index: l.IndexPath()}, "ls-files", "-z", "--others", "--exclude-standard")
			return err
		},
	)
	if err != nil {
		return nil, err
	}
	st.Unstaged = append(st.Unstaged, parseNameStatus(unstaged)...)
	for _, p := range strings.Split(string(untracked), "\x00") {
		if p != "" {
			st.Untracked = append(st.Untracked, p)
		}
	}
	if st.Recorded, err = l.RecordedPaths(); err != nil {
		return nil, err
	}
	if len(st.Staged) == 0 {
		// A stage with nothing staged shares no path with another stage.
		return st, nil
	}
	others := l.otherStagesStaged()
	for _, c := range st.Staged {
		if a := others[c.Path]; len(a) > 0 {
			st.Shared = append(st.Shared, SharedPath{c.Path, a})
		}
	}
	return st, nil
}

// StageInfo summarizes one stage for `list` and `doctor`.
type StageInfo struct {
	ID       string   `json:"stage"`
	Branch   string   `json:"branch"`
	Base     string   `json:"base"`
	Staged   []string `json:"staged"`
	Behind   bool     `json:"behind"` // the base is not the branch tip
	Busy     bool     `json:"busy"`   // a git-multistage process holds the stage now
	Pending  string   `json:"pending_commit,omitempty"`
	Commits  int      `json:"commits"`
	Problems []string `json:"problems,omitempty"`
}

// Stages lists the stages of this worktree without locking them.
func (r *Repo) Stages() ([]StageInfo, error) {
	entries, err := os.ReadDir(r.stagesDir())
	if errors.Is(err, os.ErrNotExist) {
		return []StageInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	head, _ := r.headTarget()
	res := []StageInfo{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			if strings.HasPrefix(e.Name(), ".dropped-") {
				_ = os.RemoveAll(filepath.Join(r.stagesDir(), e.Name()))
			}
			continue
		}
		if li, ok := r.stageInfo(e.Name(), head); ok {
			res = append(res, li)
		}
	}
	return res, nil
}

// stageInfo describes stage id. It reports false for a stage that the
// editor hook created and no command opened yet.
func (r *Repo) stageInfo(id, head string) (StageInfo, bool) {
	dir := filepath.Join(r.stagesDir(), id)
	li := StageInfo{ID: id, Staged: []string{}}
	if lk, err := filelock.Acquire(filepath.Join(dir, "lock"), 0); err != nil {
		li.Busy = true
	} else {
		lk.Release()
	}
	m, err := readMeta(dir)
	if errors.Is(err, os.ErrNotExist) {
		return li, false // the editor hook recorded edits; the stage is created on its first command
	}
	if err != nil {
		li.Problems = append(li.Problems, "unreadable metadata: "+err.Error())
		return li, true
	}
	li.Branch, li.Base, li.Commits = m.Branch, m.Base, len(m.Commits)
	li.Problems = append(li.Problems, r.metaProblems(&m, head)...)
	tip, _ := r.ResolveRef(m.Branch)
	li.Behind = tip != m.Base
	if m.Pending != nil {
		li.Pending = m.Pending.Commit
	}
	if paths, err := r.stageStagedPaths(dir); err != nil {
		li.Problems = append(li.Problems, "cannot read the index: "+err.Error())
	} else {
		li.Staged = append(li.Staged, paths...)
	}
	return li, true
}

// metaProblems lists the problems that a stage's metadata shows.
func (r *Repo) metaProblems(m *Meta, head string) []string {
	var problems []string
	if m.Pending != nil {
		problems = append(problems, "an interrupted commit is pending; the next command of this stage finishes it")
	}
	if m.Branch != head {
		problems = append(problems, fmt.Sprintf("targets %s, but HEAD is %s", m.Branch, describeTarget(head)))
	}
	if m.Base != "" {
		if _, err := r.Run(gitx.Opt{}, "cat-file", "-e", m.Base+"^{commit}"); err != nil {
			problems = append(problems, "base commit "+short(m.Base)+" is missing")
		}
	}
	return problems
}

// Check is one doctor finding.
type Check struct {
	Level   string `json:"level"` // ok, warning, error
	Message string `json:"message"`
}

// DoctorReport is the result of `doctor`.
type DoctorReport struct {
	Checks []Check     `json:"checks"`
	Stages []StageInfo `json:"stages"`
	Sync   *SyncResult `json:"index_sync,omitempty"`
}

// orphanedIndexLocks lists the index lock files in the directory of stage
// id. Call it only for a stage that no command holds: then no Git child of
// a running command holds them.
func (r *Repo) orphanedIndexLocks(id string) []string {
	var locks []string
	for _, pattern := range []string{"index-*.lock", "hook-index-*.lock"} {
		m, _ := filepath.Glob(filepath.Join(r.stagesDir(), id, pattern))
		locks = append(locks, m...)
	}
	return locks
}

// Doctor inspects the repository, the default index, and every stage, and
// changes nothing.
func (r *Repo) Doctor() (*DoctorReport, error) {
	rep := &DoctorReport{}
	add := rep.add
	v, _ := r.Line(gitx.Opt{}, "version")
	add("ok", "%s", v)
	head, err := r.headTarget()
	if err != nil {
		return nil, err
	}
	tip, _ := r.ResolveRef(head)
	switch tip {
	case "":
		add("ok", "HEAD is %s, which has no commits yet", describeTarget(head))
	default:
		add("ok", "HEAD is %s at %s", describeTarget(head), short(tip))
	}
	if op, blocking := r.inProgress(); blocking {
		add("warning", "%s is in progress; commits are refused until it ends", op)
	} else if op != "" {
		add("ok", "%s is in progress; a commit is refused only when that operation has changed the same paths in the index", op)
	}
	if _, err := os.Stat(r.IndexPath + ".lock"); err == nil {
		add("warning", "%s exists: a Git process is running, or one crashed; if none is running, delete it", r.IndexPath+".lock")
	}
	if !r.syncEnabled() {
		add("warning", "multistage.syncIndex is false: commits leave the default index behind the branch")
	}
	if tip != "" {
		if err := r.doctorIndex(rep, head, tip); err != nil {
			return nil, err
		}
	}
	if rep.Stages, err = r.Stages(); err != nil {
		return nil, err
	}
	r.doctorStages(rep)
	add("ok", "%d stage(s) in this worktree", len(rep.Stages))
	return rep, nil
}

func (rep *DoctorReport) add(level, format string, args ...any) {
	rep.Checks = append(rep.Checks, Check{level, fmt.Sprintf(format, args...)})
}

// doctorIndex checks whether the default index lags head, whose tip is tip.
func (r *Repo) doctorIndex(rep *DoctorReport, head, tip string) error {
	add := rep.add
	from, err := r.provenLag(head)
	if err != nil {
		return err
	}
	if from == "" {
		msg := "the default index holds natively staged changes (`git status` shows them)"
		if c, n := r.nearestAncestor(head); c != "" && c != tip {
			msg += fmt.Sprintf("; it is closest to %s (%d path(s) differ), so it may also lag behind: `git multistage sync --from %s` brings it forward", short(c), n, short(c))
		}
		add("ok", "%s", msg)
		return nil
	}
	var res SyncResult
	now, err := r.ResolveRef(head)
	if err == nil {
		res, err = r.syncIndexLocked(head, now, from, false, true)
	}
	switch {
	case err != nil:
		add("warning", "cannot compare the default index with %s: %v", head, err)
	case len(res.Updated) > 0:
		add("warning", "the default index is behind %s at %d path(s): %s; run `git multistage sync`",
			head, len(res.Updated), strings.Join(res.Updated, ", "))
	default:
		add("ok", "the default index is in step with %s", head)
	}
	for _, w := range res.Warnings {
		add("warning", "%s", w)
	}
	rep.Sync = &res
	return nil
}

// doctorStages reports each stage's problems and orphaned index locks, and
// the files staged in several stages.
func (r *Repo) doctorStages(rep *DoctorReport) {
	add := rep.add
	staged := map[string][]string{}
	for _, li := range rep.Stages {
		for _, p := range li.Staged {
			staged[p] = append(staged[p], li.ID)
		}
		for _, p := range li.Problems {
			add("warning", "stage %s: %s", li.ID, p)
		}
		if !li.Busy {
			for _, lock := range r.orphanedIndexLocks(li.ID) {
				add("warning", "stage %s: %s exists: a Git process of a killed command left it, or still runs; "+
					"the stage's next command moves past it, and deletes it once it is an hour old", li.ID, lock)
			}
		}
	}
	var shared []string
	for p, ids := range staged {
		if len(ids) > 1 {
			shared = append(shared, fmt.Sprintf("%s (%s)", p, strings.Join(ids, ", ")))
		}
	}
	sort.Strings(shared)
	if len(shared) > 0 {
		add("warning", "files staged in several stages: %s", strings.Join(shared, "; "))
	}
}

// Drop deletes the stage. With staged changes it refuses unless force is
// set, and returns the staged tree so that it can still be inspected.
func (l *Stage) Drop(force bool) (string, error) {
	var staged, base string
	var err error
	if l.Damaged {
		staged, _ = l.r.Line(gitx.Opt{}, "rev-parse", "-q", "--verify", l.keepRef()+"^{tree}")
	} else if staged, err = l.r.WriteTree(l.IndexPath()); err != nil {
		return "", err
	}
	if base, err = l.r.TreeOf(l.M.Base); err != nil {
		return "", err
	}
	if (staged != base || l.Damaged) && !force {
		return "", &Error{
			Code: ExitRefused, Kind: "has-staged",
			Message: "stage " + l.ID + " has staged changes or is damaged; nothing was dropped",
			Hint:    "commit or unstage the changes, or pass --force",
		}
	}
	if _, err := l.r.Run(gitx.Opt{}, "update-ref", "-d", l.keepRef()); err != nil {
		return "", err
	}
	// Rename first, so that an interrupted removal never leaves a stage whose
	// metadata points at a deleted index. The lock is held through the rename
	// where the system allows renaming a directory with an open file; a
	// process that waited for the lock finds that its lock file moved, and
	// locks the lock path again.
	testPause("before-drop-rename")
	if runtime.GOOS == "windows" {
		l.lock.Release()
		l.lock = nil
	}
	trash := filepath.Join(l.r.stagesDir(), fmt.Sprintf(".dropped-%s-%d", l.ID, os.Getpid()))
	err = rename(l.Dir, trash)
	l.lock.Release()
	l.lock = nil
	if err != nil {
		return "", err
	}
	_ = os.Remove(l.r.stagedPathsFile(l.ID))
	if err := os.RemoveAll(trash); err != nil {
		return "", err
	}
	if staged == base {
		return "", nil
	}
	return staged, nil
}
