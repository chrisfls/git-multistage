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

// readSyncRecord returns the branch and commit that the default index was last synced to.
func (r *Repo) readSyncRecord() (branch, commit string) {
	data, err := os.ReadFile(r.syncRecordPath())
	if err != nil {
		return "", ""
	}
	f := strings.Fields(string(data))
	if len(f) != 2 {
		return "", ""
	}
	return f[0], f[1]
}

func (r *Repo) writeSyncRecord(branch, commit string) error {
	if err := os.MkdirAll(r.Dir, 0o777); err != nil {
		return err
	}
	return writeFileAtomic(r.syncRecordPath(), []byte(branch+" "+commit+"\n"))
}

// ensureSyncRecord records the current tip when no record exists yet, so that
// the first stage commit knows which version the default index holds.
func (r *Repo) ensureSyncRecord(tip string) error {
	if tip == "" {
		return nil
	}
	if b, _ := r.readSyncRecord(); b != "" {
		return nil
	}
	head, err := r.SymbolicHead()
	if err != nil || head == "" {
		return err
	}
	return r.writeSyncRecord(head, tip)
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

func (k *indexLock) release() {
	if k == nil {
		return
	}
	k.f.Close()
	os.Remove(k.path)
}

// SyncIndex synchronizes the default index with the branch tip. fallback is
// a commit that the default index is known to match when the sync record is
// missing or unusable.
func (r *Repo) SyncIndex(fallback string, timeout time.Duration) (SyncResult, error) {
	lk, err := r.lockIndex(timeout)
	if err != nil {
		return SyncResult{}, err
	}
	defer lk.release()
	return r.syncIndexLocked(fallback, false)
}

// syncIndexLocked does the work of SyncIndex. The caller holds index.lock,
// unless dryRun is set: then nothing is written.
func (r *Repo) syncIndexLocked(fallback string, dryRun bool) (SyncResult, error) {
	var res SyncResult
	head, err := r.SymbolicHead()
	if err != nil {
		return res, err
	}
	if head == "" {
		res.Warnings = append(res.Warnings, "HEAD is detached; the default index was not synchronized")
		return res, nil
	}
	tip, err := r.ResolveRef(head)
	if err != nil || tip == "" {
		return res, err
	}
	from := ""
	if b, c := r.readSyncRecord(); b == head {
		if ok, _ := r.IsAncestor(c, tip); ok {
			from = c
		}
	}
	if from == "" {
		if fallback == r.EmptyTree {
			from = fallback // the branch was unborn before this commit
		} else if ok, _ := r.IsAncestor(fallback, tip); ok {
			from = fallback
		}
	}
	if from == "" {
		res.Warnings = append(res.Warnings, "cannot tell which commit the default index matches; it was not synchronized")
		return res, nil
	}
	res.From, res.To = from, tip
	if from == tip {
		return res, nil
	}
	changes, err := r.DiffTrees(from, tip, false)
	if err != nil {
		return res, err
	}
	if len(changes) > 0 {
		if err := r.applyIndexChanges(changes, &res, dryRun); err != nil {
			return res, err
		}
	}
	if dryRun {
		return res, nil
	}
	return res, r.writeSyncRecord(head, tip)
}

type stagedEntry struct{ mode, oid string }

// applyIndexChanges rewrites a copy of the default index and renames it into
// place. The caller holds index.lock, so no Git process writes the index
// meanwhile.
func (r *Repo) applyIndexChanges(changes []gitx.Entry, res *SyncResult, dryRun bool) error {
	// Git treats a missing index as an empty one.
	info, err := os.Stat(r.IndexPath)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return err
	}
	if dryRun {
		_, err := r.planIndexChanges(r.IndexPath, changes, res)
		return err
	}
	tmpDir := filepath.Join(r.Dir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o777); err != nil {
		return err
	}
	tmp := filepath.Join(tmpDir, fmt.Sprintf("index-%d", os.Getpid()))
	os.Remove(tmp + ".lock")
	os.Remove(tmp)
	defer os.Remove(tmp)
	if !missing {
		if err := copyFile(r.IndexPath, tmp); err != nil {
			return err
		}
		// Keep the original timestamp so that Git's racy-entry detection
		// treats the copy exactly like the original.
		if err := os.Chtimes(tmp, info.ModTime(), info.ModTime()); err != nil {
			return err
		}
	}

	input, err := r.planIndexChanges(tmp, changes, res)
	if err != nil {
		return err
	}
	if input == "" {
		return nil
	}
	if _, err := r.Run(gitx.Opt{Index: tmp, Stdin: strings.NewReader(input)},
		"update-index", "-z", "--index-info"); err != nil {
		return err
	}
	return rename(tmp, r.IndexPath)
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
