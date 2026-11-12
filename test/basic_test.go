package test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestAgentIDIsRequired(t *testing.T) {
	r := newRepo(t)
	res := r.expectCode(2, "", "status")
	if !strings.Contains(res.err, "GIT_MULTISTAGE_ID") {
		t.Fatalf("error does not name the variable:\n%s", res)
	}
	for _, bad := range []string{"Agent-A", "a b", "../x", "con", "nul.txt", "x.", ""} {
		if bad == "" {
			continue
		}
		r.expectCode(2, bad, "status")
	}
}

func TestInitClaimsUniqueStage(t *testing.T) {
	r := newRepo(t)
	var out struct{ Agent string }
	decode(t, r.mustMS("", "init", "--json"), &out)
	if !regexp.MustCompile(`^agent-[0-9a-f]{8}$`).MatchString(out.Agent) {
		t.Fatalf("generated id %q", out.Agent)
	}
	r.mustMS("", "init", "worker-1")
	res := r.expectCode(5, "", "init", "worker-1")
	if !strings.Contains(res.err, "already exists") {
		t.Fatalf("second init:\n%s", res)
	}
	// The flag and the variable both select the stage.
	r.write("f.txt", "f\n")
	r.mustMS("", "--agent", "worker-1", "add", "f.txt")
	if got := r.stagedPaths("worker-1"); !eq(got, []string{"f.txt"}) {
		t.Fatalf("staged = %v", got)
	}
	// --agent wins over the variable.
	r.mustMS("other", "add", "--agent", "worker-1", "README")
	if got := r.stagedPaths("other"); len(got) != 0 {
		t.Fatalf("stage other got %v", got)
	}
}

func TestAddCommitLog(t *testing.T) {
	r := newRepo(t)
	r.write("src/a.txt", "a\n")
	r.mustMS("agent-a", "add", "src/a.txt")
	res := r.mustMS("agent-a", "commit", "-m", "Add a", "-m", "Body text.")
	if !strings.Contains(res.out, "Add a") {
		t.Fatalf("commit output:\n%s", res)
	}
	if got := r.git("log", "-1", "--format=%B"); got != "Add a\n\nBody text.\n\n" {
		t.Fatalf("message = %q", got)
	}
	if got := r.git("log", "-1", "--format=%an <%ae>|%cn"); strings.TrimSpace(got) != "Test <test@example.com>|Test" {
		t.Fatalf("identity = %q", got)
	}
	r.checkClean()
	if got := r.stagedPaths("agent-a"); len(got) != 0 {
		t.Fatalf("stage not empty after commit: %v", got)
	}
	log := r.mustMS("agent-a", "log", "--oneline")
	if !strings.Contains(log.out, "Add a") {
		t.Fatalf("log:\n%s", log)
	}
	r.expectCode(1, "agent-a", "commit", "-m", "nothing")
	r.expectCode(2, "agent-a", "commit")
	if reflog := r.git("reflog", "-1", "--format=%gs", "main"); !strings.Contains(reflog, "multistage agent-a") {
		t.Fatalf("reflog = %q", reflog)
	}
}

func TestStagesAreIsolated(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.write("b.txt", "b\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-b", "add", "b.txt")
	if got := r.stagedPaths("agent-a"); !eq(got, []string{"a.txt"}) {
		t.Fatalf("agent-a staged %v", got)
	}
	if got := r.stagedPaths("agent-b"); !eq(got, []string{"b.txt"}) {
		t.Fatalf("agent-b staged %v", got)
	}
	r.expectCode(1, "agent-c", "commit", "-m", "x")
	// Ordinary Git's index is untouched.
	if out := r.git("diff", "--cached", "--name-only"); out != "" {
		t.Fatalf("default index changed: %s", out)
	}
	r.mustMS("agent-a", "commit", "-m", "a")
	if got := r.changedPaths("HEAD"); !eq(got, []string{"a.txt"}) {
		t.Fatalf("commit changed %v", got)
	}
}

func TestDeleteRenameAndNewFiles(t *testing.T) {
	r := newRepo(t)
	r.write("old.txt", lines(20, func(i int) string { return "line" }))
	r.write("gone.txt", "gone\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "files")
	if err := os.Rename(r.path("old.txt"), r.path("new.txt")); err != nil {
		t.Fatal(err)
	}
	os.Remove(r.path("gone.txt"))
	r.write("fresh.txt", "fresh\n")
	r.mustMS("agent-a", "add", "old.txt", "new.txt", "gone.txt", "fresh.txt")
	r.mustMS("agent-a", "commit", "-m", "move")
	out := r.git("show", "--name-status", "-M", "--format=", "HEAD")
	for _, want := range []string{"R100\told.txt\tnew.txt", "D\tgone.txt", "A\tfresh.txt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("commit lacks %q:\n%s", want, out)
		}
	}
	r.checkClean()
}

