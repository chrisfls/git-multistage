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
	head, err := r.SymbolicHead()
	if err != nil || head == "" {
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

// SyncIndex synchronizes the default index with the branch tip. from is the
// commit that the default index matches; "" uses the sync record.
func (r *Repo) SyncIndex(from string, timeout time.Duration) (SyncResult, error) {
	lk, err := r.lockIndex(timeout)
	if err != nil {
		return SyncResult{}, err
	}
	defer lk.release()
	head, err := r.SymbolicHead()
	if err != nil {
		return SyncResult{}, err
	}
	if from == "" {
		if from, err = r.validRecord(head); err != nil {
			return SyncResult{}, err
		}
		if from == "" {
			return SyncResult{}, &Error{Code: ExitError, Kind: "sync-unknown",
				Message: "cannot tell which commit the default index matches: it changed since the last sync",
				Hint:    "pass the commit with --from, such as --from HEAD@{1}; `git diff --cached` shows the difference"}
		}
	}
	return r.syncIndexLocked(head, from, true, false)
}

// syncAfterCommit synchronizes the default index after this process moved
// the branch from parent. The caller holds index.lock, and every agent moves
// the branch only while holding it, so the index matches parent unless
// another tool moved the branch without updating the index.
func (r *Repo) syncAfterCommit(parent string) (SyncResult, error) {
	head, err := r.SymbolicHead()
	if err != nil {
		return SyncResult{}, err
	}
	rec, err := r.validRecord(head)
	if err != nil {
		return SyncResult{}, err
	}
	from := parent
	if from == "" {
		from = r.EmptyTree
	}
	return r.syncIndexLocked(head, from, rec == parent, false)
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
	if head == "" {
		res.Warnings = append(res.Warnings, "HEAD is detached; the default index was not synchronized")
		return res, nil
	}
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
	if _, err := r.Run(gitx.Opt{Index: tmp, Stdin: strings.NewReader(input)},
		"update-index", "-z", "--index-info"); err != nil {
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
