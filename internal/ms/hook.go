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
	"sort"
	"strings"
	"time"

	"github.com/chrisfls/git-multistage/internal/filelock"
	"github.com/chrisfls/git-multistage/internal/gitx"
)

// `git multistage hook claude` runs as a Claude Code PreToolUse and
// PostToolUse hook on the file tools, and `git multistage hook codex` as a
// Codex hook on apply_patch. Before an edit it takes a short lease
// on the file, so that two hooked agents never write one file at the same
// moment, and snapshots the file. After the edit it records the before and
// after content in the editing agent's stage. A hook error never blocks the
// edit; only a lease that another stage keeps holding does.

const (
	leaseTTL     = 15 * time.Second
	leaseTimeout = 20 * time.Second
)

// hookInput is the part of a Claude Code or Codex hook input that the hook
// reads. Both put the editing subagent's ID in agent_id.
type hookInput struct {
	Event     string `json:"hook_event_name"`
	Tool      string `json:"tool_name"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"` // set inside a subagent
	ToolUseID string `json:"tool_use_id"`
	Cwd       string `json:"cwd"`
	Input     struct {
		FilePath string `json:"file_path"` // Claude Code Edit, MultiEdit, Write
		Command  string `json:"command"`   // Codex apply_patch: the patch text
	} `json:"tool_input"`
}

// editedFiles returns the files that the tool call edits, as absolute
// paths, for the editor family kind.
func editedFiles(kind string, in hookInput) []string {
	switch {
	case kind == agentClaude && (in.Tool == "Edit" || in.Tool == "MultiEdit" || in.Tool == "Write"):
		if in.Input.FilePath != "" {
			return []string{in.Input.FilePath}
		}
	case kind == agentCodex && in.Tool == "apply_patch":
		var files []string
		for _, line := range strings.Split(in.Input.Command, "\n") {
			line = strings.TrimRight(line, "\r")
			for _, prefix := range []string{"*** Update File: ", "*** Add File: ", "*** Delete File: ", "*** Move to: "} {
				if p, ok := strings.CutPrefix(line, prefix); ok && strings.TrimSpace(p) != "" {
					p = strings.TrimSpace(p)
					if !filepath.IsAbs(p) {
						p = filepath.Join(in.Cwd, p)
					}
					files = append(files, p)
				}
			}
		}
		return files
	}
	return nil
}

type pendingEdit struct {
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	Nonce  string `json:"nonce"`
	// TookOver is set when the lease was taken from another stage whose
	// lease expired unreleased: that stage's edit may still land.
	TookOver bool `json:"took_over,omitempty"`
}

type lease struct {
	Stage   string `json:"stage"`
	Nonce   string `json:"nonce"`
	Expires int64  `json:"expires"` // Unix milliseconds
}

// runEditHook handles one hook call. It returns the exit code for Claude
// Code: 0 lets the edit proceed, 2 blocks it with the stderr message.
func runEditHook(kind string, stdin io.Reader, stdout, stderr io.Writer, stageFlag string) int {
	var in hookInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: cannot read the hook input: %v\n", err)
		return 0
	}
	if in.Event == "SessionStart" {
		announceSharedCheckout(in.Cwd, stdout)
		return 0
	}
	if kind == "" {
		kind = agentClaude
		if in.Tool == "apply_patch" {
			kind = agentCodex
		}
	}
	files := editedFiles(kind, in)
	if len(files) == 0 {
		return 0
	}
	id := hookStageID(kind, in, stageFlag)
	if id == "" {
		return 0
	}
	return runEvent(in.Event, newEditHooks(in, id, files, stderr), stdout, stderr)
}

// hookStageID returns the stage that the edit is recorded in, or "". --stage
// in the hook command wins. A subagent always records into its own stage;
// GIT_MULTISTAGE_ID, inherited by every subagent, names only the main
// session's stage, so it can never merge subagents into one.
func hookStageID(kind string, in hookInput, stageFlag string) string {
	id := stageFlag
	if id == "" && in.AgentID != "" {
		id = sanitizeID(kind + "-" + in.AgentID)
	}
	if id == "" {
		id = os.Getenv("GIT_MULTISTAGE_ID")
	}
	if id == "" && in.SessionID != "" {
		id = sanitizeID(kind + "-" + in.SessionID)
	}
	return id
}

