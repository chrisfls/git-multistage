package ms

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// `git multistage hook claude` runs as a Claude Code PreToolUse and
// PostToolUse hook on the file tools, and `git multistage hook codex` as a
// Codex hook on apply_patch. Before an edit it snapshots the file. After the
// edit it records the tool call's own change in the editing agent's stage,
// without locking the file. A hook error never blocks the edit.

// toolApplyPatch is the file tool of Codex, and of several other harnesses.
const toolApplyPatch = "apply_patch"

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
		claudeEdit
		Content *string      `json:"content"` // Claude Code Write
		Edits   []claudeEdit `json:"edits"`   // Claude Code MultiEdit
	} `json:"tool_input"`
	Response json.RawMessage `json:"tool_response"`
}

// claudeEdit is one string replacement of a Claude Code Edit or MultiEdit.
type claudeEdit struct {
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

// claudeResponse is the part of a Claude Code Edit, MultiEdit, or Write
// tool response that the hook reads: the file as the tool read it.
type claudeResponse struct {
	OriginalFile         *string `json:"originalFile"`
	OriginalFileContents *string `json:"originalFileContents"`
	OldString            *string `json:"oldString"` // Edit: the replacement as made, after any change by the user
	NewString            *string `json:"newString"`
}

type pendingEdit struct {
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	Raw    string `json:"raw,omitempty"` // Codex: the file's bytes before the call, unfiltered
	Tip    string `json:"tip,omitempty"` // the tip of the stage's branch before the call
}

// runEditHook handles one hook call. It always returns 0, which lets the
// edit proceed.
func runEditHook(editor string, stdin io.Reader, stdout, stderr io.Writer, stageFlag string) int {
	c, err := readHookCall(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: cannot read the hook input: %v\n", err)
		return 0
	}
	c.editor = pickEditor(editor, c, os.Getenv)
	if c.Event == eventSessionStart || c.Event == "SubagentStart" {
		announceSharedCheckout(c, stageFlag, stdout)
		return 0
	}
	if c.Event == eventPreTool && shellTools[c.Tool] {
		guardShellCommand(c.hookInput, stdout)
		return 0
	}
	files := callFiles(c)
	if len(files) == 0 {
		return 0
	}
	id := hookStageID(c.editor, c.hookInput, stageFlag)
	if id == "" {
		return 0
	}
	return runEvent(c.Event, newEditHooks(c, id, files, stderr), stdout, stderr)
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

// newEditHooks returns one hook for each edited file in a Git worktree.
func newEditHooks(in hookCall, id string, files []string, stderr io.Writer) []*editHook {
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
	return hooks
}

// runEvent runs the hooks for a PreToolUse, PostToolUse, or
// PostToolUseFailure event, and returns the exit code.
func runEvent(event string, hooks []*editHook, stdout, stderr io.Writer) int {
	switch event {
	case "PreToolUse":
		for _, h := range hooks {
			h.before(stderr)
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
	in       hookCall
	stage    string
	stageDir string
	path     string // relative to the worktree root
	file     string // the path that the tool input names
	key      string
}

// newEditHook resolves the repository and path of the edited file. It
// returns nil for a file outside any Git worktree.
func newEditHook(in hookCall, stage, file string) (*editHook, error) {
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
	if !r.shared() {
		return nil, nil
	}
	rel, err := r.RepoPath(r.topSpelling(dir, abs))
	if err != nil {
		return nil, nil //nolint:nilerr // the file is outside the repository, so the hook records nothing
	}
	// On a case-insensitive file system, use the index's spelling, so that
	// the record names one file whatever case the editor used.
	if r.ConfigBool("core.ignoreCase", false) {
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
	h := &editHook{r: r, in: in, stage: stage, stageDir: filepath.Join(r.stagesDir(), stage), path: rel, file: file}
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
		strings.EqualFold(p[:n], r.Top) && r.ConfigBool("core.ignoreCase", false) {
		return r.Top + p[n:]
	}
	return abs
}

func (h *editHook) pendingPath() string {
	return filepath.Join(editsDir(h.stageDir), "pending-"+h.key+".json")
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

// branchTip returns the tip of the stage's branch, or of the branch that
// HEAD points to before the stage exists; "" when it cannot be read.
func (h *editHook) branchTip() string {
	branch := ""
	if m, err := readMeta(h.stageDir); err == nil {
		branch = m.Branch
	}
	if branch == "" {
		b, err := h.r.headTarget()
		if err != nil {
			return ""
		}
		branch = b
	}
	tip, _ := h.r.ResolveRef(branch)
	return tip
}

func (h *editHook) before(stderr io.Writer) {
	// The snapshot and the branch tip are independent reads.
	var before, tip string
	err := parallel(
		func() (err error) {
			before, err = h.snapshot()
			return err
		},
		func() error {
			tip = h.branchTip()
			return nil
		},
	)
	if err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		return
	}
	var raw string
	if before != "" && h.in.patch != "" {
		if raw, err = h.r.Line(gitx.Opt{}, "hash-object", "-w", "--no-filters", "--", h.path); err != nil {
			fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
			return
		}
	}
	data, _ := json.Marshal(pendingEdit{Path: h.path, Before: before, Raw: raw, Tip: tip})
	if err := os.MkdirAll(editsDir(h.stageDir), 0o777); err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		return
	}
	if err := writeFileAtomic(h.pendingPath(), data); err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
	}
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
	if !succeeded {
		return
	}
	if sec, ok := h.codexMove(p); ok {
		if err := h.recordMove(p, sec); err != nil {
			fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
			return
		}
		h.protectAndAnnounce(stdout)
		return
	}
	after, err := h.snapshot()
	if err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		return
	}
	before := p.Before
	if b, a, ok := h.ownChange(p); ok {
		before, after = b, a
	}
	if after == before {
		return
	}
	rec := EditRecord{Path: h.path, Before: before, After: after, Tool: h.in.Tool, Tip: p.Tip, Time: time.Now().UTC()}
	if after != "" {
		rec.Mode = h.fileMode(h.path)
	}
	if err := writeRecord(h.stageDir, rec); err != nil {
		fmt.Fprintf(stderr, "git-multistage hook: %v\n", err)
		return
	}
	h.protectAndAnnounce(stdout)
}

// protectAndAnnounce keeps the recorded blobs reachable for garbage
// collection, and tells the agent its stage.
func (h *editHook) protectAndAnnounce(stdout io.Writer) {
	if st, err := h.r.openStage(h.stage, false, 10*time.Second); err == nil {
		st.Close()
	}
	h.announceStage(stdout)
}

// codexMove returns the Codex Update File section of the file when it
// moves the file.
func (h *editHook) codexMove(p pendingEdit) (codexPatchSection, bool) {
	if h.in.patch == "" || p.Before == "" {
		return codexPatchSection{}, false
	}
	sec, ok := codexSection(h.in.patch, h.in.Cwd, h.file)
	return sec, ok && sec.kind == codexUpdate && sec.move != ""
}

// recordMove records a Codex Update File section with Move to: a move of
// the file to the new path, then the section's change at the new path.
func (h *editHook) recordMove(p pendingEdit, sec codexPatchSection) error {
	target := sec.move
	if !filepath.IsAbs(target) {
		target = filepath.Join(h.in.Cwd, target)
	}
	target = linkTarget(target)
	newp, err := h.r.RepoPath(h.r.topSpelling(filepath.Dir(target), target))
	if err != nil {
		return err
	}
	after, err := h.movedContent(p, sec, newp)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := writeRecord(h.stageDir, EditRecord{Path: newp, From: h.path, Tool: h.in.Tool, Tip: p.Tip, Time: now}); err != nil {
		return err
	}
	if after == "" || after == p.Before {
		return nil
	}
	rec := EditRecord{
		Path: newp, Before: p.Before, After: after, Mode: h.fileMode(newp), Tool: h.in.Tool, Tip: p.Tip,
		Time: now.Add(time.Nanosecond),
	}
	return writeRecord(h.stageDir, rec)
}

// movedContent returns the blob of the moved file: the section's hunks
// applied to the file before the call, or else the file at newp on disk.
func (h *editHook) movedContent(p pendingEdit, sec codexPatchSection, newp string) (string, error) {
	if p.Raw != "" {
		if raw, err := h.r.catBlob(p.Raw); err == nil && !strings.Contains(string(raw), "\r") {
			if out, ok := applyCodexHunks(string(raw), sec.hunks); ok {
				return h.r.cleanContent(h.indexFile(), newp, []byte(out))
			}
		}
	}
	if !exists(h.r.diskPath(newp)) {
		return "", nil
	}
	return h.r.snapshotBlob(h.indexFile(), newp)
}

// fileMode returns the index mode of the file p on disk.
func (h *editHook) fileMode(p string) string {
	if info, err := os.Stat(h.r.diskPath(p)); err == nil &&
		info.Mode()&0o111 != 0 && h.r.ConfigBool("core.fileMode", true) {
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
	if out := h.in.message(stageMessage(h.stage)); out != "" {
		fmt.Fprintln(stdout, out)
	}
}

// stageMessage tells the agent its stage and the commands to commit.
func stageMessage(stage string) string {
	return fmt.Sprintf("git-multistage records your file edits in stage %s. To commit them: "+
		"`git multistage --stage %s add --edits` then `git multistage --stage %s commit -m \"<message>\"`.",
		stage, stage, stage)
}

// sharedCheckoutRules tells an agent the rules of a shared checkout: what to
// expect from the other agents, so their work does not surprise it, and the
// Git commands that would change their state.
const sharedCheckoutRules = "Other agents may edit files in this checkout and commit to this branch while you work. " +
	"Commits, file changes, and stages that you did not make are theirs: do not review, revert, stage, " +
	"or mention them unless they block your task, and keep their changes in the working tree, even for a moment. " +
	"To run something without them, use a separate worktree from `git worktree add`. " +
	"Edit files with your file-editing tools, such as Edit, Write, " +
	"or apply_patch, so that git-multistage records your edits. Do not run `git add`, `git commit`, `git rm`, " +
	"`git mv`, `git stash`, `git restore`, `git reset`, `git checkout`, or `git switch`: they change state that " +
	"every agent shares. Stage and commit with `git multistage` (see the git-multistage skill)."

// configShared is the Git setting that opts a checkout in to the hooks.
const configShared = "multistage.shared"

// shared reports whether the checkout opted in to the hooks. The hooks
// leave every other repository alone, such as one that a single agent uses.
func (r *Repo) shared() bool { return r.ConfigBool(configShared, false) }

// announceSharedCheckout answers a SessionStart or SubagentStart hook in an
// opted-in checkout with sharedCheckoutRules, so a session or a subagent
// knows from its first turn that others work in the same checkout. In a
// harness that reads no output after a tool call, it also names the stage.
func announceSharedCheckout(c hookCall, stageFlag string, stdout io.Writer) {
	if c.Cwd == "" {
		return
	}
	if r, err := OpenRepo(c.Cwd); err != nil || !r.shared() {
		return // not a Git worktree, or one that did not opt in
	}
	msg := sharedCheckoutRules
	if id := hookStageID(c.editor, c.hookInput, stageFlag); noticeNamesStage(c.editor) && id != "" {
		msg += " " + stageMessage(id)
	}
	if out := c.message(msg); out != "" {
		fmt.Fprintln(stdout, out)
	}
}
