package ms

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chrisfls/git-multistage/internal/filelock"
	"github.com/chrisfls/git-multistage/internal/gitx"
)

// StatusInfo is the state of one stage and of the working tree around it.
type StatusInfo struct {
	Agent     string       `json:"agent"`
	StageDir   string       `json:"stage_dir"`
	Fresh     Freshness    `json:"freshness"`
	Staged    []Change     `json:"staged"`
	Unstaged  []Change     `json:"unstaged"`
	Untracked []string     `json:"untracked"`
	Shared    []SharedPath `json:"shared,omitempty"`
}

// SharedPath is a path that this stage and other stages have staged.
type SharedPath struct {
	Path   string   `json:"path"`
	Agents []string `json:"agents"`
}

// Status reports the stage's staged, unstaged, and untracked changes.
func (l *Stage) Status(f Freshness) (*StatusInfo, error) {
	st := &StatusInfo{Agent: l.Agent, StageDir: l.Dir, Fresh: f, Staged: []Change{}, Unstaged: []Change{}, Untracked: []string{}}
	var err error
	if st.Staged, err = l.stagedChanges(); err != nil {
		return nil, err
	}
	out, err := l.r.Run(gitx.Opt{Index: l.IndexPath()}, "diff", "-z", "--name-status", "--no-renames")
	if err != nil {
		return nil, err
	}
	st.Unstaged = append(st.Unstaged, parseNameStatus(out)...)
	out, err = l.r.Run(gitx.Opt{Index: l.IndexPath()}, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			st.Untracked = append(st.Untracked, p)
		}
	}
	others, err := l.otherStagesStaged()
	if err != nil {
		return nil, err
	}
	for _, c := range st.Staged {
		if a := others[c.Path]; len(a) > 0 {
			st.Shared = append(st.Shared, SharedPath{c.Path, a})
		}
	}
	return st, nil
}

// StageInfo summarizes one stage for `list` and `doctor`.
type StageInfo struct {
	Agent    string   `json:"agent"`
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
	head, _ := r.SymbolicHead()
	res := []StageInfo{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(r.stagesDir(), e.Name())
		li := StageInfo{Agent: e.Name(), Staged: []string{}}
		if lk, err := filelock.Acquire(filepath.Join(dir, "lock"), 0); err != nil {
			li.Busy = true
		} else {
			lk.Release()
		}
		m, err := readMeta(dir)
		if err != nil {
			li.Problems = append(li.Problems, "unreadable metadata: "+err.Error())
			res = append(res, li)
			continue
		}
		li.Branch, li.Base, li.Commits = m.Branch, m.Base, len(m.Commits)
		if m.Pending != nil {
			li.Pending = m.Pending.Commit
			li.Problems = append(li.Problems, "an interrupted commit is pending; the next command of this stage finishes it")
		}
		if m.Branch != head {
			li.Problems = append(li.Problems, fmt.Sprintf("targets %s, but HEAD is on %s", m.Branch, orDetached(head)))
		}
		if m.Base != "" {
			if _, err := r.Run(gitx.Opt{}, "cat-file", "-e", m.Base+"^{commit}"); err != nil {
				li.Problems = append(li.Problems, "base commit "+short(m.Base)+" is missing")
			}
		}
		tip, _ := r.ResolveRef(m.Branch)
		li.Behind = tip != m.Base
		if paths, err := r.stageStagedPaths(dir); err != nil {
			li.Problems = append(li.Problems, "cannot read the index: "+err.Error())
		} else {
			li.Staged = append(li.Staged, paths...)
		}
		res = append(res, li)
	}
	return res, nil
}

// Check is one doctor finding.
type Check struct {
	Level   string `json:"level"` // ok, warning, error
	Message string `json:"message"`
}

// DoctorReport is the result of `doctor`.
type DoctorReport struct {
	Checks []Check     `json:"checks"`
	Stages  []StageInfo  `json:"stages"`
	Sync   *SyncResult `json:"index_sync,omitempty"`
}

// Doctor inspects the repository, the default index, and every stage, and
// changes nothing.
func (r *Repo) Doctor() (*DoctorReport, error) {
	rep := &DoctorReport{}
	add := func(level, format string, args ...any) {
		rep.Checks = append(rep.Checks, Check{level, fmt.Sprintf(format, args...)})
	}
	v, _ := r.Line(gitx.Opt{}, "version")
	add("ok", "%s", v)
	head, err := r.SymbolicHead()
	if err != nil {
		return nil, err
	}
	tip := ""
	switch {
	case head == "":
		add("error", "HEAD is detached; stages need a checked-out branch")
	default:
		tip, _ = r.ResolveRef(head)
		if tip == "" {
			add("ok", "HEAD is on %s, which has no commits yet", head)
		} else {
			add("ok", "HEAD is on %s at %s", head, short(tip))
		}
	}
	if op := r.inProgress(); op != "" {
		add("warning", "%s is in progress; commits are refused until it ends", op)
	}
	if err := r.checkSupported(); err != nil {
		add("error", "%s", err.Error())
	}
	if _, err := os.Stat(r.IndexPath + ".lock"); err == nil {
		add("warning", "%s exists: a Git process is running, or one crashed; if none is running, delete it", r.IndexPath+".lock")
	}
	if !r.syncEnabled() {
		add("warning", "multistage.syncIndex is false: commits leave the default index behind the branch")
	} else if head != "" && tip != "" {
		res, err := r.syncIndexLocked("", true)
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
	}
	if rep.Stages, err = r.Stages(); err != nil {
		return nil, err
	}
	staged := map[string][]string{}
	for _, li := range rep.Stages {
		for _, p := range li.Staged {
			staged[p] = append(staged[p], li.Agent)
		}
		for _, p := range li.Problems {
			add("warning", "stage %s: %s", li.Agent, p)
		}
	}
	var shared []string
	for p, agents := range staged {
		if len(agents) > 1 {
			shared = append(shared, fmt.Sprintf("%s (%s)", p, strings.Join(agents, ", ")))
		}
	}
	sort.Strings(shared)
	if len(shared) > 0 {
		add("warning", "files staged by several agents: %s", strings.Join(shared, "; "))
	}
	add("ok", "%d stage(s) in this worktree", len(rep.Stages))
	return rep, nil
}

// Drop deletes the stage. With staged changes it refuses unless force is set.
func (l *Stage) Drop(force bool) (string, error) {
	staged, err := l.r.WriteTree(l.IndexPath())
	if err != nil {
		return "", err
	}
	base, err := l.r.TreeOf(l.M.Base)
	if err != nil {
		return "", err
	}
	if staged != base && !force {
		return "", &Error{Code: ExitRefused, Kind: "has-staged",
			Message: "stage " + l.Agent + " has staged changes; nothing was dropped",
			Hint:    "commit or unstage them, or pass --force"}
	}
	l.Close()
	if err := os.RemoveAll(l.Dir); err != nil {
		return "", err
	}
	if staged == base {
		return "", nil
	}
	return staged, nil
}
