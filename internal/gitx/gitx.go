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
	"strconv"
	"strings"
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
	ZeroOID      string
	EmptyTree    string
	EmptyBlob    string
	baseEnv      []string
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
// arguments are never globs, read-only commands never rewrite an index, and
// nothing prompts.
func internalEnv() []string {
	var env []string
	for _, kv := range userEnv() {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "GIT_LITERAL_PATHSPECS", "GIT_GLOB_PATHSPECS", "GIT_NOGLOB_PATHSPECS",
			"GIT_ICASE_PATHSPECS", "GIT_OPTIONAL_LOCKS", "GIT_TERMINAL_PROMPT", "GIT_EDITOR":
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GIT_LITERAL_PATHSPECS=1", "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=:")
}

// Open discovers the repository that contains dir.
func Open(dir string) (*Repo, error) {
	r := &Repo{baseEnv: internalEnv()}
	r.Top = dir
	if err := r.checkVersion(); err != nil {
		return nil, err
	}
	out, err := r.Run(Opt{Dir: dir}, "rev-parse", "--path-format=absolute",
		"--show-toplevel", "--git-dir", "--git-common-dir")
	if err != nil {
		var ee *ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("not inside a Git worktree: %s", strings.TrimSpace(ee.Stderr))
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 3 || lines[0] == "" {
		return nil, errors.New("not inside a Git worktree (bare repositories are not supported)")
	}
	r.Top, r.GitDir, r.CommonDir = filepath.Clean(lines[0]), filepath.Clean(lines[1]), filepath.Clean(lines[2])
	out, err = r.Run(Opt{Dir: dir}, "rev-parse", "--show-prefix", "--show-object-format")
	if err != nil {
		return nil, err
	}
	lines = strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) == 2 {
		r.Prefix, r.ObjectFormat = lines[0], lines[1]
	} else {
		r.ObjectFormat = lines[len(lines)-1]
	}
	if r.EmptyTree, err = r.Line(Opt{Stdin: strings.NewReader("")}, "hash-object", "-t", "tree", "--stdin"); err != nil {
		return nil, err
	}
	if r.EmptyBlob, err = r.Line(Opt{Stdin: strings.NewReader("")}, "hash-object", "-w", "--stdin"); err != nil {
		return nil, err
	}
	r.ZeroOID = strings.Repeat("0", len(r.EmptyTree))
	return r, nil
}

var versionRE = regexp.MustCompile(`git version (\d+)\.(\d+)`)

func (r *Repo) checkVersion() error {
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

// Run runs git with args and returns its stdout.
func (r *Repo) Run(o Opt, args ...string) ([]byte, error) {
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
func (r *Repo) TreeOf(commit string) (string, error) {
	if commit == "" {
		return r.EmptyTree, nil
	}
	return r.Line(Opt{}, "rev-parse", commit+"^{tree}")
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
	// Each message: <count> NUL <path>... NUL <type> NUL <text> NUL
	res.ContentOnly = map[string]bool{}
	other := map[string]bool{}
	for i++; i < len(parts); {
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
		res.Messages = append(res.Messages, strings.TrimSpace(text))
		for _, p := range paths {
			if typ == "CONFLICT (contents)" {
				if _, seen := other[p]; !seen {
					res.ContentOnly[p] = true
				}
			} else {
				other[p] = true
				delete(res.ContentOnly, p)
			}
		}
	}
	if len(res.Conflicts) == 0 {
		res.Conflicts = []string{"(unknown)"}
	}
	return res, nil
}
