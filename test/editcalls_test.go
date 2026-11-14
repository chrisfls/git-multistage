package test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// traceMS runs git-multistage with Git's trace2 events on, and returns the
// result with the Git commands that the run started, such as "ls-files".
func (r *repo) traceMS(agent string, args ...string) (result, []string) {
	r.t.Helper()
	file := filepath.Join(r.t.TempDir(), "trace2")
	res := run(r.dir, agent, []string{"GIT_TRACE2_EVENT=" + file}, args...)
	f, err := os.Open(file)
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var cmds []string
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var e struct {
			Event string   `json:"event"`
			Argv  []string `json:"argv"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Event != "start" || len(e.Argv) < 2 {
			continue
		}
		a := e.Argv[1:]
		for len(a) > 1 && a[0] == "-c" {
			a = a[2:]
		}
		if len(a) > 0 && !strings.HasPrefix(a[0], "--") {
			cmds = append(cmds, a[0])
		}
	}
	return res, cmds
}

func countCmd(cmds []string, cmd string) int {
	n := 0
	for _, c := range cmds {
		if c == cmd {
			n++
		}
	}
	return n
}

func TestAddEditsReadsAndWritesTheIndexOnce(t *testing.T) {
	t.Parallel()
	r := committedRepo(t, map[string]string{"a.txt": twenty, "b.txt": twenty, "gone.txt": "bye\n"})
	r.recordLineOne("a", "a.txt")
	r.recordLineOne("a", "b.txt")
	r.hookedEdit("a", "", "new.txt", "new\n")
	r.hookCall("PreToolUse", "Edit", "a", "", r.path("gone.txt"))
	if err := os.Remove(r.path("gone.txt")); err != nil {
		t.Fatal(err)
	}
	if res := r.hookCall("PostToolUse", "Edit", "a", "", r.path("gone.txt")); res.code != 0 {
		t.Fatal(res)
	}
	res, cmds := r.traceMS("", "--stage", "claude-a", "add", "--edits")
	if res.code != 0 {
		t.Fatal(res)
	}
	if n := countCmd(cmds, "ls-files"); n != 0 {
		t.Errorf("add --edits started %d ls-files, want none: it reads the index file: %v", n, cmds)
	}
	if n := countCmd(cmds, "update-index"); n != 1 {
		t.Errorf("add --edits started %d update-index, want 1: %v", n, cmds)
	}
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "edits")
	if !eq(r.changedPaths("main"), []string{"a.txt", "b.txt", "gone.txt", "new.txt"}) {
		t.Fatalf("the commit changed %v", r.changedPaths("main"))
	}
	if r.git("show", "main:new.txt") != "new\n" || strings.Contains(r.git("ls-tree", "-r", "--name-only", "main"), "gone.txt") {
		t.Fatalf("tree:\n%s", r.git("ls-tree", "-r", "main"))
	}
}

// A new file where the stage holds a directory, and a new file below a path
// where the stage holds a file, fail as they did with one call per record.
func TestAddEditsFailsOnDirectoryFileClash(t *testing.T) {
	t.Parallel()
	r := committedRepo(t, map[string]string{"d/x": "x\n", "f": "f\n"})
	if err := os.RemoveAll(r.path("d")); err != nil {
		t.Fatal(err)
	}
	r.hookedEdit("a", "", "d", "now a file\n")
	res := r.expectCode(1, "", "--stage", "claude-a", "add", "--edits")
	if !strings.Contains(res.err, "d") {
		t.Fatalf("error:\n%s", res)
	}
	if got := r.stagedPaths("claude-a"); len(got) != 0 {
		t.Fatalf("a failed add --edits staged %v", got)
	}

	if err := os.Remove(r.path("f")); err != nil {
		t.Fatal(err)
	}
	r.hookedEdit("b", "", "f/x", "below\n")
	res = r.expectCode(1, "", "--stage", "claude-b", "add", "--edits")
	if !strings.Contains(res.err, "f/x") {
		t.Fatalf("error:\n%s", res)
	}
	if got := r.stagedPaths("claude-b"); len(got) != 0 {
		t.Fatalf("a failed add --edits staged %v", got)
	}
}

// A record that deletes a file before another record puts a directory there
// applies, because the removal comes first in the one call.
func TestAddEditsReplacesAFileWithADirectory(t *testing.T) {
	t.Parallel()
	r := committedRepo(t, map[string]string{"f": "f\n"})
	r.hookCall("PreToolUse", "Edit", "a", "", r.path("f"))
	if err := os.Remove(r.path("f")); err != nil {
		t.Fatal(err)
	}
	if res := r.hookCall("PostToolUse", "Edit", "a", "", r.path("f")); res.code != 0 {
		t.Fatal(res)
	}
	r.hookedEdit("a", "", "f/x", "below\n")
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "dir")
	if got := r.git("ls-tree", "-r", "--name-only", "main"); !strings.Contains(got, "f/x") || strings.Contains(got, "\nf\n") {
		t.Fatalf("tree:\n%s", got)
	}
}

// An ignored file that the stage tracks, deleted and created again by two
// records, is checked as a new file: it needs --force.
func TestAddEditsChecksIgnoreAfterAnEarlierDeletion(t *testing.T) {
	t.Parallel()
	r := committedRepo(t, map[string]string{".gitignore": "*.log\n"})
	r.write("a.log", "l\n")
	r.git("add", "-f", "a.log")
	r.git("commit", "-q", "-m", "log")
	r.hookCall("PreToolUse", "Edit", "a", "", r.path("a.log"))
	if err := os.Remove(r.path("a.log")); err != nil {
		t.Fatal(err)
	}
	if res := r.hookCall("PostToolUse", "Edit", "a", "", r.path("a.log")); res.code != 0 {
		t.Fatal(res)
	}
	r.hookedEdit("a", "", "a.log", "again\n")
	res := r.expectCode(3, "", "--stage", "claude-a", "add", "--edits")
	if !strings.Contains(res.err, "a.log: the file is ignored by .gitignore") {
		t.Fatalf("error:\n%s", res)
	}
	r.mustMS("", "--stage", "claude-a", "add", "--edits", "-f")
}

// A directory/file clash outranks an edit-conflict in the same run: the exit
// code is the one of the failed Git call, as when each record wrote the index.
func TestAddEditsClashOutranksEditConflict(t *testing.T) {
	t.Parallel()
	r := committedRepo(t, map[string]string{"d/x": "x\n", "c.txt": twenty})
	r.recordLineOne("a", "c.txt")
	// Another stage's commit changes the same line, so the record conflicts.
	r.write("c.txt", "OTHER\n"+strings.SplitN(twenty, "\n", 2)[1])
	r.git("commit", "-q", "-am", "other")
	if err := os.RemoveAll(r.path("d")); err != nil {
		t.Fatal(err)
	}
	r.hookedEdit("a", "", "d", "now a file\n")
	res := r.expectCode(1, "", "--stage", "claude-a", "add", "--edits")
	if strings.Contains(res.err, "edit-conflict") || !strings.Contains(res.err, "cacheinfo") {
		t.Fatalf("error:\n%s", res)
	}
}
