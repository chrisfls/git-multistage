package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests reproduce defects found in an adversarial review.

func TestSyncAfterNativeCommitKeepsAgentChange(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("p.txt", lines(10, func(i int) string { return "l" + itoa(i) }))
	r.git("add", "p.txt")
	r.git("commit", "-q", "-m", "p")
	r.mustMS("agent-a", "status") // the stage exists before the native commit
	r.write("p.txt", strings.Replace(r.read("p.txt"), "l2\n", "HUMAN\n", 1))
	r.git("commit", "-q", "-am", "native")
	r.write("p.txt", strings.Replace(r.read("p.txt"), "l7\n", "AGENT\n", 1))
	r.mustMS("agent-a", "add", "p.txt", "--hunk", r.hunkID("agent-a", "p.txt", "AGENT"))
	res := r.mustMS("agent-a", "commit", "-m", "agent")
	if strings.Contains(res.err, "natively staged") {
		t.Fatalf("spurious warning:\n%s", res)
	}
	r.checkClean()
}

func TestSyncKeepsNativeUnstagedRemoval(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.mustMS("agent-a", "status")
	r.write("secret.txt", "secret\n")
	r.git("add", "secret.txt")
	r.git("commit", "-q", "-m", "secret")
	r.git("rm", "-q", "--cached", "secret.txt")
	r.write("b.txt", "b\n")
	r.mustMS("agent-a", "add", "b.txt")
	r.mustMS("agent-a", "commit", "-m", "b")
	if out := r.git("ls-files", "secret.txt"); out != "" {
		t.Fatal("the sync re-added a file that was removed from the index natively")
	}
}

func TestPreCommitHookCannotPullInUnstagedEdits(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", lines(10, func(i int) string { return "l" + itoa(i) }))
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.hook("pre-commit", "git diff --cached --name-only --diff-filter=ACM | xargs git add --\n")
	r.write("f.txt", strings.Replace(r.read("f.txt"), "l2\n", "AGENT-A\n", 1))
	id := r.hunkID("agent-a", "f.txt", "AGENT-A")
	r.write("f.txt", strings.Replace(r.read("f.txt"), "l7\n", "AGENT-B\n", 1))
	r.mustMS("agent-a", "add", "f.txt", "--hunk", id)
	res := r.expectCode(1, "agent-a", "commit", "-m", "a")
	if !strings.Contains(res.err, "hook changed the staged content") {
		t.Fatalf("error:\n%s", res)
	}
	if strings.Contains(r.git("show", "main:f.txt"), "AGENT-B") || len(r.commits()) != 2 {
		t.Fatal("agent-b's unstaged edit entered a commit")
	}
	if diff := r.mustMS("agent-a", "diff", "--cached").out; !strings.Contains(diff, "+AGENT-A") || strings.Contains(diff, "AGENT-B") {
		t.Fatalf("staged diff:\n%s", diff)
	}
}

func TestHooksSeeOrdinaryPathspecs(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.hook("pre-commit", "if git diff --cached --name-only -- '*.js' | grep -q .; then echo lint failed >&2; exit 1; fi\n")
	r.write("bad.js", "x\n")
	r.mustMS("agent-a", "add", "bad.js")
	res := r.expectCode(1, "agent-a", "commit", "-m", "js")
	if !strings.Contains(res.err, "lint failed") {
		t.Fatalf("hook gate skipped:\n%s", res)
	}
	if out := r.mustMS("agent-a", "diff", "--cached", "--name-only", "--", "*.js").out; out != "bad.js\n" {
		t.Fatalf("diff glob = %q", out)
	}
}

func TestGarbageCollectionKeepsStagedObjects(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("new.txt", "unique staged content 7f3a\n")
	r.mustMS("agent-a", "add", "new.txt")
	if err := os.Remove(r.path("new.txt")); err != nil {
		t.Fatal(err)
	}
	r.git("gc", "-q", "--prune=now")
	r.mustMS("agent-a", "status")
	r.mustMS("agent-a", "commit", "-m", "new")
	if r.git("show", "main:new.txt") != "unique staged content 7f3a\n" {
		t.Fatal("staged content lost")
	}

	// A journaled commit that never landed may be pruned; recovery still works.
	r.write("b.txt", "b\n")
	r.mustMS("agent-b", "add", "b.txt")
	if res := run(r.dir, "agent-b", []string{"GIT_MULTISTAGE_FAULT=before-update-ref"}, "commit", "-m", "b"); res.code != 86 {
		t.Fatalf("fault did not fire:\n%s", res)
	}
	r.git("gc", "-q", "--prune=now")
	r.mustMS("agent-b", "commit", "-m", "b")
	if !eq(r.changedPaths("main"), []string{"b.txt"}) {
		t.Fatalf("commit changed %v", r.changedPaths("main"))
	}
}