// newEditHooks returns one hook for each edited file in a Git worktree,
// sorted by lease file, so that two multi-file edits never wait for each
// other's leases in a cycle.
func newEditHooks(in hookInput, id string, files []string, stderr io.Writer) []*editHook {
	var hooks []*editHook
	seen := map[string]bool{}
	for _, f := range files {
		h, err := newEditHook(in, id, f)
		if err != nil {
			fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
			continue
		}
		if h != nil && !seen[h.r.Top+"\x00"+h.path] {
			seen[h.r.Top+"\x00"+h.path] = true
			hooks = append(hooks, h)
		}
	}
	sort.Slice(hooks, func(i, j int) bool { return hooks[i].leasePath() < hooks[j].leasePath() })
	return hooks
}

// runEvent runs the hooks for a PreToolUse, PostToolUse, or
// PostToolUseFailure event, and returns the exit code.
func runEvent(event string, hooks []*editHook, stdout, stderr io.Writer) int {
	switch event {
	case "PreToolUse":
		for i, h := range hooks {
			if code := h.before(stderr); code != 0 {
				for _, prev := range hooks[:i] {
					prev.abandon()
				}
				return code
			}
		}
	case "PostToolUse":
		for _, h := range hooks {
			h.after(stdout, stderr, true)
		}
	case "PostToolUseFailure":
		for _, h := range hooks {
			h.after(stdout, stderr, false)
		}
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
func newEditHook(in hookInput, stage, file string) (*editHook, error) {
	if err := ValidateStageID(stage); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	// An edit through a symbolic link changes the link's target, which a
	// Write creates when the link dangles.
	abs = linkTarget(abs)
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
		return nil, nil //nolint:nilerr // not in a Git worktree, so the hook records nothing
	}
	rel, err := r.RepoPath(r.topSpelling(dir, abs))
	if err != nil {
		return nil, nil //nolint:nilerr // the file is outside the repository, so the hook records nothing
	}
	// On a case-insensitive file system, use the index's spelling, so that
	// the lease and the record name one file whatever case the editor used.
	if r.configBool("core.ignoreCase", false) {
		out, err := r.Run(gitx.Opt{Env: []string{envLiteralPathspecs, "GIT_ICASE_PATHSPECS=1"}}, "ls-files", "-z", "--", rel)
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
	h.key = sanitizeID(in.ToolUseID + "-" + hex.EncodeToString(sum[:8]))
	return h, nil
}

// linkTarget follows the symbolic links at path, even to a target that does
// not exist yet. It gives up after 40 links, as the kernel does.
func linkTarget(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	for range 40 {
		target, err := os.Readlink(path)
		if err != nil {
			return path
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = target
	}
	return path
}

// topSpelling spells the worktree root of abs as Git does. On a
// case-insensitive file system an editor may spell it in another case, and
// the worktree may lie behind a symbolic link; dir is an existing directory
// that holds abs.
func (r *Repo) topSpelling(dir, abs string) string {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return abs
	}
	rest, err := filepath.Rel(dir, abs)
	if err != nil {
		return abs
	}
	p := filepath.Join(resolved, rest)
	n := len(r.Top)
	if len(p) >= n && (len(p) == n || p[n] == filepath.Separator) && p[:n] != r.Top &&
		strings.EqualFold(p[:n], r.Top) && r.configBool("core.ignoreCase", false) {
		return r.Top + p[n:]
	}
	return abs
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
	return h.r.snapshotBlob(h.indexFile(), h.path)
}

// indexFile names the index that the stage's `add --edits` applies records
// to: the stage's own index, or the default index before the stage has one.
func (h *editHook) indexFile() string {
	if m, err := readMeta(h.stageDir); err == nil {
		idx := filepath.Join(h.stageDir, m.Index)
		if _, err := os.Stat(idx); err == nil {
			return idx
		}
	}
	return h.r.IndexPath
}

func (h *editHook) before(stderr io.Writer) int {
	nonce, holder, tookOver, err := h.takeLease()
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
	data, _ := json.Marshal(pendingEdit{Path: h.path, Before: before, Nonce: nonce, TookOver: tookOver})
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
	_ = os.Remove(h.pendingPath())
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
	rec := EditRecord{Path: h.path, Before: p.Before, After: after, Tool: h.in.Tool, Uncertain: !held || p.TookOver, Time: time.Now().UTC()}
	if after != "" {
		rec.Mode = h.fileMode()
	}
	if err := writeRecord(h.stageDir, rec); err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		return
	}
	// Keep the recorded blobs reachable for garbage collection.
	if st, err := h.r.openStage(h.stage, false, 10*time.Second); err == nil {
		st.Close()
	}
	h.announceStage(stdout)
}

// fileMode returns the index mode of the edited file on disk.
func (h *editHook) fileMode() string {
	if info, err := os.Stat(filepath.Join(h.r.Top, filepath.FromSlash(h.path))); err == nil &&
		info.Mode()&0o111 != 0 && h.r.configBool("core.fileMode", true) {
		return "100755"
	}
	return "100644"
}

// announceStage tells the agent its stage ID, once per stage.
func (h *editHook) announceStage(stdout io.Writer) {
	marker := filepath.Join(h.stageDir, "announced")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	if os.WriteFile(marker, nil, 0o666) != nil {
		return
	}
	msg := fmt.Sprintf("git-multistage records your file edits in stage %s. To commit them: "+
		"`git multistage --stage %s add --edits` then `git multistage --stage %s commit -m \"<message>\"`. ",
		h.stage, h.stage, h.stage) + otherAgentsNote
	out, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PostToolUse", "additionalContext": msg,
	}})
	fmt.Fprintln(stdout, string(out))
}

