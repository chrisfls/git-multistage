// Package gitx runs Git subprocesses for one repository and worktree.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MinVersion is the oldest Git that has every feature git-multistage uses.
// `git merge-tree --write-tree` with three trees arrived in Git 2.45.
var MinVersion = [2]int{2, 45}

// Repo describes the repository and worktree that the current directory is in.
type Repo struct {
	Top          string // worktree root, absolute
	GitDir       string // per-worktree Git directory, absolute
	CommonDir    string // shared Git directory, absolute
	Prefix       string // current directory relative to Top, slash-separated, "" or ending in "/"
	ObjectFormat string // "sha1" or "sha256"
	IndexPath    string // the worktree's default index, absolute
	ZeroOID      string
	EmptyTree    string
	EmptyBlob    string
	baseEnv      []string

	emptyBlobOnce sync.Once
	emptyBlobErr  error

	treesMu sync.Mutex
	trees   map[string]string // commit ID to tree ID; a commit's tree never changes

	configOnce sync.Once
	config     map[string]*string // setting to its last value; nil for a key without "="
}

// ExitError is a Git command that exited with a nonzero status.
type ExitError struct {
	Args   []string
	Code   int
	Stderr string
}

func (e *ExitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", e.Code)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

// ExitCode returns the exit status of a Git command error, or -1.
func ExitCode(err error) int {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return -1
}

// Opt holds the per-call settings of a Git command.
type Opt struct {
	Index  string    // GIT_INDEX_FILE; "" uses the worktree's default index
	Stdin  io.Reader // nil reads nothing
	Env    []string  // extra environment entries
	Dir    string    // working directory; "" uses Top
	Stdout io.Writer // when set, output streams here instead of being returned
	Stderr io.Writer // when set, stderr streams here instead of being captured
	User   bool      // run with the caller's environment, as hooks and pass-through commands need
	// KeepFormat writes an index in the format that the repository's
	// configuration asks for, as the default index needs.
	KeepFormat bool
}

// userEnv is the caller's environment without GIT_INDEX_FILE, which
// git-multistage always sets itself.
func userEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.EqualFold(name, "GIT_INDEX_FILE") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// internalEnv is the environment of git-multistage's own Git calls: path
// arguments are never globs, read-only commands never rewrite an index,
// nothing prompts, and GIT_DIFF_OPTS cannot change the context of a diff.
func internalEnv() []string {
	var env []string
	for _, kv := range userEnv() {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "GIT_LITERAL_PATHSPECS", "GIT_GLOB_PATHSPECS", "GIT_NOGLOB_PATHSPECS",
			"GIT_ICASE_PATHSPECS", "GIT_OPTIONAL_LOCKS", "GIT_TERMINAL_PROMPT", "GIT_EDITOR", "GIT_DIFF_OPTS":
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GIT_LITERAL_PATHSPECS=1", "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=:")
}

// emptyObjects holds the object IDs of the empty tree and the empty blob for
// each object format, so that Open starts no `git hash-object` for them.
var emptyObjects = map[string]struct{ tree, blob string }{
	"sha1": {
		"4b825dc642cb6eb9a060e54bf8d69288fbee4904",
		"e69de29bb2d1d6434b8b29ae775ad8c2e48c5391",
	},
	"sha256": {
		"6ef19b41225c5369f1c104d45d8d85efa9b057b53b14b4b9b939dd74decc5321",
		"473a0f4c3be8a93681a267e3b1e9a7dcda1185436fe141f7749120a303721813",
	},
}

// Open discovers the repository that contains dir.
func Open(dir string) (*Repo, error) {
	r := &Repo{baseEnv: internalEnv()}
	r.Top = dir
	// The version check runs beside rev-parse; its error comes first, because
	// an old Git can fail rev-parse for a reason that the check explains.
	versionErr := make(chan error, 1)
	go func() { versionErr <- r.checkVersion() }()
	out, err := r.Run(Opt{Dir: dir}, "rev-parse", "--path-format=absolute",
		"--show-toplevel", "--git-dir", "--git-common-dir", "--show-prefix", "--show-object-format",
		"--git-path", "index")
	if verr := <-versionErr; verr != nil {
		return nil, verr
	}
	if err != nil {
		var ee *ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("not inside a Git worktree: %s", strings.TrimSpace(ee.Stderr))
		}
		return nil, err
	}
	// The prefix is "" at the top of the worktree, so its line can be empty.
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != 6 || lines[0] == "" {
		return nil, errors.New("not inside a Git worktree (bare repositories are not supported)")
	}
	r.Top, r.GitDir, r.CommonDir = filepath.Clean(lines[0]), filepath.Clean(lines[1]), filepath.Clean(lines[2])
	r.Prefix, r.ObjectFormat, r.IndexPath = lines[3], lines[4], filepath.Clean(lines[5])
	empty, ok := emptyObjects[r.ObjectFormat]
	if !ok {
		return nil, fmt.Errorf("unknown object format %q", r.ObjectFormat)
	}
	r.EmptyTree, r.EmptyBlob = empty.tree, empty.blob
	r.ZeroOID = strings.Repeat("0", len(r.EmptyTree))
	return r, nil
}

// EnsureEmptyBlob writes the empty blob to the object database, once for each
// process, and returns its object ID. Git does not store the empty blob by
// itself, and a diff that names it needs it to exist.
func (r *Repo) EnsureEmptyBlob() (string, error) {
	r.emptyBlobOnce.Do(func() {
		var id string
		id, r.emptyBlobErr = r.Line(Opt{Stdin: strings.NewReader("")}, "hash-object", "-w", "--stdin")
		if r.emptyBlobErr == nil && id != r.EmptyBlob {
			r.emptyBlobErr = fmt.Errorf("the empty blob is %s, want %s", id, r.EmptyBlob)
		}
	})
	return r.EmptyBlob, r.emptyBlobErr
}

var versionRE = regexp.MustCompile(`git version (\d+)\.(\d+)`)

// versionChecked caches the outcome of the version check: the Git on PATH
// does not change during one process.
var versionChecked struct {
	once sync.Once
	err  error
}

// checkVersion runs `git version` once for each process.
func (r *Repo) checkVersion() error {
	versionChecked.once.Do(func() { versionChecked.err = r.runVersionCheck() })
	return versionChecked.err
}

func (r *Repo) runVersionCheck() error {
	out, err := r.Run(Opt{}, "version")
	if err != nil {
		return fmt.Errorf("cannot run git: %w", err)
	}
	m := versionRE.FindStringSubmatch(string(out))
	if m == nil {
		return fmt.Errorf("cannot parse %q", strings.TrimSpace(string(out)))
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < MinVersion[0] || (major == MinVersion[0] && minor < MinVersion[1]) {
		return fmt.Errorf("git-multistage needs Git %d.%d or newer; found %s",
			MinVersion[0], MinVersion[1], strings.TrimSpace(string(out)))
	}
	return nil
}

// lostObjectWriteRE matches the error of a loose object write that lost a
// race. Git for Windows creates loose objects by rename, and it checks for an
// existing object before the rename. When another Git process creates the
// same object between the check and the rename, the rename onto the new
// read-only object fails with "Permission denied", and Git reports a failed
// write although the object now exists.
var lostObjectWriteRE = regexp.MustCompile(
	`unable to write file .*objects[/\\][0-9a-f]{2}[/\\][0-9a-f]+: Permission denied`)

// lostObjectWrite reports whether stderr is the error of a loose object
// write that lost a race to another Git process.
func lostObjectWrite(stderr string) bool {
	return lostObjectWriteRE.MatchString(stderr)
}

// renamesObjects reports whether Git creates loose objects by rename, as
// only Git for Windows does. Tests set it to exercise the rerun.
var renamesObjects = runtime.GOOS == "windows"

// objectWriteRetries is how many times Run reruns a command whose loose
// object write lost a race. A rerun finds the object and does not write it.
const objectWriteRetries = 5

// Run runs git with args and returns its stdout.
func (r *Repo) Run(o Opt, args ...string) ([]byte, error) {
	rewind, retry := rerunnable(o)
	for attempt := 1; ; attempt++ {
		out, err := r.run(o, args...)
		var ee *ExitError
		if !retry || attempt > objectWriteRetries || !errors.As(err, &ee) || !lostObjectWrite(ee.Stderr) {
			return out, err
		}
		if rewind() != nil {
			return out, err
		}
		time.Sleep(time.Duration(attempt) * 10 * time.Millisecond)
	}
}

// rerunnable reports whether Run may rerun a call with o after a lost
// object write, and returns the function that rewinds its stdin for the
// rerun. Only Git for Windows creates loose objects by rename. A rerun must
// read the same stdin and must not repeat streamed output.
func rerunnable(o Opt) (rewind func() error, ok bool) {
	noop := func() error { return nil }
	if !renamesObjects || o.User || o.Stdout != nil || o.Stderr != nil {
		return noop, false
	}
	if o.Stdin == nil {
		return noop, true
	}
	stdin, rewindable := o.Stdin.(io.Seeker)
	if !rewindable {
		return noop, false
	}
	start, err := stdin.Seek(0, io.SeekCurrent)
	if err != nil {
		return noop, false
	}
	return func() error {
		_, err := stdin.Seek(start, io.SeekStart)
		return err
	}, true
}

// run runs git with args once.
func (r *Repo) run(o Opt, args ...string) ([]byte, error) {
	gitArgs := args
	if !o.User && !o.KeepFormat {
		// Stage indexes and rewritten copies of the default index are full,
		// unsplit indexes: Git expires the shared index files of a split
		// index, which would break an idle stage.
		gitArgs = append([]string{"-c", "core.splitIndex=false", "-c", "index.sparse=false"}, args...)
	}
	cmd := exec.CommandContext(context.Background(), "git", gitArgs...)
	cmd.Dir = o.Dir
	if cmd.Dir == "" {
		cmd.Dir = r.Top
	}
	cmd.Env = append([]string{}, r.baseEnv...)
	if o.User {
		cmd.Env = userEnv()
	}
	if !o.User && o.Stdout == nil && o.Stderr == nil {
		// git-multistage reads this output, such as an error that Run
		// matches, so Git must not translate it.
		cmd.Env = append(cmd.Env, "LC_ALL=C")
	}
	if o.Index != "" {
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+o.Index)
	}
	cmd.Env = append(cmd.Env, o.Env...)
	cmd.Stdin = o.Stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	if o.Stdout != nil {
		cmd.Stdout = o.Stdout
	}
	cmd.Stderr = &stderr
	if o.Stderr != nil {
		cmd.Stderr = io.MultiWriter(o.Stderr, &stderr)
	}
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return stdout.Bytes(), &ExitError{Args: args, Code: ee.ExitCode(), Stderr: stderr.String()}
		}
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return stdout.Bytes(), nil
}