func TestBinaryFilesStageWhole(t *testing.T) {
	r := newRepo(t)
	r.write("img.bin", "\x00\x01\x02")
	r.git("add", "img.bin")
	r.git("commit", "-q", "-m", "bin")
	r.write("img.bin", "\x00\x01\x03")
	var fj filesJSON
	decode(t, r.mustMS("agent-a", "hunks", "--json", "img.bin"), &fj)
	if !fj.Files[0].Binary || len(fj.Files[0].Hunks) != 0 {
		t.Fatalf("binary listing: %+v", fj)
	}
	r.expectCode(4, "agent-a", "add", "img.bin", "--hunk", "0123456789ab")
	r.mustMS("agent-a", "add", "img.bin")
	r.mustMS("agent-a", "commit", "-m", "bin2")
	if r.git("show", "HEAD:img.bin") != "\x00\x01\x03" {
		t.Fatal("binary content not committed")
	}
}

func TestHunkIDsAreStableAndStaleIDsAreRejected(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", lines(30, func(i int) string { return "l" + itoa(i) }))
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	content := strings.Replace(r.read("f.txt"), "l3\n", "THREE\n", 1)
	content = strings.Replace(content, "l20\n", "TWENTY\n", 1)
	r.write("f.txt", content)
	id3 := r.hunkID("agent-a", "f.txt", "THREE")
	id20 := r.hunkID("agent-a", "f.txt", "TWENTY")
	r.mustMS("agent-a", "add", "f.txt", "--hunk", id3)
	// The second ID, listed before the first hunk was staged, still works.
	r.mustMS("agent-a", "add", "f.txt", "--hunk", id20)
	if out := r.mustMS("agent-a", "diff", "--cached").out; !strings.Contains(out, "+THREE") || !strings.Contains(out, "+TWENTY") {
		t.Fatalf("staged diff:\n%s", out)
	}
	r.mustMS("agent-a", "restore", "--staged", "f.txt")

	// After the file changes, an ID of the old hunk is rejected and nothing is staged.
	r.write("f.txt", strings.Replace(r.read("f.txt"), "THREE\n", "3!\n", 1))
	res := r.expectCode(4, "agent-a", "add", "f.txt", "--hunk", id3, "--hunk", id20)
	if !strings.Contains(res.err, id3) {
		t.Fatalf("error does not name the stale id:\n%s", res)
	}
	if got := r.stagedPaths("agent-a"); len(got) != 0 {
		t.Fatalf("a rejected selection staged %v", got)
	}
}

func TestPartialNewFileAndUnstageHunk(t *testing.T) {
	r := newRepo(t)
	// A new file is one hunk: every line is an addition to an empty file.
	r.write("new.txt", "one\ntwo\n")
	id := r.hunkID("agent-a", "new.txt", "one")
	r.mustMS("agent-a", "add", "new.txt", "--hunk", id)
	r.write("f.txt", "")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")

	// Two staged hunks; unstage one by its staged-hunk ID.
	r.write("f.txt", lines(12, func(i int) string { return "x" + itoa(i) }))
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f2")
	r.write("f.txt", strings.NewReplacer("x2\n", "X2\n", "x10\n", "X10\n").Replace(r.read("f.txt")))
	r.mustMS("agent-a", "add", "f.txt")
	var fj filesJSON
	decode(t, r.mustMS("agent-a", "hunks", "--cached", "--json", "f.txt"), &fj)
	if len(fj.Files) != 1 || len(fj.Files[0].Hunks) != 2 {
		t.Fatalf("staged hunks: %+v", fj)
	}
	r.mustMS("agent-a", "restore", "--staged", "f.txt", "--hunk", fj.Files[0].Hunks[0].ID)
	diff := r.mustMS("agent-a", "diff", "--cached", "f.txt").out
	if strings.Contains(diff, "+X2") || !strings.Contains(diff, "+X10") {
		t.Fatalf("after unstaging one hunk:\n%s", diff)
	}
	r.mustMS("agent-a", "commit", "-m", "partial")
	if got := r.git("show", "HEAD:new.txt"); got != "one\ntwo\n" {
		t.Fatalf("new.txt committed as %q", got)
	}
	if got := r.git("show", "HEAD:f.txt"); !strings.Contains(got, "x2\n") || !strings.Contains(got, "X10\n") {
		t.Fatalf("f.txt committed as %q", got)
	}
}

