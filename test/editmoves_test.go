package test

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// recordTwoEdits records in stage claude-<session> two edits of p, of its
// first and second lines, and leaves the disk as it was. It returns the
// content after both.
func (r *repo) recordTwoEdits(session, p string) string {
	r.t.Helper()
	orig := r.read(p)
	ls := strings.SplitN(orig, "\n", 3)
	r.hookedEdit(session, "", p, "FIRST\n"+ls[1]+"\n"+ls[2])
	both := "FIRST\nSECOND\n" + ls[2]
	r.hookedEdit(session, "", p, both)
	r.write(p, orig)
	return both
}

func TestAddEditsFollowsAMoveWithSeveralRecords(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	r.recordTwoEdits("a", "x.go")
	r.moveAndRewrite("b", "x.go", "y.go", true)
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	if got := r.stagedStatus("claude-a"); !eq(got, []string{"M y.go"}) {
		t.Fatalf("stage claude-a holds %v", got)
	}
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "two lines")
	if got, want := r.git("show", "main:y.go"), "FIRST\nSECOND\n"+strings.SplitN(heavyEdit(twenty), "\n", 3)[2]; got != want {
		t.Fatalf("y.go = %q, want %q", got, want)
	}
}

func TestAddEditsRecreatesFromSeveralRecords(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	both := r.recordTwoEdits("a", "x.go")
	r.deleteAndCommit("x.go")
	r.expectCode(3, "", "--stage", "claude-a", "add", "--edits")
	r.mustMS("", "--stage", "claude-a", "add", "--edits", "--recreate", "x.go")
	if got := r.stagedStatus("claude-a"); !eq(got, []string{"A x.go"}) {
		t.Fatalf("stage claude-a holds %v", got)
	}
	if st := r.mustMS("", "--stage", "claude-a", "status").out; strings.Contains(st, "recorded edits") {
		t.Fatalf("records are left:\n%s", st)
	}
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "Keep x")
	if got := r.git("show", "main:x.go"); got != both {
		t.Fatalf("x.go = %q, want %q", got, both)
	}
}

func TestAddEditsFollowsAPlainGitRename(t *testing.T) {
	for _, staged := range []bool{false, true} {
		r := committedRepo(t, map[string]string{"x.go": twenty})
		if staged {
			r.write("x.go", strings.Replace(twenty, "line p", "STAGED", 1))
			r.mustMS("", "--stage", "claude-a", "add", "x.go")
			r.write("x.go", twenty)
		}
		r.recordLineOne("a", "x.go")
		r.git("mv", "x.go", "y.go")
		r.git("commit", "-q", "-m", "native rename")
		r.mustMS("", "--stage", "claude-a", "add", "--edits")
		if got := r.stagedStatus("claude-a"); !eq(got, []string{"M y.go"}) {
			t.Fatalf("staged %v: stage claude-a holds %v", staged, got)
		}
		if d := r.mustMS("", "--stage", "claude-a", "diff", "--cached").out; !strings.Contains(d, "+FIRST") {
			t.Fatalf("staged %v: staged diff:\n%s", staged, d)
		}
	}
}

func TestAddEditsFollowsAMoveCommitWithAnOlderDate(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	r.recordLineOne("a", "x.go")
	r.mustMS("b", "mv", "x.go", "y.go")
	r.write("y.go", heavyEdit(r.read("y.go")))
	r.mustMS("b", "add", "y.go")
	if res := run(r.dir, "b", []string{"GIT_COMMITTER_DATE=2020-01-01T00:00:00Z"}, "commit", "-m", "move x"); res.code != 0 {
		t.Fatalf("commit:\n%s", res)
	}
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	if got := r.stagedStatus("claude-a"); !eq(got, []string{"M y.go"}) {
		t.Fatalf("stage claude-a holds %v", got)
	}
}

func TestAddEditsFollowsTheStagesOwnMoveAfterAFollowedMove(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	r.recordLineOne("a", "x.go")
	r.moveAndRewrite("b", "x.go", "y.go", true)
	r.mustMS("", "--stage", "claude-a", "mv", "y.go", "z.go")
	var out struct {
		Staged []string `json:"staged_paths"`
	}
	decode(t, r.mustMS("", "--stage", "claude-a", "add", "--edits", "--json"), &out)
	if !eq(out.Staged, []string{"z.go"}) {
		t.Fatalf("staged %v", out.Staged)
	}
	if d := r.mustMS("", "--stage", "claude-a", "diff", "--cached").out; !strings.Contains(d, "+FIRST") {
		t.Fatalf("staged diff:\n%s", d)
	}
}

