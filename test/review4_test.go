package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// These tests reproduce defects found in the fourth review.

func TestJSONOutputKeepsStageWarnings(t *testing.T) {
	r := newRepo(t)
	var out struct {
		Warnings []string `json:"warnings"`
	}
	res := r.mustMS("agent-a", "status", "--json")
	decode(t, res, &out)
	if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "created stage agent-a") {
		t.Fatalf("status --json warnings = %q:\n%s", out.Warnings, res)
	}
	res = r.mustMS("agent-b", "status")
	if n := strings.Count(res.err, "created stage agent-b"); n != 1 {
		t.Fatalf("text mode printed the warning %d times:\n%s", n, res)
	}
}

func TestLogFiltersByThePathspec(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-a", "commit", "-m", "add a")
	r.write("b.txt", "b\n")
	r.mustMS("agent-b", "add", "b.txt")
	r.mustMS("agent-b", "commit", "-m", "add b")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"log", "--format=%s"}, "add a\n"},
		{[]string{"log", "--format=%s", "a.txt"}, "add a\n"},
		{[]string{"log", "--format=%s", "--", "a.txt"}, "add a\n"},
		{[]string{"log", "--format=%s", "--", "b.txt"}, ""},
	} {
		if res := r.mustMS("agent-a", c.args...); res.out != c.want {
			t.Errorf("%s = %q, want %q", strings.Join(c.args, " "), res.out, c.want)
		}
	}
}

func TestHookAfterGlobalOptions(t *testing.T) {
	r := newRepo(t)
	file := r.path("README")
	call := func(event string, args ...string) {
		t.Helper()
		in := `{"hook_event_name":"` + event + `","tool_name":"Edit","session_id":"s","tool_use_id":"toolu_1",` +
			`"tool_input":{"file_path":` + strconv.Quote(file) + `}}`
		if res := runStdin(r.dir, "", nil, in, args...); res.code != 0 {
			t.Fatalf("%s %s:\n%s", strings.Join(args, " "), event, res)
		}
	}
	call("PreToolUse", "--stage", "mine", "hook", "claude")
	r.write("README", "edited\n")
	call("PostToolUse", "--stage=mine", "--json", "hook", "claude")
	r.mustMS("mine", "add", "--edits")
	if got := r.stagedPaths("mine"); !eq(got, []string{"README"}) {
		t.Fatalf("staged %q", got)
	}
}

// expireLeases makes every edit lease of the repository expire, as 15
// seconds of waiting on an approval prompt would.
func (r *repo) expireLeases() {
	r.t.Helper()
	paths, err := filepath.Glob(r.path(".git/multistage/leases/*.json"))
	if err != nil || len(paths) == 0 {
		r.t.Fatalf("no leases: %v", err)
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			r.t.Fatal(err)
		}
		var l map[string]any
		if err := json.Unmarshal(data, &l); err != nil {
			r.t.Fatal(err)
		}
		l["expires"] = 0
		data, _ = json.Marshal(l)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
}

func TestTakingAnExpiredLeaseMakesTheEditUncertain(t *testing.T) {
	r := newRepo(t)
	r.write("notes.txt", "one\ntwo\nthree\n")
	r.git("add", "notes.txt")
	r.git("commit", "-q", "-m", "notes")
	patchA := "*** Begin Patch\n*** Update File: notes.txt\n@@\n-two\n+TWO\n*** End Patch"
	patchB := "*** Begin Patch\n*** Update File: notes.txt\n@@\n-three\n+THREE\n*** End Patch"
	if res := r.codexCall("PreToolUse", "a", "", "exec-a", patchA); res.code != 0 {
		t.Fatalf("A PreToolUse:\n%s", res)
	}
	r.expireLeases()
	if res := r.codexCall("PreToolUse", "b", "", "exec-b", patchB); res.code != 0 {
		t.Fatalf("B PreToolUse:\n%s", res)
	}
	r.write("notes.txt", "one\nTWO\nthree\n")
	r.codexCall("PostToolUse", "a", "", "exec-a", patchA)
	r.write("notes.txt", "one\nTWO\nTHREE\n")
	r.codexCall("PostToolUse", "b", "", "exec-b", patchB)
	res := r.expectCode(3, "", "--stage", "codex-b", "add", "--edits")
	if !strings.Contains(res.err, "another writer may have changed") {
		t.Fatalf("add --edits:\n%s", res)
	}
	r.expectCode(3, "", "--stage", "codex-a", "add", "--edits")
}

func TestWriteThroughADanglingSymlinkIsRecorded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	r := newRepo(t)
	if err := os.Symlink("docs/AGENTS.md", r.path("CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	r.git("add", ".")
	r.git("commit", "-q", "-m", "link")
	file := r.path("CLAUDE.md")
	r.hookCall("PreToolUse", "Write", "s", "", file)
	r.write("docs/AGENTS.md", "- one\n")
	r.hookCall("PostToolUse", "Write", "s", "", file)
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	if got := r.stagedPaths("claude-s"); !eq(got, []string{"docs/AGENTS.md"}) {
		t.Fatalf("staged %q", got)
	}
}

func TestHookAcceptsAPathInAnotherCase(t *testing.T) {
	r := newRepo(t)
	if strings.TrimSpace(r.git("config", "--get", "core.ignoreCase")) != "true" {
		t.Skip("the file system is case-sensitive")
	}
	r.write("f.txt", "1\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	// The test's temporary directory, such as TestHook.../001, in upper case.
	parent := filepath.Dir(r.dir)
	file := filepath.Join(filepath.Dir(parent), strings.ToUpper(filepath.Base(parent)), filepath.Base(r.dir), "f.txt")
	if _, err := os.Stat(file); err != nil {
		t.Skip("the file system is case-sensitive")
	}
	r.hookCall("PreToolUse", "Edit", "s", "", file)
	r.write("f.txt", "2\n")
	r.hookCall("PostToolUse", "Edit", "s", "", file)
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	if got := r.stagedPaths("claude-s"); !eq(got, []string{"f.txt"}) {
		t.Fatalf("staged %q", got)
	}
}

func TestHookKeepsCRLFOfACommittedFile(t *testing.T) {
	r := newRepo(t)
	r.write("crlf.txt", "one\r\ntwo\r\nthree\r\n")
	r.git("add", "crlf.txt")
	r.git("commit", "-q", "-m", "crlf")
	r.git("config", "core.autocrlf", "true")
	r.hookedEdit("s", "", "crlf.txt", "one\r\nTWO\r\nthree\r\n")
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "claude-s", "commit", "-m", "two")
	if got := r.git("cat-file", "blob", "main:crlf.txt"); got != "one\r\nTWO\r\nthree\r\n" {
		t.Fatalf("crlf.txt = %q", got)
	}
}
