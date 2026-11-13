package test

import (
	"os"
	"strings"
	"testing"
)

// recordLineOne records in stage claude-<session> an edit of the first line
// of p, and leaves the disk as it was. It returns the edited content.
func (r *repo) recordLineOne(session, p string) string {
	r.t.Helper()
	orig := r.read(p)
	edited := "FIRST\n" + strings.SplitN(orig, "\n", 2)[1]
	r.hookedEdit(session, "", p, edited)
	r.write(p, orig)
	return edited
}

func TestAddEditsFollowsAnotherStagesMove(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	r.recordLineOne("a", "x.go")
	r.moveAndRewrite("b", "x.go", "y.go", true)
	var out struct {
		Staged []string `json:"staged_paths"`
	}
	decode(t, r.mustMS("", "--stage", "claude-a", "add", "--edits", "--json"), &out)
	if !eq(out.Staged, []string{"y.go"}) {
		t.Fatalf("staged %v", out.Staged)
	}
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "first line")
	if !eq(r.changedPaths("main"), []string{"y.go"}) {
		t.Fatalf("the commit changed %v", r.changedPaths("main"))
	}
	if got, want := r.git("show", "main:y.go"), "FIRST\n"+strings.SplitN(heavyEdit(twenty), "\n", 2)[1]; got != want {
		t.Fatalf("y.go = %q, want %q", got, want)
	}
}

func TestAddEditsMeetsAnotherStagesDeletion(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	edited := r.recordLineOne("a", "x.go")
	deleting := r.deleteAndCommit("x.go")
	res := r.expectCode(3, "", "--stage", "claude-a", "add", "--edits")
	if !strings.Contains(res.err, "x.go was deleted by "+deleting) || !strings.Contains(res.err, "--recreate") ||
		!strings.Contains(res.err, "drop --edits") {
		t.Fatalf("error:\n%s", res)
	}
	var out struct {
		Error struct{ Kind string } `json:"error"`
	}
	decode(t, r.expectCode(3, "", "--stage", "claude-a", "add", "--edits", "--json"), &out)
	if out.Error.Kind != "path-deleted" {
		t.Fatalf("kind %q", out.Error.Kind)
	}
	if st := r.mustMS("", "--stage", "claude-a", "status").out; !strings.Contains(st, "recorded edits") || !strings.Contains(st, "x.go") {
		t.Fatalf("the records are gone:\n%s", st)
	}

	r.mustMS("", "--stage", "claude-a", "add", "--edits", "--recreate", "x.go")
	if got := r.stagedStatus("claude-a"); !eq(got, []string{"A x.go"}) {
		t.Fatalf("stage claude-a holds %v", got)
	}
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "Keep x")
	if got := r.git("show", "main:x.go"); got != edited {
		t.Fatalf("x.go = %q, want %q", got, edited)
	}
	r.expectCode(2, "a", "add", "x.go", "--recreate", "x.go")
}

func TestCodexMovesAFile(t *testing.T) {
	r := committedRepo(t, map[string]string{"f.txt": "one\ntwo\nthree\n"})
	const stage = "codex-019a7c2e-4f1b-7d30-9a8e-5c6b7d8e9f01"
	file := r.path("f.txt")
	if res := r.replayHook("codex-pre-apply-patch-move", file); res.code != 0 {
		t.Fatalf("PreToolUse:\n%s", res)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	r.write("g.txt", "one\nTWO\nthree\n")
	if res := r.replayHook("codex-post-apply-patch-move", file); res.code != 0 {
		t.Fatalf("PostToolUse:\n%s", res)
	}
	r.mustMS("", "--stage", stage, "add", "--edits")
	if got := r.stageMoves(stage); !eq(got, []string{"f.txt -> g.txt"}) {
		t.Fatalf("moves %v", got)
	}
	r.mustMS("", "--stage", stage, "commit", "-m", "Move f")
	if got := r.git("show", "main:g.txt"); got != "one\nTWO\nthree\n" {
		t.Fatalf("g.txt = %q", got)
	}
	if _, err := r.gitTry("cat-file", "-e", "main:f.txt"); err == nil {
		t.Fatal("f.txt is still on main")
	}
	if msg := strings.TrimSpace(r.git("log", "-1", "--format=%B")); !strings.HasSuffix(msg, "Multistage-Moved: f.txt -> g.txt") {
		t.Fatalf("message:\n%s", msg)
	}
	r.checkClean()
}
