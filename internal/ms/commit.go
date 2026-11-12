package ms

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

const maxCommitAttempts = 100

// CommitResult describes a finished commit.
type CommitResult struct {
	Result   string     `json:"result"` // "committed" or "already-applied"
	Commit   string     `json:"commit,omitempty"`
	Parent   string     `json:"parent,omitempty"`
	Branch   string     `json:"branch"`
	Attempts int        `json:"attempts"`
	Files    []Change   `json:"files"`
	Sync     SyncResult `json:"index_sync"`
	Subject  string     `json:"subject,omitempty"`
}

// inProgress names a Git operation that is in progress in this worktree.
// blocking is set for operations that a moving HEAD would break: a rebase
// finishes by moving the branch from the commit it started on.
func (r *Repo) inProgress() (op string, blocking bool) {
	for _, f := range []struct {
		name, op string
		blocking bool
	}{
		{"rebase-merge", "a rebase", true},
		{"rebase-apply", "a rebase or am", true},
		{"sequencer", "a cherry-pick or revert sequence", true},
		{"MERGE_HEAD", "a merge", false},
		{"CHERRY_PICK_HEAD", "a cherry-pick", false},
		{"REVERT_HEAD", "a revert", false},
	} {
		if _, err := os.Stat(filepath.Join(r.GitDir, f.name)); err == nil {
			return f.op, f.blocking
		}
	}
	return "", false
}

// checkOperation decides whether a commit from parent to commit may land
// while a merge, cherry-pick, or revert is in progress. It may when the
// default index holds the parent's version of every path the commit changes:
// the sync then adds the commit's changes, and the operation's own commit
// later includes them. The caller holds index.lock.
func (r *Repo) checkOperation(parent, commit string, sync bool) error {
	op, blocking := r.inProgress()
	if op == "" {
		return nil
	}
	if blocking {
		return errorf(ExitRefused, "operation-in-progress", "%s is in progress in this worktree; nothing was committed", op)
	}
	if !sync {
		return errorf(ExitRefused, "operation-in-progress",
			"%s is in progress and multistage.syncIndex is false, so its commit would undo this one; nothing was committed", op)
	}
	from := parent
	if from == "" {
		from = r.EmptyTree
	}
	changes, err := r.DiffTrees(from, commit, false)
	if err != nil {
		return err
	}
	var res SyncResult
	if _, err := r.planIndexChanges(r.IndexPath, changes, &res); err != nil {
		return err
	}
	if len(res.Kept) > 0 {
		return &Error{
			Code: ExitRefused, Kind: "operation-in-progress",
			Message: op + " is in progress and has changed these paths in the index; nothing was committed",
			Paths:   res.Kept, Hint: "finish or abort " + op + ", then commit again",
		}
	}
	return nil
}

// runHook runs a Git hook through `git hook run`, so core.hooksPath and
// configured hooks apply. Hook output goes to stderr.
func (r *Repo) runHook(index, name string, args ...string) error {
	cmd := append([]string{"hook", "run", "--ignore-missing", name}, args...)
	if len(args) > 0 {
		cmd = append([]string{"hook", "run", "--ignore-missing", name, "--"}, args...)
	}
	_, err := r.Run(gitx.Opt{Index: index, User: true, Stdout: os.Stderr, Stderr: os.Stderr}, cmd...)
	return err
}

