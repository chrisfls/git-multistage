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
func (r *Repo) inProgress() string {
	for _, f := range []struct{ name, op string }{
		{"MERGE_HEAD", "a merge"}, {"CHERRY_PICK_HEAD", "a cherry-pick"},
		{"REVERT_HEAD", "a revert"}, {"rebase-merge", "a rebase"}, {"rebase-apply", "a rebase or am"},
	} {
		if _, err := os.Stat(filepath.Join(r.GitDir, f.name)); err == nil {
			return f.op
		}
	}
	return ""
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
	if err := l.r.runHook(l.IndexPath(), "prepare-commit-msg", file, "message"); err != nil {
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
		if err := l.r.runHook(l.IndexPath(), "commit-msg", file); err != nil {
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
	os.Remove(idx)
	defer os.Remove(idx)
	if _, err := l.r.Run(gitx.Opt{Index: idx}, "read-tree", tree); err != nil {
		return "", err
	}
	if err := l.r.runHook(idx, "pre-commit"); err != nil {
		return "", &Error{Code: ExitError, Kind: "hook-failed",
			Message: "the pre-commit hook failed; nothing was committed and the staged changes are kept"}
	}
	return l.r.WriteTree(idx)
}

// Commit commits the stage's staged changes onto the current tip of its
// branch. Other stages or tools may move the branch at any time: the staged change is
// replayed onto the newest tip and the branch moves only by compare-and-swap.
func (l *Stage) Commit(rawMsg string, noVerify bool) (*CommitResult, error) {
	r := l.r
	res := &CommitResult{Branch: l.M.Branch}
	head, err := r.SymbolicHead()
	if err != nil {
		return nil, err
	}
	if head != l.M.Branch {
		return nil, &Error{Code: ExitRefused, Kind: "branch-mismatch",
			Message: fmt.Sprintf("HEAD is on %s, but this stage targets %s; nothing was committed", orDetached(head), l.M.Branch),
			Hint:    "check out " + strings.TrimPrefix(l.M.Branch, "refs/heads/") + " again, or drop the stage"}
	}
	if op := r.inProgress(); op != "" {
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
					return nil, &Error{Code: ExitError, Kind: "hook-modified",
						Message: "the pre-commit hook changed the staged content; nothing was committed and the staged changes are kept",
						Hint: "the files that the hook re-added may hold unstaged edits that belong to other stages; " +
							"check the working tree, stage your changes again, and commit again"}
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
		// A native merge or rebase may have started since the first check.
		if op := r.inProgress(); op != "" {
			lk.release()
			l.M.Pending = nil
			_ = l.save()
			return nil, errorf(ExitRefused, "operation-in-progress", "%s started in this worktree; nothing was committed", op)
		}
		old := tip
		if old == "" {
			old = r.ZeroOID
		}
		reflog := fmt.Sprintf("commit (multistage %s): %s", l.ID, res.Subject)
		_, err = r.Run(gitx.Opt{}, "update-ref", "-m", reflog, l.M.Branch, commit, old)
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
				time.Sleep(time.Duration(rand.Intn(10*min(attempt, 10))+1) * time.Millisecond)
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
		_ = r.runHook(l.IndexPath(), "post-commit")
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