// abandon releases the lease and pending snapshot of a PreToolUse call
// whose edit will not happen.
func (h *editHook) abandon() {
	data, err := os.ReadFile(h.pendingPath())
	if err != nil {
		return
	}
	_ = os.Remove(h.pendingPath())
	var p pendingEdit
	if json.Unmarshal(data, &p) == nil {
		h.releaseLease(p.Nonce)
	}
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
// returns an empty nonce, and the holder, when the wait times out. tookOver
// reports that another stage's lease expired without a release.
func (h *editHook) takeLease() (nonce, holder string, tookOver bool, err error) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	mine := hex.EncodeToString(b)
	deadline := time.Now().Add(testLeaseTimeout(leaseTimeout))
	for {
		taken := false
		err := h.withLeaseFile(func(cur *lease) (*lease, error) {
			now := time.Now()
			if cur != nil && cur.Stage != h.stage && cur.Expires > now.UnixMilli() {
				holder = cur.Stage
				return cur, nil
			}
			taken = true
			tookOver = cur != nil && cur.Stage != h.stage
			return &lease{Stage: h.stage, Nonce: mine, Expires: now.Add(leaseTTL).UnixMilli()}, nil
		})
		if err != nil {
			return "", "", false, err
		}
		if taken {
			return mine, "", tookOver, nil
		}
		if time.Now().After(deadline) {
			return "", holder, false, nil
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

// otherAgentsNote tells an agent what to expect from the other agents in a
// shared checkout, so their work does not surprise it.
const otherAgentsNote = "Other agents may edit files and commit to this branch while you work. " +
	"Commits, file changes, and stages that you did not make are theirs: do not review, revert, stage, " +
	"or report them, and leave them out of your summaries unless they block your task."

// announceSharedCheckout answers a SessionStart hook in a Git worktree with
// otherAgentsNote, so the agent knows from its first turn that others work
// in the same checkout.
func announceSharedCheckout(cwd string, stdout io.Writer) {
	if cwd == "" {
		return
	}
	if _, err := OpenRepo(cwd); err != nil {
		return // not a Git worktree
	}
	out, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "SessionStart",
		"additionalContext": "This Git checkout may be shared with other agents through git-multistage. " +
			otherAgentsNote + " Stage and commit your own work with `git multistage` (see the git-multistage skill).",
	}})
	fmt.Fprintln(stdout, string(out))
}
