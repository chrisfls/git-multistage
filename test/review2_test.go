package test

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// These tests reproduce defects found in the second adversarial review.

func TestEditsOfIgnoredNewFileNeedForce(t *testing.T) {
	r := newRepo(t)
	r.write(".gitignore", ".env\n")
	r.git("add", ".gitignore")
	r.git("commit", "-q", "-m", "ignore")
	file := r.path(".env")
	r.hookCall("PreToolUse", "Write", "s", "", file)
	r.write(".env", "API_KEY=secret\n")
	r.hookCall("PostToolUse", "Write", "s", "", file)
	if res := r.ms("", "--stage", "claude-s", "add", "--edits"); res.code == 0 {
		t.Fatalf("an ignored file was staged:\n%s", res)
	}
	r.mustMS("", "--stage", "claude-s", "add", "--edits", "-f")
}

func TestEditThroughSymlinkChangesTheTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	r := newRepo(t)
	r.write("AGENTS.md", "- one\n")
	if err := os.Symlink("AGENTS.md", r.path("CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	r.git("add", ".")
	r.git("commit", "-q", "-m", "link")
	file := r.path("CLAUDE.md")
	r.hookCall("PreToolUse", "Edit", "s", "", file)
	r.write("AGENTS.md", "- one\n- two\n")
	r.hookCall("PostToolUse", "Edit", "s", "", file)
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "claude-s", "commit", "-m", "two")
	if got := r.git("show", "main:CLAUDE.md"); got != "AGENTS.md" {
		t.Fatalf("the link became %q", got)
	}
	if got := r.git("show", "main:AGENTS.md"); got != "- one\n- two\n" {
		t.Fatalf("AGENTS.md = %q", got)
	}
}

func TestSparseCommitDeletingAndAddingOutsideTheCone(t *testing.T) {
	r := newRepo(t)
	r.sparseSetup(false)
	patch := "diff --git a/d2/y.txt b/d2/y.txt\ndeleted file mode 100644\n--- a/d2/y.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-y\n" +
		"diff --git a/d2/new.txt b/d2/new.txt\nnew file mode 100644\n--- /dev/null\n+++ b/d2/new.txt\n@@ -0,0 +1 @@\n+new\n"
	p := t.TempDir() + "/p.diff"
	os.WriteFile(p, []byte(patch), 0o644)
	r.mustMS("agent-a", "apply", p)
	res := r.mustMS("agent-a", "commit", "-m", "outside the cone")
	if strings.Contains(res.err, "not synchronized") {
		t.Fatalf("sync failed:\n%s", res)
	}
	if out := r.git("status", "--porcelain"); out != "" {
		t.Fatalf("git status:\n%s", out)
	}
	var st statusJSON
	decode(t, r.mustMS("agent-a", "status", "--json"), &st)
	if len(st.Status.Unstaged) != 0 {
		t.Fatalf("the stage sees changes: %+v", st.Status.Unstaged)
	}
}

func TestDetachedStageRefusesAnUnrelatedHead(t *testing.T) {
	r := newRepo(t)
	r.git("switch", "-q", "--orphan", "other")
	r.write("o.txt", "o\n")
	r.git("add", "o.txt")
	r.git("commit", "-q", "-m", "other root")
	r.git("switch", "-q", "--detach", "main")
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.git("checkout", "-q", "--detach", "other")
	r.expectCode(5, "agent-a", "commit", "-m", "a")
}

func TestDetachedStageRefusesAfterSwitchDuringCommit(t *testing.T) {
	r := newRepo(t)
	r.hook("pre-commit", "unset GIT_INDEX_FILE; git switch -q main\n")
	r.git("switch", "-q", "--detach", "main")
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	main := r.git("rev-parse", "main")
	r.expectCode(5, "agent-a", "commit", "-m", "a")
	if _, err := r.gitTry("symbolic-ref", "-q", "HEAD"); err != nil {
		t.Fatal("HEAD was detached again")
	}
	if r.git("rev-parse", "main") != main {
		t.Fatal("main moved")
	}
}

func TestSyncDoesNotUndoANativeUnstage(t *testing.T) {
	r := newRepo(t)
	r.write("s.txt", "s\n")
	r.git("add", "s.txt")
	r.git("commit", "-q", "-m", "s")
	r.git("rm", "-q", "--cached", "s.txt")
	if doc := r.mustMS("", "doctor"); strings.Contains(doc.out, "is behind") {
		t.Fatalf("doctor reports a native unstage as lag:\n%s", doc)
	}
	r.expectCode(1, "", "sync")
	if out := r.git("ls-files", "s.txt"); out != "" {
		t.Fatal("sync re-added s.txt")
	}
}

func TestHookLeaseIgnoresCase(t *testing.T) {
	r := newRepo(t)
	if !caseInsensitiveFS(r.dir) {
		t.Skip("case-sensitive file system")
	}
	r.write("F.txt", "f\n")
	r.hookCall("PreToolUse", "Edit", "s1", "", r.path("F.txt"))
	if res := r.hookCall("PreToolUse", "Edit", "s2", "", r.path("f.txt"), "GIT_MULTISTAGE_LEASE_TIMEOUT_MS=100"); res.code != 2 {
		t.Fatalf("a differently cased path took a second lease:\n%s", res)
	}
}

func TestRecordedEditsSurviveGarbageCollection(t *testing.T) {
	r := newRepo(t)
	// The first edit's result exists only in its record once the second
	// edit replaces it on disk.
	r.hookedEdit("s", "", "README", "unique intermediate content 9c1e\nreadme\n")
	r.hookedEdit("s", "", "README", "unique intermediate content 9c1e\nreadme\nmore\n")
	r.git("gc", "-q", "--prune=now")
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "claude-s", "commit", "-m", "r")
}

func TestCherryPickSequenceIsRefused(t *testing.T) {
	r := newRepo(t)
	r.write("c.txt", "a\n")
	r.git("add", "c.txt")
	r.git("commit", "-q", "-m", "c")
	r.git("switch", "-q", "-c", "o")
	r.write("c.txt", "o\n")
	r.git("commit", "-q", "-am", "o1")
	r.write("d.txt", "d\n")
	r.git("add", "d.txt")
	r.git("commit", "-q", "-m", "o2")
	r.git("switch", "-q", "main")
	r.write("c.txt", "m\n")
	r.git("commit", "-q", "-am", "m")
	if _, err := r.gitTry("cherry-pick", "main..o"); err == nil {
		t.Fatal("setup: the cherry-pick should stop")
	}
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.expectCode(5, "agent-a", "commit", "-m", "a")
}
