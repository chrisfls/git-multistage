package ms

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// The default index is the worktree's ordinary .git/index. When a stage moves
// the branch with update-ref, Git does not touch the default index, so
// `git status` would show the new commit as staged reversals. SyncIndex
// brings each default-index entry that still holds the previously synced
// version up to the branch tip, under Git's own index.lock, and leaves every
// entry that someone staged natively as it is.

const (
	syncTimeoutCommit   = 15 * time.Second
	syncTimeoutRecovery = 3 * time.Second
)

// SyncResult reports what SyncIndex changed in the default index.
type SyncResult struct {
	From     string   `json:"from,omitempty"`
	To       string   `json:"to,omitempty"`
	Updated  []string `json:"updated,omitempty"`
	Kept     []string `json:"kept,omitempty"` // entries with native staged changes, left as they are
	Warnings []string `json:"warnings,omitempty"`
}

func (r *Repo) syncEnabled() bool { return r.configBool("multistage.syncIndex", true) }

func (r *Repo) syncRecordPath() string { return filepath.Join(r.Dir, "index-sync") }

// The sync record names the commit that the default index was last synced
// to, and the tree of the index right after that sync. The record is trusted
// only while the index still has that tree: any native change to the index
// entries (commit, add, rm --cached, reset) invalidates it.
type syncRecord struct{ branch, commit, tree string }

func (r *Repo) readSyncRecord() syncRecord {
	data, err := os.ReadFile(r.syncRecordPath())
	if err != nil {
		return syncRecord{}
	}
	f := strings.Fields(string(data))
	if len(f) != 3 {
		return syncRecord{}
	}
	return syncRecord{f[0], f[1], f[2]}
}

func (r *Repo) writeSyncRecord(rec syncRecord) error {
	if err := os.MkdirAll(r.Dir, 0o777); err != nil {
		return err
	}
	return writeFileAtomic(r.syncRecordPath(), []byte(rec.branch+" "+rec.commit+" "+rec.tree+"\n"))
}

// indexTree returns the tree of the index file at path without writing that
// file, or "" when the index has conflicts. A missing index is empty.
func (r *Repo) indexTree(path string) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return r.EmptyTree, nil
	}
	tmpDir := filepath.Join(r.Dir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o777); err != nil {
		return "", err
	}
	tmp := filepath.Join(tmpDir, fmt.Sprintf("tree-index-%d", os.Getpid()))
	defer os.Remove(tmp)
	if err := copyFile(path, tmp); err != nil {
		return "", err
	}
	tree, err := r.WriteTree(tmp)
	if gitx.ExitCode(err) > 0 {
		return "", nil // unmerged entries
	}
	return tree, err
}

// ensureSyncRecord records the current tip when no record exists yet.
func (r *Repo) ensureSyncRecord(tip string) error {
	if tip == "" || r.readSyncRecord().branch != "" {
		return nil
	}
	head, err := r.headTarget()
	if err != nil {
		return err
	}
	tree, err := r.indexTree(r.IndexPath)
	if err != nil || tree == "" {
		return err
	}
	return r.writeSyncRecord(syncRecord{head, tip, tree})
}

// indexLock is Git's index.lock, created by this process.
type indexLock struct {
	path string
	f    *os.File
}

func (r *Repo) lockIndex(timeout time.Duration) (*indexLock, error) {
	path := r.IndexPath + ".lock"
	deadline := time.Now().Add(timeout)
	delay := 5 * time.Millisecond
	for {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		if err == nil {
			return &indexLock{path: path, f: f}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, &Error{Code: ExitBusy, Kind: "index-locked",
				Message: fmt.Sprintf("the default index is locked: %s exists", path),
				Hint:    "another Git process is running; if none is, delete the lock file and retry"}
		}
		time.Sleep(delay)
		if delay < 100*time.Millisecond {
			delay *= 2
		}
	}
}

// release removes the lock once; later calls do nothing, so a second call
// can never delete a lock that another process took meanwhile.
func (k *indexLock) release() {
	if k == nil || k.f == nil {
		return
	}
	k.f.Close()
	k.f = nil
	os.Remove(k.path)
}

