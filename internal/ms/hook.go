package ms

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chrisfls/git-multistage/internal/filelock"
	"github.com/chrisfls/git-multistage/internal/gitx"
)

// `git multistage hook claude` runs as a Claude Code PreToolUse and
// PostToolUse hook on the file tools. Before an edit it takes a short lease
// on the file, so that two hooked agents never write one file at the same
// moment, and snapshots the file. After the edit it records the before and
// after content in the editing agent's stage. A hook error never blocks the
// edit; only a lease that another stage keeps holding does.

const (
	leaseTTL     = 15 * time.Second
	leaseTimeout = 20 * time.Second
)

// hookInput is the part of Claude Code's hook input that the hook reads.
type hookInput struct {
	Event     string `json:"hook_event_name"`
	Tool      string `json:"tool_name"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"` // set inside a subagent
	ToolUseID string `json:"tool_use_id"`
	Input     struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

type pendingEdit struct {
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	Nonce  string `json:"nonce"`
}

type lease struct {
	Stage   string `json:"stage"`
	Nonce   string `json:"nonce"`
	Expires int64  `json:"expires"` // Unix milliseconds
}

// runEditHook handles one hook call. It returns the exit code for Claude
// Code: 0 lets the edit proceed, 2 blocks it with the stderr message.
func runEditHook(stdin io.Reader, stdout, stderr io.Writer, stageFlag string) int {
	var in hookInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: cannot read the hook input: %v\n", err)
		return 0
	}
	switch in.Tool {
	case "Edit", "MultiEdit", "Write":
	default:
		return 0
	}
	if in.Input.FilePath == "" {
		return 0
	}
	id := stageFlag
	if id == "" {
		id = os.Getenv("GIT_MULTISTAGE_ID")
	}
	if id == "" {
		owner := in.AgentID
		if owner == "" {
			owner = in.SessionID
		}
		if owner == "" {
			return 0
		}
		id = sanitizeID("claude-" + owner)
	}
	h, err := newEditHook(in, id)
	if err != nil || h == nil {
		if err != nil {
			fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		}
		return 0
	}
	switch in.Event {
	case "PreToolUse":
		return h.before(stderr)
	case "PostToolUse":
		h.after(stdout, stderr, true)
	case "PostToolUseFailure":
		h.after(stdout, stderr, false)
	}
	return 0
}

type editHook struct {
	r        *Repo
	in       hookInput
	stage    string
	stageDir string
	path     string // relative to the worktree root
	key      string
}

// newEditHook resolves the repository and path of the edited file. It
// returns nil for a file outside any Git worktree.
func newEditHook(in hookInput, stage string) (*editHook, error) {
	if err := ValidateStageID(stage); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(in.Input.FilePath)
	if err != nil {
		return nil, err
	}
	// An edit through a symbolic link changes the link's target.
	if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if abs, err = filepath.EvalSymlinks(abs); err != nil {
			return nil, nil
		}
	}
	dir := filepath.Dir(abs)
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil
		}
		dir = parent
	}
	r, err := OpenRepo(dir)
	if err != nil {
		return nil, nil // not in a Git worktree
	}
	rel, err := r.RepoPath(abs)
	if err != nil {
		return nil, nil
	}
	// On a case-insensitive file system, use the index's spelling, so that
	// the lease and the record name one file whatever case the editor used.
	if r.configBool("core.ignoreCase", false) {
		out, err := r.Run(gitx.Opt{Env: []string{"GIT_LITERAL_PATHSPECS=0", "GIT_ICASE_PATHSPECS=1"}}, "ls-files", "-z", "--", rel)
		if err == nil {
			for _, name := range strings.Split(string(out), "\x00") {
				if strings.EqualFold(name, rel) {
					rel = name
					break
				}
			}
		}
	}
	h := &editHook{r: r, in: in, stage: stage, stageDir: filepath.Join(r.stagesDir(), stage), path: rel}
	sum := sha256.Sum256([]byte(rel))
	h.key = in.ToolUseID
	if h.key == "" {
		h.key = hex.EncodeToString(sum[:8])
	}
	h.key = sanitizeID(h.key)
	return h, nil
}

func (h *editHook) pendingPath() string {
	return filepath.Join(editsDir(h.stageDir), "pending-"+h.key+".json")
}

func (h *editHook) leasePath() string {
	key := h.path
	if h.r.configBool("core.ignoreCase", false) {
		key = strings.ToLower(key) // one file, whatever case the editor used
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(h.r.Dir, "leases", hex.EncodeToString(sum[:8])+".json")
}

// snapshot stores the file's current content as a blob; "" when the file
// does not exist.
func (h *editHook) snapshot() (string, error) {
	if _, err := os.Lstat(filepath.Join(h.r.Top, filepath.FromSlash(h.path))); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return h.r.Line(gitx.Opt{}, "hash-object", "-w", "--path="+h.path, "--", h.path)
}

func (h *editHook) before(stderr io.Writer) int {
	nonce, holder, err := h.takeLease()
	if err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		return 0
	}
	if nonce == "" {
		fmt.Fprintf(stderr, "%s is being edited by stage %s. Wait a moment and make the edit again.\n", h.path, holder)
		return 2
	}
	before, err := h.snapshot()
	if err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		return 0
	}
	data, _ := json.Marshal(pendingEdit{Path: h.path, Before: before, Nonce: nonce})
	if err := os.MkdirAll(editsDir(h.stageDir), 0o777); err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		return 0
	}
	if err := writeFileAtomic(h.pendingPath(), data); err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
	}
	return 0
}

