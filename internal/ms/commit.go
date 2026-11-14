package ms

import (
	"bytes"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chrisfls/git-multistage/internal/filelock"
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
			Paths:   res.Kept, Hint: forAgent("finish or abort "+op+", then commit again", hintStop),
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
	data := []byte(raw)
	if l.r.HasHook("prepare-commit-msg") {
		if err := os.WriteFile(file, data, 0o666); err != nil {
			return "", err
		}
		if err := l.withIndexCopy(func(idx string) error { return l.r.runHook(idx, "prepare-commit-msg", file, "message") }); err != nil {
			return "", &Error{Code: ExitError, Kind: "hook-failed", Message: "the prepare-commit-msg hook failed; nothing was committed"}
		}
		var err error
		if data, err = os.ReadFile(file); err != nil {
			return "", err
		}
	}
	msg := cleanMessage(string(data))
	// Without a commit-msg hook the message stays clean.
	if !noVerify && l.r.HasHook("commit-msg") {
		if err := os.WriteFile(file, []byte(msg), 0o666); err != nil {
			return "", err
		}
		if err := l.withIndexCopy(func(idx string) error { return l.r.runHook(idx, "commit-msg", file) }); err != nil {
			return "", &Error{Code: ExitError, Kind: "hook-failed", Message: "the commit-msg hook failed; nothing was committed and the staged changes are kept"}
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		msg = cleanMessage(string(data))
	}
	if strings.TrimSpace(msg) == "" {
		return "", &Error{Code: ExitError, Kind: "empty-message", Message: "the commit message is empty"}
	}
	return msg, nil
}

// cleanMessage cleans a commit message as `git stripspace` does. Each line
// loses its trailing white space, which Git takes to be spaces, tabs, CRs,
// and LFs. Empty lines at the start and at the end go, a run of empty lines
// inside becomes one, and each line ends with LF.
func cleanMessage(s string) string {
	var b strings.Builder
	empties := 0
	for s != "" {
		var line string
		line, s, _ = strings.Cut(s, "\n")
		if line = strings.TrimRight(line, " \t\r"); line == "" {
			empties++
			continue
		}
		if empties > 0 && b.Len() > 0 {
			b.WriteByte('\n')
		}
		empties = 0
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// addMoveTrailers ends msg with one Multistage-Moved trailer for each
// staged move onto tip. A move that recreate names, and whose old path tip
// no longer holds, is committed as a new file and gets no trailer.
func (l *Stage) addMoveTrailers(msg, tip string, recreate []string) (string, error) {
	var trailers []string
	for _, m := range l.M.Moves {
		if slices.Contains(recreate, m.Old) || slices.Contains(recreate, m.New) {
			_, onTip, err := l.r.lsTree(tip, m.Old)
			if err != nil {
				return "", err
			}
			if !onTip {
				continue
			}
		}
		trailers = append(trailers, "--trailer", trailerMoved+": "+moveTrailer(m))
	}
	if len(trailers) == 0 {
		return msg, nil
	}
	args := append([]string{"interpret-trailers", "--where=end", "--if-exists=addIfDifferent"}, trailers...)
	return stringOut(l.r.Run(gitx.Opt{Stdin: strings.NewReader(msg)}, args...))
}

func stringOut(b []byte, err error) (string, error) { return string(b), err }

// hookIndexPath returns a new name for an index that a Git hook may write.
// Each run has its own name, so a lock file that a hook orphaned by a killed
// process still holds never blocks or meets a later run.
func (l *Stage) hookIndexPath() string {
	b := make([]byte, 4)
	_, _ = crand.Read(b)
	return filepath.Join(l.Dir, "hook-index-"+hex.EncodeToString(b))
}

// runPreCommit runs the pre-commit hook against an index that holds tree,
// and returns the tree that the hook leaves in that index.
func (l *Stage) runPreCommit(tree string) (string, error) {
	idx := l.hookIndexPath()
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
// Each path of recreate that a commit on the branch deleted is committed as
// a new file.
func (l *Stage) Commit(rawMsg string, noVerify bool, recreate []string) (*CommitResult, error) {
	res := &CommitResult{Branch: l.M.Branch}
	st, err := l.prepareCommit(res, rawMsg, noVerify)
	if err != nil {
		return nil, err
	}
	st.recreate = recreate
	for attempt := 1; attempt <= maxCommitAttempts; attempt++ {
		res.Attempts = attempt
		done, err := l.commitAttempt(res, st, attempt)
		if err != nil {
			return nil, err
		}
		if done {
			return res, nil
		}
	}
	return nil, errorf(ExitBusy, "too-many-retries",
		"the branch kept moving; gave up after %d attempts. The staged changes are kept; run the commit again", maxCommitAttempts)
}

// commitState holds what each attempt of a commit needs.
type commitState struct {
	staged, baseTree, msg string
	noVerify, sign, sync  bool
	recreate              []string
	verified              []gitx.Entry   // changes that the pre-commit hook accepted
	turn                  *filelock.Lock // the commit turn of this attempt, or nil
}

// commitTurnTimeout bounds the wait for the commit turn.
const commitTurnTimeout = 10 * time.Second

// takeCommitTurn waits for the commit turn of the worktree. Commits hold it
// from the read of the tip to the update of the branch, so a commit that
// waits for it lands without a replay. After the timeout, or when the turn
// cannot be taken, it returns nil and the commit goes on without the turn:
// the compare-and-swap of the branch still decides which commit lands.
func (r *Repo) takeCommitTurn() *filelock.Lock {
	lk, err := filelock.Acquire(filepath.Join(r.Dir, "commit.lock"), testLockTimeout(commitTurnTimeout))
	if err != nil {
		return nil
	}
	return lk
}

// prepareCommit checks that the stage can commit, and reads its staged
// tree, its changes, and the message.
func (l *Stage) prepareCommit(res *CommitResult, rawMsg string, noVerify bool) (*commitState, error) {
	r := l.r
	tip0, head, err := r.tipAndTarget(l.M.Branch)
	if err != nil {
		return nil, err
	}
	if err := l.checkHead(head, tip0); err != nil {
		return nil, err
	}
	if op, blocking := r.inProgress(); blocking {
		return nil, errorf(ExitRefused, "operation-in-progress", "%s is in progress in this worktree; finish or abort it before committing", op)
	}
	st := &commitState{noVerify: noVerify}
	if st.staged, err = r.WriteTree(l.IndexPath()); err != nil {
		return nil, err
	}
	if st.baseTree, err = r.TreeOf(l.M.Base); err != nil {
		return nil, err
	}
	if st.staged == st.baseTree {
		return nil, &Error{Code: ExitError, Kind: "nothing-staged", Message: "nothing is staged for " + l.ID}
	}
	if res.Files, err = r.stagedFiles(st.baseTree, st.staged); err != nil {
		return nil, err
	}
	if st.msg, err = l.prepareMessage(rawMsg, noVerify); err != nil {
		return nil, err
	}
	res.Subject, _, _ = strings.Cut(st.msg, "\n")
	st.sign = r.ConfigBool("commit.gpgSign", false)
	st.sync = r.syncEnabled()
	return st, nil
}

// stagedFiles lists the changes from baseTree to the staged tree with renames,
// from the tree diff that the sync after the commit reuses.
func (r *Repo) stagedFiles(baseTree, staged string) ([]Change, error) {
	entries, err := r.DiffTrees(baseTree, staged, true)
	if err != nil {
		return nil, err
	}
	var files []Change
	for _, e := range entries {
		files = append(files, Change{Status: string(e.Status), Path: e.Path, OldPath: e.OldPath})
	}
	return files, nil
}

// commitAttempt replays the staged changes onto the branch's current tip
// and tries to move the branch to the new commit. It reports false when
// another process moved the branch first, so the commit must be tried again.
func (l *Stage) commitAttempt(res *CommitResult, st *commitState, attempt int) (bool, error) {
	r := l.r
	st.turn = r.takeCommitTurn()
	// verifyTree may take the turn again after the pre-commit hook.
	defer func() { st.turn.Release() }()
	tip, err := r.ResolveRef(l.M.Branch)
	if err != nil {
		return false, err
	}
	testPause("after-turn-tip")
	if tip == "" && l.M.Base != "" {
		return false, errorf(ExitRefused, "branch-missing", "branch %s no longer exists; nothing was committed", l.M.Branch)
	}
	tree, err := r.integrate(l.M.Base, st.baseTree, st.staged, tip, l.M.Moves, st.recreate)
	if err != nil {
		return false, err
	}
	tipTree, err := r.TreeOf(tip)
	if err != nil {
		return false, err
	}
	if tree == tipTree {
		// Every staged change is already on the branch.
		if err := l.setIndex(tip); err != nil {
			return false, err
		}
		res.Result, res.Parent = "already-applied", tip
		return true, nil
	}
	if !st.noVerify {
		if onTip, err := l.verifyTree(st, tip, tipTree, tree); err != nil || !onTip {
			return false, err
		}
	}
	msg, err := l.addMoveTrailers(st.msg, tip, st.recreate)
	if err != nil {
		return false, err
	}
	commit, err := r.commitTree(tree, tip, msg, st.sign)
	if err != nil {
		return false, err
	}
	return l.land(res, st, commit, tip, attempt)
}

// verifyTree runs the pre-commit hook on tree. A retry that introduces
// exactly the changes that the hook already accepted does not run the hook
// again, so a slow hook cannot starve the commit while other stages commit.
// The hook runs without the commit turn, so that it never holds the commits
// of other stages. verifyTree then takes the turn again, and reports false
// when another commit moved the branch from tip meanwhile: the next attempt
// replays and finds the changes that the hook accepted.
func (l *Stage) verifyTree(st *commitState, tip, tipTree, tree string) (bool, error) {
	if !l.r.HasHook("pre-commit") {
		// A fresh index that holds tree writes tree back.
		return true, nil
	}
	introduced, err := l.r.DiffTrees(tipTree, tree, false)
	if err != nil {
		return false, err
	}
	if sameEntries(introduced, st.verified) {
		return true, nil
	}
	st.turn.Release()
	hooked, err := l.runPreCommit(tree)
	if err != nil {
		return false, err
	}
	if hooked != tree {
		// The hook took content from the shared working tree,
		// which may hold other stages' unstaged edits.
		return false, &Error{
			Code: ExitError, Kind: "hook-modified",
			Message: "the pre-commit hook changed the staged content; nothing was committed and the staged changes are kept",
			Hint: forAgent("the files that the hook re-added may hold unstaged edits that belong to other stages; "+
				"check the working tree, stage your changes again, and commit again",
				"the hook rewrote files on disk, such as by formatting them; stage your changes again "+
					"with `git multistage hunks <path>` and `add <path> --hunk <id>`, and commit again"),
		}
	}
	st.verified = introduced
	st.turn = l.r.takeCommitTurn()
	now, err := l.r.ResolveRef(l.M.Branch)
	return err == nil && now == tip, err
}

// commitTree writes the commit object of tree on top of tip.
func (r *Repo) commitTree(tree, tip, msg string, sign bool) (string, error) {
	args := []string{"commit-tree", tree}
	if tip != "" {
		args = append(args, "-p", tip)
	}
	if sign {
		args = append(args, "-S")
	}
	commit, err := r.Line(gitx.Opt{Stdin: strings.NewReader(msg)}, append(args, "-F", "-")...)
	if err != nil {
		return "", gitErr(err, ExitError, "commit-tree-failed")
	}
	r.RememberCommit(commit, tree, tip)
	return commit, nil
}

// land journals commit, moves the branch from tip to it by compare-and-swap,
// syncs the default index, and finishes the commit. It reports false when
// another process moved the branch first.
func (l *Stage) land(res *CommitResult, st *commitState, commit, tip string, attempt int) (bool, error) {
	r := l.r
	testDelay()
	fault("before-journal")
	l.M.Pending = &Pending{Commit: commit, Parent: tip}
	if err := l.save(); err != nil {
		return false, err
	}
	fault("before-update-ref")
	testRace(r, l.M.Branch)

	var lk *indexLock
	if st.sync {
		var err error
		if lk, err = r.lockIndex(syncTimeoutCommit); err != nil {
			l.M.Pending = nil
			_ = l.save()
			return false, err
		}
	}
	// HEAD may have moved, or a native merge or rebase may have started,
	// since the first check.
	if err := firstErr(l.checkTarget(tip), r.checkOperation(tip, commit, st.sync)); err != nil {
		lk.release()
		l.M.Pending = nil
		_ = l.save()
		return false, err
	}
	err := l.updateRef(res, commit, tip, st.sync)
	// The sync runs after the turn, under index.lock, which the next commit
	// takes before its update-ref.
	st.turn.Release()
	if err != nil {
		lk.release()
		return l.afterFailedUpdate(err, tip, attempt)
	}
	fault("after-update-ref")
	if lk != nil {
		var err error
		res.Sync, err = r.syncLanded(l.M.Branch, commit, tip)
		lk.release()
		if err != nil {
			res.Sync.Warnings = append(res.Sync.Warnings, fmt.Sprintf("default index not synchronized: %v; run `git multistage sync`", err))
		}
	}
	if err := l.finishCommit(commit); err != nil {
		return false, err
	}
	// Like `git commit`, a failing post-commit hook does not undo the commit.
	if r.HasHook("post-commit") {
		_ = l.withIndexCopy(func(idx string) error { return r.runHook(idx, "post-commit") })
	}
	res.Result, res.Commit, res.Parent = "committed", commit, tip
	return true, nil
}

// updateRef moves the stage's branch from tip to commit, only if the branch
// is still at tip.
func (l *Stage) updateRef(res *CommitResult, commit, tip string, sync bool) error {
	old := tip
	if old == "" {
		old = l.r.ZeroOID
	}
	refArgs := []string{"update-ref", "-m", reflogMessage(l.ID, res.Subject, sync)}
	if l.M.Branch == refHEAD {
		refArgs = append(refArgs, "--no-deref")
	}
	_, err := l.r.Run(gitx.Opt{}, append(refArgs, l.M.Branch, commit, old)...)
	return err
}

// afterFailedUpdate clears the journal of a commit whose update-ref failed.
// It reports false, to try again, when another process moved the branch
// from tip first; otherwise it returns the failure.
func (l *Stage) afterFailedUpdate(updateErr error, tip string, attempt int) (bool, error) {
	l.M.Pending = nil
	if err := l.save(); err != nil {
		return false, err
	}
	now, err := l.r.ResolveRef(l.M.Branch)
	if err != nil {
		return false, err
	}
	if now != tip {
		// Another process moved the branch first: replay onto the new tip.
		time.Sleep(time.Duration(rand.Intn(10*min(attempt, 10))+1) * time.Millisecond) //nolint:gosec // retry jitter needs no cryptographic randomness
		return false, nil
	}
	return false, gitErr(updateErr, ExitError, "update-ref-failed")
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
	return l.checkHead(head, tip)
}

// checkHead is checkTarget for a known target of HEAD.
func (l *Stage) checkHead(head, tip string) error {
	if head != l.M.Branch {
		return &Error{
			Code: ExitRefused, Kind: "branch-mismatch",
			Message: fmt.Sprintf("HEAD is %s, but this stage targets %s; nothing was committed", describeTarget(head), describeTarget(l.M.Branch)),
			Hint:    forAgent("check out "+strings.TrimPrefix(l.M.Branch, "refs/heads/")+" again, or drop the stage", hintStop),
		}
	}
	if l.M.Branch == refHEAD && l.M.Base != "" && tip != l.M.Base {
		if ok, err := l.r.IsAncestor(l.M.Base, tip); err != nil || !ok {
			return &Error{
				Code: ExitRefused, Kind: "branch-mismatch",
				Message: "the detached HEAD moved to a commit that does not contain this stage's base; nothing was committed",
				Hint:    forAgent("check out "+short(l.M.Base)+" or a descendant again, or drop the stage", hintStop),
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
	idx := l.hookIndexPath()
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