// SyncIndex synchronizes the default index with the tip of HEAD. from is the
// commit that the default index matches; "" uses the sync record, or else the
// newest ancestor of the tip whose tree equals the index.
func (r *Repo) SyncIndex(from string, timeout time.Duration) (SyncResult, error) {
	lk, err := r.lockIndex(timeout)
	if err != nil {
		return SyncResult{}, err
	}
	defer lk.release()
	head, err := r.headTarget()
	if err != nil {
		return SyncResult{}, err
	}
	if from == "" {
		if from, err = r.validRecord(head); err != nil {
			return SyncResult{}, err
		}
	}
	if from == "" {
		if from, err = r.matchingAncestor(head); err != nil {
			return SyncResult{}, err
		}
	}
	if from == "" {
		hint := "pass the commit that the index matches with --from; `git diff --cached` shows the difference"
		if c, n := r.nearestAncestor(head); c != "" {
			hint = fmt.Sprintf("the closest commit is %s, which differs from the index at %d path(s): pass --from %s if the index lags behind it", short(c), n, short(c))
		}
		return SyncResult{}, &Error{Code: ExitError, Kind: "sync-unknown",
			Message: "cannot tell which commit the default index matches: it holds changes that no commit of " + head + " has",
			Hint:    hint}
	}
	return r.syncIndexLocked(head, from, true, false)
}

// syncAfterCommit synchronizes the default index after this process moved
// the branch from parent. The caller holds index.lock, and every stage moves
// the branch only while holding it, so the index matches parent, unless a
// tool moved the branch without updating the index. That lag is caught up
// only when it is certain: the index equals an older commit exactly, and the
// reflog shows that only index-free updates happened since.
func (r *Repo) syncAfterCommit(parent string) (SyncResult, error) {
	head, err := r.headTarget()
	if err != nil {
		return SyncResult{}, err
	}
	rec, err := r.validRecord(head)
	if err != nil {
		return SyncResult{}, err
	}
	if parent == "" {
		return r.syncIndexLocked(head, r.EmptyTree, true, false)
	}
	from, keep := parent, rec == parent
	if !keep {
		current, err := r.indexTree(r.IndexPath)
		if err != nil {
			return SyncResult{}, err
		}
		parentTree, err := r.TreeOf(parent)
		if err != nil {
			return SyncResult{}, err
		}
		keep = current == parentTree
		if current != "" && current != parentTree {
			if c, _ := r.ancestorWithTree(parent, current); c != "" && r.lagOnly(head, parent, c) {
				from, keep = c, true
			}
		}
	}
	return r.syncIndexLocked(head, from, keep, false)
}

// matchingAncestor returns the newest commit of head's first-parent history
// whose tree equals the default index, or "".
func (r *Repo) matchingAncestor(head string) (string, error) {
	tip, err := r.ResolveRef(head)
	if err != nil || tip == "" {
		return "", err
	}
	current, err := r.indexTree(r.IndexPath)
	if err != nil || current == "" {
		return "", err
	}
	return r.ancestorWithTree(tip, current)
}

const historyScan = 200

// ancestorWithTree returns the newest of the first historyScan first-parent
// ancestors of tip (tip included) whose tree is tree.
func (r *Repo) ancestorWithTree(tip, tree string) (string, error) {
	out, err := r.Run(gitx.Opt{}, "rev-list", "--first-parent", fmt.Sprintf("--max-count=%d", historyScan), "--format=%T", tip)
	if err != nil {
		return "", err
	}
	f := strings.Fields(string(out)) // commit <oid> <tree> ...
	for i := 0; i+2 < len(f); i += 3 {
		if f[i+2] == tree {
			return f[i+1], nil
		}
	}
	return "", nil
}

