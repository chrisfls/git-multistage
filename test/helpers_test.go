// Package test holds the integration tests of git-multistage. They run the
// real binary and real Git against temporary repositories.
package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

var bin string // the git-multistage binary under test

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "git-multistage-test")
	if err != nil {
		panic(err)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bin = filepath.Join(dir, "git-multistage"+exe)
	build := exec.Command("go", "build", "-tags", "testhooks", "-o", bin, "../cmd/git-multistage")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.Exit(1)
	}
	cfg := filepath.Join(dir, "gitconfig")
	os.WriteFile(cfg, []byte("[user]\n\tname = Test\n\temail = test@example.com\n"+
		"[init]\n\tdefaultBranch = main\n[core]\n\tautocrlf = false\n[commit]\n\tgpgSign = false\n"), 0o644)
	os.Setenv("GIT_CONFIG_GLOBAL", cfg)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, v := range []string{"GIT_MULTISTAGE_ID", "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE",
		"GIT_MULTISTAGE_FAULT", "GIT_MULTISTAGE_TEST_RACE", "GIT_MULTISTAGE_TEST_DELAY_MS",
		"PASEO_AGENT_ID", "CLAUDE_CODE_SESSION_ID"} {
		os.Unsetenv(v)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type repo struct {
	t   testing.TB
	dir string
}

// newRepo creates a repository on branch main with one commit.
func newRepo(t testing.TB) *repo {
	t.Helper()
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.write("README", "readme\n")
	r.git("add", "README")
	r.git("commit", "-q", "-m", "init")
	return r
}

func (r *repo) path(p string) string { return filepath.Join(r.dir, filepath.FromSlash(p)) }

func (r *repo) git(args ...string) string {
	r.t.Helper()
	out, err := r.gitTry(args...)
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func (r *repo) gitTry(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func (r *repo) write(p, content string) {
	r.t.Helper()
	full := r.path(p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) read(p string) string {
	r.t.Helper()
	b, err := os.ReadFile(r.path(p))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

type result struct {
	out, err string
	code     int
}

func (res result) String() string {
	return fmt.Sprintf("exit %d\nstdout:\n%s\nstderr:\n%s", res.code, res.out, res.err)
}

// run runs git-multistage in dir with GIT_MULTISTAGE_ID=agent and extra env.
func run(dir, agent string, env []string, args ...string) result {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if agent != "" {
		cmd.Env = append(cmd.Env, "GIT_MULTISTAGE_ID="+agent)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		code = -1
		errb.WriteString(err.Error())
	}
	return result{out.String(), errb.String(), code}
}

func (r *repo) ms(agent string, args ...string) result { return run(r.dir, agent, nil, args...) }

func (r *repo) mustMS(agent string, args ...string) result {
	r.t.Helper()
	res := r.ms(agent, args...)
	if res.code != 0 {
		r.t.Fatalf("git-multistage %s (agent %s) failed:\n%s", strings.Join(args, " "), agent, res)
	}
	return res
}

func (r *repo) expectCode(want int, agent string, args ...string) result {
	r.t.Helper()
	res := r.ms(agent, args...)
	if res.code != want {
		r.t.Fatalf("git-multistage %s (agent %s): want exit %d\n%s", strings.Join(args, " "), agent, want, res)
	}
	return res
}

func decode(t testing.TB, res result, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(res.out), v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, res)
	}
}

type hunkJSON struct {
	ID       string   `json:"id"`
	Lines    []string `json:"lines"`
	StagedBy []string `json:"staged_by"`
}

type filesJSON struct {
	Files []struct {
		Path   string     `json:"path"`
		Status string     `json:"status"`
		Binary bool       `json:"binary"`
		Note   string     `json:"note"`
		Hunks  []hunkJSON `json:"hunks"`
	} `json:"files"`
}

// findHunk returns the ID of the hunk of path that adds the line added.
// It is safe to call from any goroutine.
func findHunk(dir, agent, path, added string) (string, error) {
	res := run(dir, agent, nil, "hunks", "--json", path)
	if res.code != 0 {
		return "", fmt.Errorf("hunks: %s", res)
	}
	var fj filesJSON
	if err := json.Unmarshal([]byte(res.out), &fj); err != nil {
		return "", fmt.Errorf("bad JSON: %v\n%s", err, res)
	}
	for _, f := range fj.Files {
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if l == "+"+added {
					return h.ID, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no hunk adds %q in %s for %s:\n%s", added, path, agent, res.out)
}

func (r *repo) hunkID(agent, path, added string) string {
	r.t.Helper()
	id, err := findHunk(r.dir, agent, path, added)
	if err != nil {
		r.t.Fatal(err)
	}
	return id
}

func gitCmd(dir string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, env
	return cmd
}

type statusJSON struct {
	Status struct {
		Staged []struct {
			Status, Path string
		} `json:"staged"`
		Unstaged []struct {
			Status, Path string
		} `json:"unstaged"`
		Untracked []string `json:"untracked"`
		Freshness struct {
			Stale bool `json:"stale"`
		} `json:"freshness"`
	} `json:"status"`
}

func (r *repo) stagedPaths(agent string) []string {
	r.t.Helper()
	var st statusJSON
	decode(r.t, r.mustMS(agent, "status", "--json"), &st)
	paths := []string{}
	for _, s := range st.Status.Staged {
		paths = append(paths, s.Path)
	}
	sort.Strings(paths)
	return paths
}

// commits returns the commits of main, newest first.
func (r *repo) commits() []string {
	return strings.Fields(r.git("rev-list", "main"))
}

// changedPaths lists the paths that commit changes relative to its parent.
func (r *repo) changedPaths(commit string) []string {
	out := r.git("diff-tree", "-z", "--no-commit-id", "-r", "--name-only", "--root", "--no-renames", commit)
	paths := []string{}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths
}

// checkHealthy verifies the repository and that history is linear.
func (r *repo) checkHealthy() {
	r.t.Helper()
	if out, err := r.gitTry("fsck", "--strict", "--no-progress"); err != nil {
		r.t.Fatalf("git fsck: %v\n%s", err, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(r.git("rev-list", "--parents", "main")), "\n") {
		if len(strings.Fields(line)) > 2 {
			r.t.Fatalf("history is not linear: %s", line)
		}
	}
}

// checkIndexSynced verifies that the default index matches HEAD.
func (r *repo) checkIndexSynced() {
	r.t.Helper()
	if out := r.git("diff", "--cached", "--name-status"); strings.TrimSpace(out) != "" {
		r.t.Fatalf("default index differs from HEAD:\n%s", out)
	}
}

// checkClean verifies that the default index and working tree match HEAD.
func (r *repo) checkClean() {
	r.t.Helper()
	if out := r.git("status", "--porcelain"); strings.TrimSpace(out) != "" {
		r.t.Fatalf("git status is not clean:\n%s", out)
	}
}

func eq(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }

func lines(n int, f func(i int) string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString(f(i))
		b.WriteString("\n")
	}
	return b.String()
}