func TestAddEditsDropsTheRecordsOfAFollowedPathThatTheStageDeleted(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty, "o.go": "o\n"})
	r.recordLineOne("a", "x.go")
	r.hookedEdit("a", "", "o.go", "o\nedited\n")
	r.moveAndRewrite("b", "x.go", "y.go", true)
	r.mustMS("", "--stage", "claude-a", "rm", "-f", "y.go")
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	if got := r.stagedStatus("claude-a"); !eq(got, []string{"M o.go", "D y.go"}) {
		t.Fatalf("stage claude-a holds %v", got)
	}
	if st := r.mustMS("", "--stage", "claude-a", "status").out; strings.Contains(st, "recorded edits") {
		t.Fatalf("records are left:\n%s", st)
	}
}

func TestAddEditsFollowsARenameForARecordWithoutTip(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	r.recordLineOne("a", "x.go")
	files, err := filepath.Glob(r.path(".git/multistage/stages/claude-a/edits/*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("records %v: %v", files, err)
	}
	var rec map[string]any
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	delete(rec, "tip")
	if data, err = json.Marshal(rec); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0], data, 0o666); err != nil {
		t.Fatal(err)
	}
	r.git("mv", "x.go", "y.go")
	r.git("commit", "-q", "-m", "plain rename")
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	if got := r.stagedStatus("claude-a"); !eq(got, []string{"M y.go"}) {
		t.Fatalf("stage claude-a holds %v", got)
	}
}

func TestAddEditsOfAnotherStagesNewFile(t *testing.T) {
	r := committedRepo(t, map[string]string{"o.go": "o\n"})
	r.write("new.go", "one\ntwo\n")
	r.mustMS("b", "add", "new.go")
	r.hookedEdit("a", "", "new.go", "one\nTWO\n")
	var out struct {
		Error struct{ Kind string } `json:"error"`
	}
	decode(t, r.expectCode(3, "", "--stage", "claude-a", "add", "--edits", "--json"), &out)
	if out.Error.Kind != "edit-conflict" {
		t.Fatalf("kind %q", out.Error.Kind)
	}
	if st := r.mustMS("", "--stage", "claude-a", "status").out; !strings.Contains(st, "recorded edits") || !strings.Contains(st, "new.go") {
		t.Fatalf("the records are gone:\n%s", st)
	}
}

func TestAddEditsOfAnUntrackedFileOnAnUnbornBranch(t *testing.T) {
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q")
	r.write("n.go", "one\n")
	r.hookedEdit("a", "", "n.go", "two\n")
	var out struct {
		Error struct{ Kind string } `json:"error"`
	}
	decode(t, r.expectCode(3, "", "--stage", "claude-a", "add", "--edits", "--json"), &out)
	if out.Error.Kind != "edit-conflict" {
		t.Fatalf("kind %q", out.Error.Kind)
	}
}

func TestRmForgetsTheRecordsThatFollowAMoveToTheDeletedPath(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty, "w.go": "w\n"})
	r.recordLineOne("a", "x.go")
	r.moveAndRewrite("b", "x.go", "y.go", false)
	r.mustMS("", "--stage", "claude-a", "rm", "-f", "y.go")
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "a deletes y")
	r.hookedEdit("a", "", "w.go", "w\nedited\n")
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	if got := r.stagedStatus("claude-a"); !eq(got, []string{"M w.go"}) {
		t.Fatalf("stage claude-a holds %v", got)
	}
}

func TestRmForgetsTheRecordsThatFollowAFollowedMoveAndTheStagesOwnMove(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty, "w.go": "w\n"})
	r.recordLineOne("a", "x.go")
	r.moveAndRewrite("b", "x.go", "y.go", false)
	r.mustMS("", "--stage", "claude-a", "mv", "y.go", "z.go")
	r.mustMS("", "--stage", "claude-a", "rm", "-f", "z.go")
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "a deletes y")
	r.hookedEdit("a", "", "w.go", "w\nedited\n")
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	if got := r.stagedStatus("claude-a"); !eq(got, []string{"M w.go"}) {
		t.Fatalf("stage claude-a holds %v", got)
	}
}