// prepareMessage runs the message hooks and cleans the message as
// `git commit -m` does.
func (l *Stage) prepareMessage(raw string, noVerify bool) (string, error) {
	file := filepath.Join(l.Dir, "COMMIT_EDITMSG")
	clean := func(s string) (string, error) {
		return stringOut(l.r.Run(gitx.Opt{Stdin: strings.NewReader(s)}, "stripspace"))
	}
	if err := os.WriteFile(file, []byte(raw), 0o666); err != nil {
		return "", err
	}
	if err := l.withIndexCopy(func(idx string) error { return l.r.runHook(idx, "prepare-commit-msg", file, "message") }); err != nil {
		return "", &Error{Code: ExitError, Kind: "hook-failed", Message: "the prepare-commit-msg hook failed; nothing was committed"}
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	msg, err := clean(string(data))
	if err != nil {
		return "", err
	}
	if !noVerify {
		if err := os.WriteFile(file, []byte(msg), 0o666); err != nil {
			return "", err
		}
		if err := l.withIndexCopy(func(idx string) error { return l.r.runHook(idx, "commit-msg", file) }); err != nil {
			return "", &Error{Code: ExitError, Kind: "hook-failed", Message: "the commit-msg hook failed; nothing was committed and the staged changes are kept"}
		}
		if data, err = os.ReadFile(file); err != nil {
			return "", err
		}
		if msg, err = clean(string(data)); err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(msg) == "" {
		return "", &Error{Code: ExitError, Kind: "empty-message", Message: "the commit message is empty"}
	}
	return msg, nil
}

func stringOut(b []byte, err error) (string, error) { return string(b), err }

// runPreCommit runs the pre-commit hook against an index that holds tree,
// and returns the tree that the hook leaves in that index.
func (l *Stage) runPreCommit(tree string) (string, error) {
	idx := filepath.Join(l.Dir, "hook-index")
	_ = os.Remove(idx)
	defer func() { _ = os.Remove(idx) }()
	if _, err := l.r.Run(gitx.Opt{Index: idx}, "read-tree", tree); err != nil {
		return "", err
	}
	if err := l.r.runHook(idx, "pre-commit"); err != nil {
		return "", &Error{
			Code: ExitError, Kind: "hook-failed",
			Message: "the pre-commit hook failed; nothing was committed and the staged changes are kept",
		}
	}
	return l.r.WriteTree(idx)
}

// Commit commits the stage's staged changes onto the current tip of its
// branch. Other stages or tools may move the branch at any time: the staged change is
// replayed onto the newest tip and the branch moves only by compare-and-swap.
func (l *Stage) Commit(rawMsg string, noVerify bool) (*CommitResult, error) {
	r := l.r
	res := &CommitResult{Branch: l.M.Branch}
	tip0, err := r.ResolveRef(l.M.Branch)
	if err != nil {
		return nil, err
	}
	if err := l.checkTarget(tip0); err != nil {
		return nil, err
	}
	if op, blocking := r.inProgress(); blocking {
		return nil, errorf(ExitRefused, "operation-in-progress", "%s is in progress in this worktree; finish or abort it before committing", op)
	}
	staged, err := r.WriteTree(l.IndexPath())
	if err != nil {
		return nil, err
	}
	baseTree, err := r.TreeOf(l.M.Base)
	if err != nil {
		return nil, err
	}
	if staged == baseTree {
		return nil, &Error{Code: ExitError, Kind: "nothing-staged", Message: "nothing is staged for " + l.ID}
	}
	if res.Files, err = l.stagedChanges(); err != nil {
		return nil, err
	}
	msg, err := l.prepareMessage(rawMsg, noVerify)
	if err != nil {
		return nil, err
	}
	res.Subject, _, _ = strings.Cut(msg, "\n")
	sign := r.configBool("commit.gpgSign", false)
	sync := r.syncEnabled()
	var verified []gitx.Entry // changes that the pre-commit hook accepted

	for attempt := 1; attempt <= maxCommitAttempts; attempt++ {
		res.Attempts = attempt
		tip, err := r.ResolveRef(l.M.Branch)
		if err != nil {
			return nil, err
		}
		if tip == "" && l.M.Base != "" {
			return nil, errorf(ExitRefused, "branch-missing", "branch %s no longer exists; nothing was committed", l.M.Branch)
		}
		tree, err := r.integrate(baseTree, staged, tip)
		if err != nil {
			return nil, err
		}
		tipTree, err := r.TreeOf(tip)
		if err != nil {
			return nil, err
		}
		if tree == tipTree {
			// Every staged change is already on the branch.
			if err := l.setIndex(tip); err != nil {
				return nil, err
			}
			res.Result, res.Parent = "already-applied", tip
			return res, nil
		}
		if !noVerify {
			// A retry that introduces exactly the changes that the hook
			// already accepted does not run the hook again, so a slow hook
			// cannot starve the commit while other stages commit.
			introduced, err := r.DiffTrees(tipTree, tree, false)
			if err != nil {
				return nil, err
			}
			if !sameEntries(introduced, verified) {
				hooked, err := l.runPreCommit(tree)
				if err != nil {
					return nil, err
				}
				if hooked != tree {
					// The hook took content from the shared working tree,
					// which may hold other stages' unstaged edits.
					return nil, &Error{
						Code: ExitError, Kind: "hook-modified",
						Message: "the pre-commit hook changed the staged content; nothing was committed and the staged changes are kept",
						Hint: "the files that the hook re-added may hold unstaged edits that belong to other stages; " +
							"check the working tree, stage your changes again, and commit again",
					}
				}
				verified = introduced
			}
		}
		args := []string{"commit-tree", tree}
		if tip != "" {
			args = append(args, "-p", tip)
		}
		if sign {
			args = append(args, "-S")
		}
		commit, err := r.Line(gitx.Opt{Stdin: strings.NewReader(msg)}, append(args, "-F", "-")...)
		if err != nil {
			return nil, gitErr(err, ExitError, "commit-tree-failed")
		}
		testDelay()
		fault("before-journal")
		l.M.Pending = &Pending{Commit: commit, Parent: tip}
		if err := l.save(); err != nil {
			return nil, err
		}
		fault("before-update-ref")
		testRace(r, l.M.Branch)

		var lk *indexLock
		if sync {
			if lk, err = r.lockIndex(syncTimeoutCommit); err != nil {
				l.M.Pending = nil
				_ = l.save()
				return nil, err
			}
		}
		// HEAD may have moved, or a native merge or rebase may have started,
		// since the first check.
		if err := firstErr(l.checkTarget(tip), r.checkOperation(tip, commit, sync)); err != nil {
			lk.release()
			l.M.Pending = nil
			_ = l.save()
			return nil, err
		}
		old := tip
		if old == "" {
			old = r.ZeroOID
		}
		refArgs := []string{"update-ref", "-m", reflogMessage(l.ID, res.Subject, sync)}
		if l.M.Branch == "HEAD" {
			refArgs = append(refArgs, "--no-deref")
		}
		_, err = r.Run(gitx.Opt{}, append(refArgs, l.M.Branch, commit, old)...)
		if err != nil {
			lk.release()
			l.M.Pending = nil
			if err := l.save(); err != nil {
				return nil, err
			}
			now, err2 := r.ResolveRef(l.M.Branch)
			if err2 != nil {
				return nil, err2
			}
			if now != tip {
				// Another process moved the branch first: replay onto the new tip.
				time.Sleep(time.Duration(rand.Intn(10*min(attempt, 10))+1) * time.Millisecond) //nolint:gosec // retry jitter needs no cryptographic randomness
				continue
			}
			return nil, gitErr(err, ExitError, "update-ref-failed")
		}
		fault("after-update-ref")
		if lk != nil {
			res.Sync, err = r.syncAfterCommit(tip)
			lk.release()
			if err != nil {
				res.Sync.Warnings = append(res.Sync.Warnings, fmt.Sprintf("default index not synchronized: %v; run `git multistage sync`", err))
			}
		}
		if err := l.finishCommit(commit); err != nil {
			return nil, err
		}
		// Like `git commit`, a failing post-commit hook does not undo the commit.
		_ = l.withIndexCopy(func(idx string) error { return r.runHook(idx, "post-commit") })
		res.Result, res.Commit, res.Parent = "committed", commit, tip
		return res, nil
	}
	return nil, errorf(ExitBusy, "too-many-retries",
		"the branch kept moving; gave up after %d attempts. The staged changes are kept; run the commit again", maxCommitAttempts)
}

func sameEntries(a, b []gitx.Entry) bool {
	if len(a) != len(b) || a == nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// checkTarget refuses a commit when HEAD no longer points where the stage
// commits: another branch, or, for a stage of a detached HEAD, a commit
// outside the history of the stage's base.
func (l *Stage) checkTarget(tip string) error {
	head, err := l.r.headTarget()
	if err != nil {
		return err
	}
	if head != l.M.Branch {
		return &Error{
			Code: ExitRefused, Kind: "branch-mismatch",
			Message: fmt.Sprintf("HEAD is %s, but this stage targets %s; nothing was committed", describeTarget(head), describeTarget(l.M.Branch)),
			Hint:    "check out " + strings.TrimPrefix(l.M.Branch, "refs/heads/") + " again, or drop the stage",
		}
	}
	if l.M.Branch == "HEAD" && l.M.Base != "" && tip != l.M.Base {
		if ok, err := l.r.IsAncestor(l.M.Base, tip); err != nil || !ok {
			return &Error{
				Code: ExitRefused, Kind: "branch-mismatch",
				Message: "the detached HEAD moved to a commit that does not contain this stage's base; nothing was committed",
				Hint:    "check out " + short(l.M.Base) + " or a descendant again, or drop the stage",
			}
		}
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// withIndexCopy runs f with a copy of the stage's index, so that a hook
// cannot rewrite the stage's own index (for example in split-index format).
func (l *Stage) withIndexCopy(f func(idx string) error) error {
	idx := filepath.Join(l.Dir, "hook-index")
	_ = os.Remove(idx)
	defer func() { _ = os.Remove(idx) }()
	if err := copyFile(l.IndexPath(), idx); err != nil {
		return err
	}
	return f(idx)
}

// commitMessage joins -m paragraphs as `git commit` does.
func commitMessage(parts []string, file string) (string, error) {
	if file != "" {
		var data []byte
		var err error
		if file == "-" {
			var buf bytes.Buffer
			_, err = buf.ReadFrom(os.Stdin)
			data = buf.Bytes()
		} else {
			data, err = os.ReadFile(file)
		}
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return strings.Join(parts, "\n\n"), nil
}
