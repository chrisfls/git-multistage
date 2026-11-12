package test

import (
	"os"
	"strings"
	"testing"
)

func TestDetachedHead(t *testing.T) {
	r := newRepo(t)
	r.git("switch", "-q", "--detach")
	main := r.git("rev-parse", "main")
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.write("b.txt", "b\n")
	r.mustMS("agent-b", "add", "b.txt")
	parallel(2, func(i int) {
		r.ms([]string{"agent-a", "agent-b"}[i], "commit", "-m", []string{"a", "b"}[i])
	})
	if _, err := r.gitTry("symbolic-ref", "-q", "HEAD"); err == nil {
		t.Fatal("HEAD is no longer detached")
	}
	if r.git("rev-parse", "main") != main {
		t.Fatal("main moved")
	}
	if got := len(strings.Fields(r.git("rev-list", "HEAD"))); got != 3 {
		t.Fatalf("%d commits on HEAD, want 3", got)
	}
	r.checkClean()
}

func (r *repo) sparseSetup(sparseIndex bool) {
	r.t.Helper()
	r.write("d1/x.txt", "x\n")
	r.write("d2/y.txt", "y\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "dirs")
	args := []string{"sparse-checkout", "set", "--cone"}
	if sparseIndex {
		args = append(args, "--sparse-index")
	}
	r.git(append(args, "d1")...)
	if _, err := os.Stat(r.path("d2/y.txt")); err == nil {
		r.t.Fatal("setup: d2 is still checked out")
	}
}

func testSparse(t *testing.T, sparseIndex bool) {
	r := newRepo(t)
	r.sparseSetup(sparseIndex)
	var st statusJSON
	decode(t, r.mustMS("agent-a", "status", "--json"), &st)
	if len(st.Status.Unstaged) != 0 {
		t.Fatalf("files outside the sparse checkout look changed: %+v", st.Status.Unstaged)
	}
	r.write("d1/x.txt", "x2\n")
	r.mustMS("agent-a", "add", "d1/x.txt")
	r.mustMS("agent-a", "commit", "-m", "x2")
	if got := r.git("show", "main:d2/y.txt"); got != "y\n" {
		t.Fatalf("d2/y.txt = %q", got)
	}
	if out := r.git("status", "--porcelain"); out != "" {
		t.Fatalf("git status:\n%s", out)
	}
	if got := r.git("-c", "advice.sparseIndexExpanded=false", "ls-files", "-t", "d2/y.txt"); !strings.HasPrefix(got, "S ") {
		t.Fatalf("d2/y.txt lost its skip-worktree bit: %q", got)
	}
}

func TestSparseCheckout(t *testing.T) { testSparse(t, false) }
func TestSparseIndex(t *testing.T)    { testSparse(t, true) }

func TestSplitIndex(t *testing.T) {
	r := newRepo(t)
	r.git("config", "core.splitIndex", "true")
	r.write("n.txt", "n\n")
	r.git("add", "n.txt")
	r.git("commit", "-q", "-m", "n")
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-a", "commit", "-m", "a")
	r.checkClean()
	r.checkHealthy()
	r.write("p.txt", "p\n")
	r.git("add", "p.txt")
	r.git("commit", "-q", "-m", "native after agent")
	r.checkClean()
}

func TestMergeInProgress(t *testing.T) {
	r := newRepo(t)
	r.write("o.txt", "base\n")
	r.git("add", "o.txt")
	r.git("commit", "-q", "-m", "o")
	r.git("switch", "-q", "-c", "other")
	r.write("o.txt", "other\n")
	r.git("commit", "-q", "-am", "other")
	r.git("switch", "-q", "main")
	r.write("o.txt", "main\n")
	r.git("commit", "-q", "-am", "main")
	if _, err := r.gitTry("merge", "-q", "other"); err == nil {
		t.Fatal("setup: the merge should conflict")
	}
	// A commit of an unrelated path lands during the merge.
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-a", "commit", "-m", "a during merge")
	// A commit of a path that the merge changed is refused.
	r.write("o.txt", "agent\n")
	r.mustMS("agent-b", "add", "o.txt")
	res := r.expectCode(5, "agent-b", "commit", "-m", "o during merge")
	if !strings.Contains(res.err, "o.txt") {
		t.Fatalf("error:\n%s", res)
	}
	r.mustMS("agent-b", "restore", "--staged", "o.txt")
	r.write("o.txt", "resolved\n")
	r.git("add", "o.txt")
	r.git("commit", "-q", "--no-edit")
	if got := strings.Fields(r.git("log", "-1", "--format=%P")); len(got) != 2 {
		t.Fatalf("the merge commit has parents %v", got)
	}
	if r.git("show", "main:a.txt") != "a\n" {
		t.Fatal("the merge commit dropped the stage's commit")
	}
	r.checkClean()
}

func TestRebaseInProgressIsRefused(t *testing.T) {
	r := newRepo(t)
	r.git("switch", "-q", "-c", "feature")
	r.write("README", "feature\n")
	r.git("commit", "-q", "-am", "feature")
	r.git("switch", "-q", "main")
	r.write("README", "main\n")
	r.git("commit", "-q", "-am", "main")
	r.git("switch", "-q", "feature")
	if _, err := r.gitTry("rebase", "main"); err == nil {
		t.Fatal("setup: the rebase should stop")
	}
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.expectCode(5, "agent-a", "commit", "-m", "a")
}

// TestIndexLagIsCaughtUp moves the branch behind the default index's back,
// then checks that the next stage commit brings the index forward.
func TestIndexLagIsCaughtUp(t *testing.T) {
	r := newRepo(t)
	r.mustMS("agent-a", "status")
	blob := strings.TrimSpace(r.git("rev-parse", "main:README"))
	r.externalCommit("external", func(idx string) {
		gitCmd(r.dir, append(os.Environ(), "GIT_INDEX_FILE="+idx), "update-index", "--add", "--cacheinfo", "100644", blob, "copy.txt").Run()
	})
	if r.git("diff", "--cached", "--name-only") == "" {
		t.Fatal("setup: the index should lag")
	}
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-a", "commit", "-m", "a")
	r.checkIndexSynced()
}

func TestManualSyncFindsTheMatchingCommit(t *testing.T) {
	r := newRepo(t)
	r.git("config", "multistage.syncIndex", "false")
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-a", "commit", "-m", "a")
	os.Remove(r.path(".git/multistage/index-sync"))
	doc := r.mustMS("", "doctor")
	if !strings.Contains(doc.out, "behind") {
		t.Fatalf("doctor:\n%s", doc)
	}
	r.mustMS("", "sync")
	r.checkClean()
}
