package test

import (
	"os"
	"strings"
	"testing"
)

// twenty is a file of twenty lines.
var twenty = lines(20, func(i int) string { return "line " + string(rune('a'+i)) })

// heavyEdit rewrites lines 6 to 20 of content, so that Git's rename
// detection no longer pairs the file with its old version.
func heavyEdit(content string) string {
	ls := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for i := 5; i < len(ls); i++ {
		ls[i] = "rewritten " + ls[i]
	}
	return strings.Join(ls, "\n") + "\n"
}

// stageLineOne stages in agent a change of the first line of p, and leaves
// the disk as it was.
func (r *repo) stageLineOne(agent, p string) string {
	r.t.Helper()
	orig := r.read(p)
	changed := "FIRST\n" + strings.SplitN(orig, "\n", 2)[1]
	r.write(p, changed)
	r.mustMS(agent, "add", p)
	r.write(p, orig)
	return changed
}

// moveAndRewrite has agent move oldp to newp, rewrite it heavily, and commit.
func (r *repo) moveAndRewrite(agent, oldp, newp string, rewrite bool) {
	r.t.Helper()
	r.mustMS(agent, "mv", oldp, newp)
	if rewrite {
		r.write(newp, heavyEdit(r.read(newp)))
		r.mustMS(agent, "add", newp)
	}
	r.mustMS(agent, "commit", "-m", "move "+oldp)
}

func TestCommitAMove(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": "x\n"})
	r.mustMS("a", "mv", "x.go", "y.go")
	r.mustMS("a", "commit", "-m", "Move x\n\nBody.")
	msg := strings.TrimSpace(r.git("log", "-1", "--format=%B"))
	if !strings.HasSuffix(msg, "\nMultistage-Moved: x.go -> y.go") {
		t.Fatalf("message:\n%s", msg)
	}
	if got := r.stageMoves("a"); len(got) != 0 {
		t.Fatalf("the commit kept the moves %v", got)
	}
	r.write("y.go", "x\ny\n")
	r.mustMS("a", "add", "y.go")
	r.mustMS("a", "commit", "-m", "Edit y")
	if msg := r.git("log", "-1", "--format=%B"); strings.Contains(msg, "Multistage-Moved") {
		t.Fatalf("a later commit carries the trailer:\n%s", msg)
	}
	r.checkClean()
}

func TestCommitFollowsAnotherStagesMove(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	r.stageLineOne("a", "x.go")
	r.moveAndRewrite("b", "x.go", "y.go", true)
	r.mustMS("a", "commit", "-m", "first line")
	if _, err := r.gitTry("cat-file", "-e", "main:x.go"); err == nil {
		t.Fatal("the commit created x.go")
	}
	if got, want := r.git("show", "main:y.go"), "FIRST\n"+strings.SplitN(heavyEdit(twenty), "\n", 2)[1]; got != want {
		t.Fatalf("y.go = %q, want %q", got, want)
	}
	if !eq(r.changedPaths("main"), []string{"y.go"}) {
		t.Fatalf("the commit changed %v", r.changedPaths("main"))
	}
}