// nearestAncestor returns the recent first-parent ancestor of head whose tree
// differs from the default index at the fewest paths.
func (r *Repo) nearestAncestor(head string) (string, int) {
	out, err := r.Run(gitx.Opt{}, "rev-list", "--first-parent", "--max-count=50", head)
	if err != nil {
		return "", 0
	}
	best, bestN := "", -1
	for _, c := range strings.Fields(string(out)) {
		diff, err := r.Run(gitx.Opt{Index: r.IndexPath}, "diff-index", "--cached", "--name-only", "-z", c)
		if err != nil {
			continue
		}
		if n := strings.Count(string(diff), "\x00"); bestN < 0 || n < bestN {
			best, bestN = c, n
		}
	}
	return best, bestN
}

// lagOnly reports whether every update of ref from older (exclusive) up to
// newer (inclusive) was one that leaves the default index alone: an update-ref
// without a reflog message, or a stage commit made with multistage.syncIndex
// off.
func (r *Repo) lagOnly(ref, newer, older string) bool {
	out, err := r.Run(gitx.Opt{}, "reflog", "show", "--format=%H%x00%gs", fmt.Sprintf("-n%d", historyScan), ref, "--")
	if err != nil {
		return false
	}
	started := false
	for _, line := range strings.Split(string(out), "\n") {
		oid, msg, ok := strings.Cut(line, "\x00")
		if !ok {
			continue
		}
		if !started {
			started = oid == newer
		}
		if !started {
			continue
		}
		if oid == older {
			return true
		}
		if msg != "" && !notSyncedReflog(msg) {
			return false
		}
	}
	return false
}

// reflogMessage is the reflog subject of a stage commit. A commit made with
// multistage.syncIndex off says so, which lets lagOnly recognize it.
func reflogMessage(stage, subject string, synced bool) string {
	if synced {
		return fmt.Sprintf("commit (multistage %s): %s", stage, subject)
	}
	return fmt.Sprintf("commit (multistage %s, index not synced): %s", stage, subject)
}

func notSyncedReflog(msg string) bool {
	head, _, ok := strings.Cut(msg, "): ")
	return ok && strings.HasPrefix(head, "commit (multistage ") && strings.HasSuffix(head, ", index not synced")
}

// headTarget returns the branch that HEAD points to, or "HEAD" when HEAD is
// detached.
func (r *Repo) headTarget() (string, error) {
	b, err := r.SymbolicHead()
	if err != nil || b != "" {
		return b, err
	}
	return "HEAD", nil
}

// validRecord returns the commit of the sync record when the record belongs
// to head and the default index still has the recorded tree.
func (r *Repo) validRecord(head string) (string, error) {
	rec := r.readSyncRecord()
	if rec.branch == "" || rec.branch != head {
		return "", nil
	}
	current, err := r.indexTree(r.IndexPath)
	if err != nil || current != rec.tree {
		return "", err
	}
	return rec.commit, nil
}

// syncIndexLocked brings each default-index entry that holds its version in
// from to its version in the tip of head. When keepRecord is set, the result
// becomes the new sync record; otherwise the record is deleted, because the
// index may still lag behind changes before from. The caller holds
// index.lock, unless dryRun is set: then nothing is written.
func (r *Repo) syncIndexLocked(head, from string, keepRecord, dryRun bool) (SyncResult, error) {
	var res SyncResult
	tip, err := r.ResolveRef(head)
	if err != nil || tip == "" {
		return res, err
	}
	if from != r.EmptyTree {
		if ok, _ := r.IsAncestor(from, tip); !ok {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s is not an ancestor of %s; the default index was not synchronized", short(from), head))
			return res, nil
		}
	}
	res.From, res.To = from, tip
	changes, err := r.DiffTrees(from, tip, false)
	if err != nil {
		return res, err
	}
	tree, err := r.applyIndexChanges(changes, &res, dryRun)
	if err != nil || dryRun {
		return res, err
	}
	if !keepRecord {
		os.Remove(r.syncRecordPath())
		return res, nil
	}
	if tree == "" {
		if tree, err = r.indexTree(r.IndexPath); err != nil || tree == "" {
			return res, err
		}
	}
	return res, r.writeSyncRecord(syncRecord{head, tip, tree})
}

type stagedEntry struct{ mode, oid string }