func TestApplyPatch(t *testing.T) {
	r := newRepo(t)
	patch := "diff --git a/README b/README\n--- a/README\n+++ b/README\n@@ -1 +1 @@\n-readme\n+patched\n"
	p := filepath.Join(t.TempDir(), "p.diff")
	os.WriteFile(p, []byte(patch), 0o644)
	r.mustMS("agent-a", "apply", p)
	r.mustMS("agent-a", "commit", "-m", "patched")
	if r.git("show", "HEAD:README") != "patched\n" {
		t.Fatal("patch not committed")
	}
	// The working tree was not touched.
	if r.read("README") != "readme\n" {
		t.Fatal("apply changed the working tree")
	}
	r.expectCode(3, "agent-a", "apply", p)
}

func TestSubdirectoryAndUnusualPaths(t *testing.T) {
	base := filepath.Join(t.TempDir(), "dir with spaces", "ünïcödé")
	os.MkdirAll(base, 0o755)
	r := &repo{t: t, dir: base}
	r.git("init", "-q", "-b", "main")
	names := []string{"sub/a file.txt", "sub/ü.txt", "sub/-dash.txt", "sub/[glob]*.txt"}
	if runtime.GOOS == "windows" {
		names = names[:3]
	}
	for _, n := range names {
		r.write(n, "x\n")
	}
	r.git("add", ".")
	r.git("commit", "-q", "-m", "init")
	for _, n := range names {
		r.write(n, "x\ny\n")
	}
	sub := r.path("sub")
	for _, n := range names {
		rel := strings.TrimPrefix(n, "sub/")
		if res := run(sub, "agent-a", nil, "add", "--", rel); res.code != 0 {
			t.Fatalf("add %s:\n%s", rel, res)
		}
	}
	if res := run(sub, "agent-a", nil, "commit", "-m", "paths"); res.code != 0 {
		t.Fatalf("commit:\n%s", res)
	}
	got := r.changedPaths("HEAD")
	if len(got) != len(names) {
		t.Fatalf("committed %v", got)
	}
	r.checkClean()
	// An absolute path works too.
	r.write("sub/a file.txt", "z\n")
	r.mustMS("agent-a", "add", r.path("sub/a file.txt"))
	if got := r.stagedPaths("agent-a"); !eq(got, []string{"sub/a file.txt"}) {
		t.Fatalf("staged %v", got)
	}
	r.expectCode(2, "agent-a", "add", filepath.Join(t.TempDir(), "outside.txt"))
}

func TestRefusesDetachedHeadAndBranchSwitch(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.git("switch", "-q", "-c", "other")
	res := r.expectCode(5, "agent-a", "commit", "-m", "x")
	if !strings.Contains(res.err, "refs/heads/main") {
		t.Fatalf("mismatch error:\n%s", res)
	}
	r.git("switch", "-q", "main")
	r.mustMS("agent-a", "commit", "-m", "a")
	r.git("switch", "-q", "--detach")
	r.expectCode(5, "agent-new", "status")
}

func TestUnbornBranch(t *testing.T) {
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.write("a.txt", "a\n")
	r.write("b.txt", "b\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-b", "add", "b.txt")
	r.mustMS("agent-a", "commit", "-m", "root")
	r.mustMS("agent-b", "commit", "-m", "second")
	if got := len(r.commits()); got != 2 {
		t.Fatalf("%d commits", got)
	}
	r.checkClean()
}

func TestIgnoredFilesNeedForce(t *testing.T) {
	r := newRepo(t)
	r.write(".gitignore", "*.log\n")
	r.write("x.log", "x\n")
	r.expectCode(1, "agent-a", "add", "x.log")
	r.mustMS("agent-a", "add", "-f", "x.log")
	if got := r.stagedPaths("agent-a"); !eq(got, []string{"x.log"}) {
		t.Fatalf("staged %v", got)
	}
}

func TestDropAndList(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-b", "status")
	var l struct {
		Stages []struct {
			Agent  string
			Staged []string
		}
	}
	decode(t, r.mustMS("", "list", "--json"), &l)
	if len(l.Stages) != 2 || l.Stages[0].Agent != "agent-a" || len(l.Stages[0].Staged) != 1 {
		t.Fatalf("list: %+v", l)
	}
	r.expectCode(5, "agent-a", "drop")
	r.mustMS("agent-a", "drop", "--force")
	r.mustMS("agent-b", "drop")
	decode(t, r.mustMS("", "list", "--json"), &l)
	if len(l.Stages) != 0 {
		t.Fatalf("stages left: %+v", l)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