// TestCommitWithoutTrailerMissesAHeavyMove checks that the heavy edit of the
// tests above defeats Git's rename detection, so that they test the trailers.
func TestCommitWithoutTrailerMissesAHeavyMove(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	r.stageLineOne("a", "x.go")
	r.externalCommit("move x without a trailer", func(idx string) {
		env := append(os.Environ(), "GIT_INDEX_FILE="+idx)
		blob := r.hashObject(heavyEdit(twenty))
		for _, args := range [][]string{
			{"update-index", "--force-remove", "x.go"},
			{"update-index", "--add", "--cacheinfo", "100644," + blob + ",y.go"},
		} {
			if out, err := gitCmd(r.dir, env, args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	})
	r.expectCode(3, "a", "commit", "-m", "first line")
}

func (r *repo) hashObject(content string) string {
	r.t.Helper()
	cmd := gitCmd(r.dir, os.Environ(), "hash-object", "-w", "--stdin")
	cmd.Stdin = strings.NewReader(content)
	out, err := cmd.Output()
	if err != nil {
		r.t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestCommitFollowsAChainOfMoves(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	r.stageLineOne("a", "x.go")
	r.moveAndRewrite("b", "x.go", "y.go", true)
	r.moveAndRewrite("c", "y.go", "d/z.go", false)
	r.mustMS("a", "commit", "-m", "first line")
	if !eq(r.changedPaths("main"), []string{"d/z.go"}) {
		t.Fatalf("the commit changed %v", r.changedPaths("main"))
	}
	if got := r.git("show", "main:d/z.go"); !strings.HasPrefix(got, "FIRST\n") || !strings.Contains(got, "rewritten") {
		t.Fatalf("d/z.go = %q", got)
	}
}

// deleteAndCommit has agent b delete p and commit, and returns the commit.
func (r *repo) deleteAndCommit(p string) string {
	r.t.Helper()
	r.mustMS("b", "rm", p)
	r.mustMS("b", "commit", "-m", "delete "+p)
	return strings.TrimSpace(r.git("log", "-1", "--format=%h"))
}

func TestCommitOfAFileThatAnotherStageDeleted(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty, "other.go": "o\n"})
	mine := r.stageLineOne("a", "x.go")
	deleting := r.deleteAndCommit("x.go")
	head := r.git("rev-parse", "main")
	res := r.expectCode(3, "a", "commit", "-m", "first line")
	if !strings.Contains(res.err, "x.go was deleted by "+deleting) || !strings.Contains(res.err, "--recreate") ||
		!strings.Contains(res.err, "restore --staged") {
		t.Fatalf("error:\n%s", res)
	}
	var out struct {
		Error struct {
			Kind  string
			Paths []string
		} `json:"error"`
	}
	decode(t, r.expectCode(3, "a", "commit", "-m", "first line", "--json"), &out)
	if out.Error.Kind != "path-deleted" || !eq(out.Error.Paths, []string{"x.go"}) {
		t.Fatalf("error %+v", out.Error)
	}
	if r.git("rev-parse", "main") != head {
		t.Fatal("a commit landed")
	}
	if got := r.stagedPaths("a"); !eq(got, []string{"x.go"}) {
		t.Fatalf("stage a holds %v", got)
	}
	r.mustMS("a", "commit", "--recreate", "x.go", "-m", "Keep x")
	if got := r.git("show", "main:x.go"); got != mine {
		t.Fatalf("x.go = %q, want %q", got, mine)
	}
	if !eq(r.changedPaths("main"), []string{"x.go"}) {
		t.Fatalf("the commit changed %v", r.changedPaths("main"))
	}
}

func TestRefreshFollowsAMove(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	r.stageLineOne("a", "x.go")
	r.moveAndRewrite("b", "x.go", "y.go", true)
	res := r.mustMS("a", "status")
	if strings.Contains(res.err, "behind") {
		t.Fatalf("status warns:\n%s", res)
	}
	if got := r.stagedStatus("a"); !eq(got, []string{"M y.go"}) {
		t.Fatalf("stage a holds %v", got)
	}
	if d := r.mustMS("a", "diff", "--cached").out; !strings.Contains(d, "+FIRST") || strings.Contains(d, "rewritten") {
		t.Fatalf("staged diff:\n%s", d)
	}
}

func TestRefreshMeetsADeletion(t *testing.T) {
	r := committedRepo(t, map[string]string{"x.go": twenty})
	r.stageLineOne("a", "x.go")
	deleting := r.deleteAndCommit("x.go")
	res := r.mustMS("a", "status")
	if !strings.Contains(res.err, "behind") || !strings.Contains(res.err, "x.go was deleted by "+deleting) {
		t.Fatalf("status:\n%s", res)
	}
	if got := r.stagedPaths("a"); !eq(got, []string{"x.go"}) {
		t.Fatalf("stage a holds %v", got)
	}
}

func TestCommitFollowsSeveralMovesInOneCommit(t *testing.T) {
	r := committedRepo(t, map[string]string{"a.go": twenty, "c.go": strings.ReplaceAll(twenty, "line", "cc")})
	mine := r.stageLineOne("b", "a.go")
	r.mustMS("a", "mv", "a.go", "b.go")
	r.mustMS("a", "mv", "c.go", "d.go")
	r.mustMS("a", "mv", "b.go", "c.go")
	r.mustMS("a", "commit", "-m", "shuffle")
	msg := r.git("log", "-1", "--format=%B")
	if !strings.Contains(msg, "Multistage-Moved: a.go -> c.go") || !strings.Contains(msg, "Multistage-Moved: c.go -> d.go") {
		t.Fatalf("message:\n%s", msg)
	}
	r.mustMS("b", "commit", "-m", "first line of a")
	if !eq(r.changedPaths("main"), []string{"c.go"}) {
		t.Fatalf("the commit changed %v", r.changedPaths("main"))
	}
	if got := r.git("show", "main:c.go"); got != mine {
		t.Fatalf("c.go = %q, want %q", got, mine)
	}
}

func TestCommitAMoveOfAPathWithAnArrow(t *testing.T) {
	const odd = "p -> q.go"
	r := committedRepo(t, map[string]string{odd: twenty})
	r.stageLineOne("b", odd)
	r.moveAndRewrite("a", odd, "r.go", true)
	msg := strings.TrimSpace(r.git("log", "-1", "--format=%B"))
	if !strings.HasSuffix(msg, `Multistage-Moved: "p -> q.go" -> r.go`) {
		t.Fatalf("message:\n%s", msg)
	}
	r.mustMS("b", "commit", "-m", "first line")
	if !eq(r.changedPaths("main"), []string{"r.go"}) {
		t.Fatalf("the commit changed %v", r.changedPaths("main"))
	}
}

func TestCommitKeepsANewFileAtAMovedPath(t *testing.T) {
	r := committedRepo(t, map[string]string{"other.go": "o\n"})
	r.write("x.go", "mine\n")
	r.mustMS("a", "add", "x.go")
	r.externalCommit("add x", func(idx string) {
		env := append(os.Environ(), "GIT_INDEX_FILE="+idx)
		blob := r.hashObject("theirs\n")
		if out, err := gitCmd(r.dir, env, "update-index", "--add", "--cacheinfo", "100644,"+blob+",x.go").CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	})
	r.externalCommit("move x\n\nMultistage-Moved: x.go -> y.go", func(idx string) {
		env := append(os.Environ(), "GIT_INDEX_FILE="+idx)
		blob := r.hashObject("theirs\n")
		for _, args := range [][]string{
			{"update-index", "--force-remove", "x.go"},
			{"update-index", "--add", "--cacheinfo", "100644," + blob + ",y.go"},
		} {
			if out, err := gitCmd(r.dir, env, args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	})
	r.mustMS("a", "commit", "-m", "add x")
	if got := r.git("show", "main:x.go"); got != "mine\n" {
		t.Fatalf("x.go = %q", got)
	}
	if got := r.git("show", "main:y.go"); got != "theirs\n" {
		t.Fatalf("y.go = %q", got)
	}
}