// Line runs git and returns the first line of its output.
func (r *Repo) Line(o Opt, args ...string) (string, error) {
	out, err := r.Run(o, args...)
	if err != nil {
		return "", err
	}
	s := string(out)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s), nil
}

// Env returns the environment that Git commands run with.
func (r *Repo) Env() []string { return append([]string{}, r.baseEnv...) }

// ResolveRef returns the object ID of ref, or "" when ref does not exist.
func (r *Repo) ResolveRef(ref string) (string, error) {
	out, err := r.Run(Opt{}, "rev-parse", "-q", "--verify", ref+"^{commit}")
	if err != nil {
		if ExitCode(err) == 1 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// SymbolicHead returns the branch that HEAD points to, or "" for a detached HEAD.
func (r *Repo) SymbolicHead() (string, error) {
	out, err := r.Run(Opt{}, "symbolic-ref", "-q", "HEAD")
	if err != nil {
		if ExitCode(err) == 1 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// TreeOf returns the tree of commit, or the empty tree when commit is "".
// It reads the tree of each commit once for each Repo.
func (r *Repo) TreeOf(commit string) (string, error) {
	if commit == "" {
		return r.EmptyTree, nil
	}
	r.treesMu.Lock()
	tree, ok := r.trees[commit]
	r.treesMu.Unlock()
	if ok {
		return tree, nil
	}
	tree, err := r.Line(Opt{}, "rev-parse", commit+"^{tree}")
	if err != nil {
		return "", err
	}
	r.treesMu.Lock()
	if r.trees == nil {
		r.trees = map[string]string{}
	}
	r.trees[commit] = tree
	r.treesMu.Unlock()
	return tree, nil
}

// ConfigBool reads a boolean Git setting, or returns def when the setting is
// missing or not a boolean. The first call reads every setting with one
// `git config --list`; later calls reuse it.
func (r *Repo) ConfigBool(key string, def bool) bool {
	r.configOnce.Do(r.readConfig)
	v, ok := r.config[configKey(key)]
	if !ok {
		return def
	}
	b, ok := parseBool(v)
	if !ok {
		return def
	}
	return b
}

// readConfig reads the output of `git config -z --list`: each setting is
// <key> LF <value> NUL, or <key> NUL for a key without "=". A failed read
// leaves no settings.
func (r *Repo) readConfig() {
	r.config = map[string]*string{}
	out, err := r.Run(Opt{}, "config", "-z", "--list")
	if err != nil {
		return
	}
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		key, value, found := strings.Cut(rec, "\n")
		if !found {
			r.config[configKey(key)] = nil
			continue
		}
		r.config[configKey(key)] = &value
	}
}

// configKey folds the case of the section and the variable of key, which
// Git compares without case, and keeps the case of a subsection.
func configKey(key string) string {
	first, last := strings.IndexByte(key, '.'), strings.LastIndexByte(key, '.')
	if first < 0 {
		return strings.ToLower(key)
	}
	return strings.ToLower(key[:first]) + key[first:last] + strings.ToLower(key[last:])
}

// parseBool reads a boolean setting by Git's rules: a key without "=" is
// true, and an integer, with an optional k, m, or g unit, is true when it is
// not zero.
func parseBool(v *string) (value, ok bool) {
	if v == nil {
		return true, true
	}
	switch strings.ToLower(*v) {
	case "true", "yes", "on":
		return true, true
	case "false", "no", "off", "":
		return false, true
	}
	// Git's strtoimax skips leading white space.
	num := strings.TrimRight(strings.TrimLeft(strings.ToLower(*v), " \t\n\v\f\r"), "kmg")
	if len(*v)-len(num) > 1 || strings.Contains(num, "_") {
		return false, false
	}
	n, err := strconv.ParseInt(num, 0, 64)
	if err != nil {
		return false, false
	}
	return n != 0, true
}

// IsAncestor reports whether a is an ancestor of, or equal to, b.
func (r *Repo) IsAncestor(a, b string) (bool, error) {
	if a == "" || b == "" {
		return false, nil
	}
	_, err := r.Run(Opt{}, "merge-base", "--is-ancestor", a, b)
	if err == nil {
		return true, nil
	}
	if ExitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

// WriteTree writes the index at path as a tree object.
func (r *Repo) WriteTree(index string) (string, error) {
	return r.Line(Opt{Index: index}, "write-tree")
}

// ReadTree replaces the index at path with tree, then refreshes its stat data.
func (r *Repo) ReadTree(index, tree string) error {
	if _, err := r.Run(Opt{Index: index}, "read-tree", tree); err != nil {
		return err
	}
	// Exit status 1 only means that some files differ from the index.
	if _, err := r.Run(Opt{Index: index}, "update-index", "-q", "--refresh"); err != nil && ExitCode(err) != 1 {
		return err
	}
	return nil
}

// Entry is one path of a raw diff between two trees or indexes.
type Entry struct {
	Path, OldPath    string
	OldMode, NewMode string
	OldOID, NewOID   string
	Status           byte
}

// ParseRaw parses the output of a `--raw -z` diff.
func ParseRaw(out []byte) ([]Entry, error) {
	fields := strings.Split(string(out), "\x00")
	var entries []Entry
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "" {
			continue
		}
		if f[0] != ':' {
			return nil, fmt.Errorf("unexpected raw diff field %q", f)
		}
		meta := strings.Fields(f[1:])
		if len(meta) < 5 || i+1 >= len(fields) {
			return nil, fmt.Errorf("short raw diff record %q", f)
		}
		e := Entry{OldMode: meta[0], NewMode: meta[1], OldOID: meta[2], NewOID: meta[3], Status: meta[4][0]}
		i++
		e.Path = fields[i]
		if e.Status == 'R' || e.Status == 'C' {
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("short rename record %q", f)
			}
			i++
			e.OldPath, e.Path = e.Path, fields[i]
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// DiffTrees returns the raw differences between trees a and b.
func (r *Repo) DiffTrees(a, b string, renames bool) ([]Entry, error) {
	args := []string{"diff-tree", "-r", "-z", "--raw", "--no-abbrev"}
	if renames {
		args = append(args, "-M")
	} else {
		args = append(args, "--no-renames")
	}
	out, err := r.Run(Opt{}, append(args, a, b)...)
	if err != nil {
		return nil, err
	}
	return ParseRaw(out)
}

// MergeResult is the outcome of a three-way tree merge.
type MergeResult struct {
	Tree        string
	Conflicts   []string        // conflicted paths
	Messages    []string        // Git's conflict descriptions
	ContentOnly map[string]bool // paths whose only conflict is in their content
}

// parseMergeMessages reads the messages of git merge-tree -z. Each is
// <count> NUL <path>... NUL <type> NUL <text> NUL. It returns the texts of
// the conflicts, and the paths whose only conflict is in their content.
func parseMergeMessages(parts []string) (messages []string, contentOnly map[string]bool) {
	contentOnly = map[string]bool{}
	other := map[string]bool{}
	for i := 0; i < len(parts); {
		n, err := strconv.Atoi(parts[i])
		if err != nil || i+n+2 >= len(parts) {
			break
		}
		paths := parts[i+1 : i+1+n]
		i += 1 + n
		typ, text := parts[i], parts[i+1]
		i += 2
		if !strings.HasPrefix(typ, "CONFLICT") {
			continue
		}
		messages = append(messages, strings.TrimSpace(text))
		for _, p := range paths {
			if typ == "CONFLICT (contents)" {
				if _, seen := other[p]; !seen {
					contentOnly[p] = true
				}
			} else {
				other[p] = true
				delete(contentOnly, p)
			}
		}
	}
	return messages, contentOnly
}

// MergeTrees merges the changes from base to theirs into ours, without
// touching any index or the working tree.
func (r *Repo) MergeTrees(base, ours, theirs string) (MergeResult, error) {
	out, err := r.Run(Opt{}, "merge-tree", "--write-tree", "-z", "--name-only",
		"--merge-base="+base, ours, theirs)
	code := ExitCode(err)
	if err != nil && code != 1 {
		return MergeResult{}, err
	}
	//nolint:dupword // git merge-tree -z ends the conflicted paths with two NULs
	// <tree> NUL <path> NUL ... NUL NUL <messages>
	parts := strings.Split(string(out), "\x00")
	res := MergeResult{Tree: parts[0]}
	if err == nil {
		return res, nil
	}
	i := 1
	for ; i < len(parts) && parts[i] != ""; i++ {
		res.Conflicts = append(res.Conflicts, parts[i])
	}
	var messages []string
	if i+1 < len(parts) {
		messages = parts[i+1:]
	}
	res.Messages, res.ContentOnly = parseMergeMessages(messages)
	if len(res.Conflicts) == 0 {
		res.Conflicts = []string{"(unknown)"}
	}
	return res, nil
}