func (h *editHook) after(stdout, stderr io.Writer, succeeded bool) {
	data, err := os.ReadFile(h.pendingPath())
	if err != nil {
		if succeeded {
			fmt.Fprintf(stderr, "git-multistage hook: no snapshot from before the edit of %s; the edit was not recorded\n", h.path)
		}
		return
	}
	os.Remove(h.pendingPath())
	var p pendingEdit
	if err := json.Unmarshal(data, &p); err != nil {
		return
	}
	defer h.releaseLease(p.Nonce)
	if !succeeded {
		return
	}
	// Snapshot first: the lease must still be ours after the snapshot, or
	// another writer may have changed the file before it.
	after, err := h.snapshot()
	if err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		return
	}
	held := h.leaseHeld(p.Nonce)
	if after == p.Before {
		return
	}
	rec := EditRecord{Path: h.path, Before: p.Before, After: after, Tool: h.in.Tool, Uncertain: !held, Time: time.Now().UTC()}
	if after != "" {
		rec.Mode = "100644"
		if info, err := os.Stat(filepath.Join(h.r.Top, filepath.FromSlash(h.path))); err == nil &&
			info.Mode()&0o111 != 0 && h.r.configBool("core.fileMode", true) {
			rec.Mode = "100755"
		}
	}
	if err := writeRecord(h.stageDir, rec); err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		return
	}
	// Keep the recorded blobs reachable for garbage collection.
	if st, err := h.r.openStage(h.stage, false, 10*time.Second); err == nil {
		st.Close()
	}
	// Tell the agent its stage ID once.
	marker := filepath.Join(h.stageDir, "announced")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	if os.WriteFile(marker, nil, 0o666) != nil {
		return
	}
	msg := fmt.Sprintf("git-multistage records your file edits in stage %s. To commit them: "+
		"`git multistage --stage %s add --edits` then `git multistage --stage %s commit -m \"<message>\"`.", h.stage, h.stage, h.stage)
	out, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PostToolUse", "additionalContext": msg}})
	fmt.Fprintln(stdout, string(out))
}

// withLeaseFile runs f while holding the guard lock of the file's lease.
func (h *editHook) withLeaseFile(f func(cur *lease) (*lease, error)) error {
	path := h.leasePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return err
	}
	guard, err := filelock.Acquire(path+".lock", 5*time.Second)
	if err != nil {
		return err
	}
	defer guard.Release()
	var cur *lease
	if data, err := os.ReadFile(path); err == nil {
		var l lease
		if json.Unmarshal(data, &l) == nil {
			cur = &l
		}
	}
	next, err := f(cur)
	if err != nil {
		return err
	}
	switch {
	case next == cur:
		return nil
	case next == nil:
		return os.Remove(path)
	case next != nil:
		data, _ := json.Marshal(next)
		return writeFileAtomic(path, data)
	}
	return nil
}

// takeLease waits until no other stage holds the file, then takes it. It
// returns an empty nonce, and the holder, when the wait times out.
func (h *editHook) takeLease() (nonce, holder string, err error) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	mine := hex.EncodeToString(b)
	timeout := leaseTimeout
	if ms, err := strconv.Atoi(os.Getenv("GIT_MULTISTAGE_LEASE_TIMEOUT_MS")); err == nil && ms >= 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for {
		taken := false
		err := h.withLeaseFile(func(cur *lease) (*lease, error) {
			now := time.Now()
			if cur != nil && cur.Stage != h.stage && cur.Expires > now.UnixMilli() {
				holder = cur.Stage
				return cur, nil
			}
			taken = true
			return &lease{Stage: h.stage, Nonce: mine, Expires: now.Add(leaseTTL).UnixMilli()}, nil
		})
		if err != nil {
			return "", "", err
		}
		if taken {
			return mine, "", nil
		}
		if time.Now().After(deadline) {
			return "", holder, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (h *editHook) leaseHeld(nonce string) bool {
	held := false
	_ = h.withLeaseFile(func(cur *lease) (*lease, error) {
		held = cur != nil && cur.Nonce == nonce
		return cur, nil
	})
	return held
}

func (h *editHook) releaseLease(nonce string) {
	_ = h.withLeaseFile(func(cur *lease) (*lease, error) {
		if cur != nil && cur.Nonce == nonce {
			return nil, nil
		}
		return cur, nil
	})
}