// applyIndexChanges rewrites a copy of the default index and renames it into
// place. The caller holds index.lock, so no Git process writes the index
// meanwhile.
// It returns the tree of the new index, or "" when it wrote nothing.
func (r *Repo) applyIndexChanges(changes []gitx.Entry, res *SyncResult, dryRun bool) (string, error) {
	if len(changes) == 0 {
		return "", nil
	}
	// Git treats a missing index as an empty one.
	info, err := os.Stat(r.IndexPath)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return "", err
	}
	if dryRun {
		_, err := r.planIndexChanges(r.IndexPath, changes, res)
		return "", err
	}
	tmpDir := filepath.Join(r.Dir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o777); err != nil {
		return "", err
	}
	tmp := filepath.Join(tmpDir, fmt.Sprintf("index-%d", os.Getpid()))
	os.Remove(tmp + ".lock")
	os.Remove(tmp)
	defer os.Remove(tmp)
	if !missing {
		if err := copyFile(r.IndexPath, tmp); err != nil {
			return "", err
		}
		// Keep the original timestamp so that Git's racy-entry detection
		// treats the copy exactly like the original.
		if err := os.Chtimes(tmp, info.ModTime(), info.ModTime()); err != nil {
			return "", err
		}
	}

	input, err := r.planIndexChanges(tmp, changes, res)
	if err != nil {
		return "", err
	}
	if input == "" {
		return "", nil
	}
	if _, err := r.Run(gitx.Opt{Index: tmp, KeepFormat: true, Stdin: strings.NewReader(input)},
		"update-index", "-z", "--index-info"); err != nil {
		return "", err
	}
	// --index-info drops the skip-worktree bit of the entries it replaces.
	if err := r.copySkipWorktree(tmp, res.Updated, true); err != nil {
		return "", err
	}
	tree, err := r.indexTree(tmp)
	if err != nil {
		return "", err
	}
	return tree, rename(tmp, r.IndexPath)
}

// planIndexChanges reads the entries of the changed paths from index and
// returns the --index-info input that brings each unmodified entry to the tip.
func (r *Repo) planIndexChanges(index string, changes []gitx.Entry, res *SyncResult) (string, error) {
	paths := make([]string, len(changes))
	for i, c := range changes {
		paths[i] = c.Path
	}
	current := map[string]stagedEntry{}
	conflicted := map[string]bool{}
	for start := 0; start < len(paths); start += 200 {
		end := min(start+200, len(paths))
		out, err := r.Run(gitx.Opt{Index: index}, append([]string{"ls-files", "-s", "-z", "--"}, paths[start:end]...)...)
		if err != nil {
			return "", err
		}
		for _, rec := range strings.Split(string(out), "\x00") {
			meta, path, ok := strings.Cut(rec, "\t")
			if !ok {
				continue
			}
			f := strings.Fields(meta)
			if len(f) != 3 {
				continue
			}
			if f[2] != "0" {
				conflicted[path] = true
				continue
			}
			current[path] = stagedEntry{f[0], f[1]}
		}
	}

	var input strings.Builder
	for _, c := range changes {
		cur, present := current[c.Path]
		oldAbsent, newAbsent := c.OldMode == "000000", c.NewMode == "000000"
		switch {
		case conflicted[c.Path]:
			res.Kept = append(res.Kept, c.Path)
		case newAbsent && !present, present && cur == (stagedEntry{c.NewMode, c.NewOID}):
			// Already matches the tip.
		case oldAbsent && !present, present && cur == (stagedEntry{c.OldMode, c.OldOID}):
			if newAbsent {
				fmt.Fprintf(&input, "0 %s\t%s\x00", r.ZeroOID, c.Path)
			} else {
				fmt.Fprintf(&input, "%s %s\t%s\x00", c.NewMode, c.NewOID, c.Path)
			}
			res.Updated = append(res.Updated, c.Path)
		default:
			res.Kept = append(res.Kept, c.Path)
		}
	}
	if len(res.Kept) > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"the default index has natively staged changes that differ from %s and were kept: %s",
			short(res.To), strings.Join(res.Kept, ", ")))
	}
	return input.String(), nil
}

func copyFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}