// TestProtectSkipsAnUnchangedKeepTree leaves the keep ref of an unchanged
// stage alone without mktree, and rewrites a keep commit of an older version
// once.
func TestProtectSkipsAnUnchangedKeepTree(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("new.txt", "n\n")
	r.mustMS("agent-a", "add", "new.txt")
	const keep = "refs/worktree/multistage/agent-a"
	if msg := r.git("log", "-1", "--format=%B", keep); !strings.Contains(msg, "\nkeep-input ") {
		t.Fatalf("keep commit message %q has no digest", msg)
	}
	// mktrees runs a command with Git's trace on and counts its mktree calls.
	mktrees := func(args ...string) int {
		t.Helper()
		trace := filepath.Join(t.TempDir(), "trace")
		if res := run(r.dir, "agent-a", []string{"GIT_TRACE2_EVENT=" + trace}, args...); res.code != 0 {
			t.Fatalf("%v:\n%s", args, res)
		}
		data, err := os.ReadFile(trace)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, `"event":"start"`) && strings.Contains(line, `"mktree"`) {
				n++
			}
		}
		return n
	}
	ref := strings.TrimSpace(r.git("rev-parse", keep))
	for _, args := range [][]string{{"status"}, {"diff", "--cached"}} {
		if n := mktrees(args...); n != 0 {
			t.Fatalf("%v on an unchanged stage started %d mktree", args, n)
		}
	}
	if now := strings.TrimSpace(r.git("rev-parse", keep)); now != ref {
		t.Fatalf("the keep ref moved from %s to %s", ref, now)
	}

	treeAndBase := r.git("rev-parse", keep+"^{tree}", keep+"^")
	tree, base, _ := strings.Cut(strings.TrimSpace(treeAndBase), "\n")
	old := strings.TrimSpace(r.git("commit-tree", tree, "-p", base, "-m", "git-multistage stage agent-a"))
	r.git("update-ref", keep, old)
	if n := mktrees("status"); n != 1 {
		t.Fatalf("status over a keep commit without the digest started %d mktree, want 1", n)
	}
	if got := r.git("rev-parse", keep+"^{tree}", keep+"^"); got != treeAndBase {
		t.Fatalf("the rewritten keep commit holds %q, want %q", got, treeAndBase)
	}
	if n := mktrees("status"); n != 0 {
		t.Fatalf("status after the rewrite started %d mktree", n)
	}
}

func caseInsensitiveFS(tb testing.TB, dir string) bool {
	tb.Helper()
	p := filepath.Join(dir, "CaseProbe")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = os.Remove(p) }()
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

func TestHunkPathWithOtherCase(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	if !caseInsensitiveFS(t, r.dir) {
		t.Skip("case-sensitive file system")
	}
	r.write("Foo.txt", "a\nb\nc\n")
	r.git("add", "Foo.txt")
	r.git("commit", "-q", "-m", "foo")
	r.write("Foo.txt", "a\nB\nc\n")
	r.mustMS("agent-a", "add", "foo.txt", "--hunk", r.hunkID("agent-a", "foo.txt", "B"))
	r.mustMS("agent-a", "commit", "-m", "case")
	r.write("Foo.txt", "a\nB\nC\n")
	r.mustMS("agent-a", "add", "foo.txt")
	if got := r.stagedPaths("agent-a"); !eq(got, []string{"Foo.txt"}) {
		t.Fatalf("whole-file add staged %v", got)
	}
	if got := strings.Fields(r.git("ls-tree", "--name-only", "main")); !eq(got, []string{"Foo.txt", "README"}) {
		t.Fatalf("tree = %v", got)
	}
}

func TestMissingStageIndexIsRefused(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	matches, _ := filepath.Glob(r.path(".git/multistage/stages/agent-a/index-*"))
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			t.Fatal(err)
		}
	}
	r.write("d.txt", "d\n")
	if res := r.ms("agent-a", "add", "d.txt"); res.code == 0 {
		t.Fatalf("a stage without an index was used:\n%s", res)
	}
	if res := r.ms("agent-a", "commit", "-m", "d"); res.code == 0 {
		t.Fatalf("committed from a stage without an index:\n%s", res)
	}
	if len(r.commits()) != 1 {
		t.Fatal("a commit landed")
	}
	r.mustMS("agent-a", "drop", "--force")
	r.mustMS("agent-a", "add", "d.txt")
}

func TestInterHunkContextConfig(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.git("config", "diff.interHunkContext", "5")
	r.write("f.txt", "1\n2\n3\n4\n5\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.write("f.txt", "1\nTWO\n3\nFOUR\n5\n")
	r.mustMS("agent-a", "add", "f.txt", "--hunk", r.hunkID("agent-a", "f.txt", "TWO"))
	if got := r.git("show", ":f.txt"); got != "1\n2\n3\n4\n5\n" {
		t.Fatalf("default index touched: %q", got)
	}
}

func TestHunkStagingRespectsIgnoreRules(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write(".gitignore", ".env\n")
	r.write(".env", "TOKEN=x\n")
	var fj filesJSON
	decode(t, r.mustMS("agent-a", "hunks", "--json", ".env"), &fj)
	if len(fj.Files) != 1 || len(fj.Files[0].Hunks) != 1 {
		t.Fatalf("want one hunk of .env:\n%+v", fj)
	}
	id := fj.Files[0].Hunks[0].ID
	if res := r.ms("agent-a", "add", ".env", "--hunk", id); res.code == 0 {
		t.Fatalf("staged an ignored file:\n%s", res)
	}
	r.mustMS("agent-a", "add", "-f", ".env", "--hunk", id)
}

func TestDuplicateHunksCannotShift(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "a\nx\nb\nc\nd\nx\ne\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.write("f.txt", "a\ny\nb\nc\nd\ny\ne\n")
	var fj filesJSON
	decode(t, r.mustMS("agent-a", "hunks", "--json", "f.txt"), &fj)
	h := fj.Files[0].Hunks
	if len(h) != 2 {
		t.Fatalf("hunks: %+v", h)
	}
	// agent-b commits the first of the two identical hunks.
	r.mustMS("agent-b", "add", "f.txt", "--hunk", h[0].ID)
	r.mustMS("agent-b", "commit", "-m", "first")
	// agent-a's ID of the first hunk must not now select the second one.
	r.expectCode(4, "agent-a", "add", "f.txt", "--hunk", h[0].ID)
	r.mustMS("agent-a", "add", "f.txt", "--hunk", r.hunkID("agent-a", "f.txt", "y"))
}
