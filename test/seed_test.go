package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// traceArgs runs git-multistage with Git's trace2 events on and returns the
// arguments of each Git process that it started, without the -c options.
func (r *repo) traceArgs(agent string, args ...string) [][]string {
	r.t.Helper()
	trace := filepath.Join(r.t.TempDir(), "trace")
	if res := run(r.dir, agent, []string{"GIT_TRACE2_EVENT=" + trace}, args...); res.code != 0 {
		r.t.Fatalf("git-multistage %s:\n%s", strings.Join(args, " "), res)
	}
	data, err := os.ReadFile(trace)
	if err != nil {
		r.t.Fatal(err)
	}
	var calls [][]string
	for _, line := range strings.Split(string(data), "\n") {
		var e struct {
			Event string
			Argv  []string
		}
		if json.Unmarshal([]byte(line), &e) != nil || e.Event != "start" {
			continue
		}
		var call []string
		for i := 1; i < len(e.Argv); i++ {
			if e.Argv[i] == "-c" {
				i++
			} else {
				call = append(call, e.Argv[i])
			}
		}
		calls = append(calls, call)
	}
	return calls
}

// readTreeResets reports whether calls hold a `read-tree --reset`.
func readTreeResets(calls [][]string) bool {
	return slices.ContainsFunc(calls, func(c []string) bool {
		return len(c) > 1 && c[0] == "read-tree" && c[1] == "--reset"
	})
}

// stageIndex returns the path of the current index file of a stage.
func (r *repo) stageIndex(stage string) string {
	r.t.Helper()
	var m struct {
		Index string `json:"index"`
	}
	data, err := os.ReadFile(r.path(".git/multistage/stages/" + stage + "/meta.json"))
	if err != nil {
		r.t.Fatal(err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		r.t.Fatal(err)
	}
	return r.path(".git/multistage/stages/" + stage + "/" + m.Index)
}

// stageGit runs git with the index of a stage.
func (r *repo) stageGit(stage string, args ...string) string {
	r.t.Helper()
	out, err := gitCmd(r.dir, []string{"GIT_INDEX_FILE=" + r.stageIndex(stage)}, args...).CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func unstagedPaths(r *repo, agent string) []string {
	r.t.Helper()
	var st statusJSON
	decode(r.t, r.mustMS(agent, "status", "--json"), &st)
	var paths []string
	for _, u := range st.Status.Unstaged {
		paths = append(paths, u.Path)
	}
	return paths
}

// TestBaseMoveStartsFromTheStageIndex moves the base of a stage with a
// staged change after another stage commits. The new index starts from the
// stage's index, and holds the staged change and the committed file.
func TestBaseMoveStartsFromTheStageIndex(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.write("b.txt", "b\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "files")
	r.write("a.txt", "a2\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.write("b.txt", "b2\n")
	r.mustMS("agent-b", "add", "b.txt")
	r.mustMS("agent-b", "commit", "-m", "b2")
	if calls := r.traceArgs("agent-a", "status"); !readTreeResets(calls) {
		t.Fatalf("the base move ran no read-tree --reset: %v", calls)
	}
	if got := r.stagedPaths("agent-a"); !eq(got, []string{"a.txt"}) {
		t.Fatalf("staged %v, want [a.txt]", got)
	}
	if got := unstagedPaths(r, "agent-a"); len(got) != 0 {
		t.Fatalf("unstaged %v after the base move", got)
	}
	if got := r.stageGit("agent-a", "show", ":b.txt"); got != "b2\n" {
		t.Fatalf("the stage index holds b.txt = %q", got)
	}
	r.mustMS("agent-a", "commit", "-m", "a2")
	if got := r.git("show", "main:a.txt") + r.git("show", "main:b.txt"); got != "a2\nb2\n" {
		t.Fatalf("main holds %q", got)
	}
	r.checkHealthy()
}

// TestNewStageStartsFromAnotherStageIndex creates a stage while another
// stage has a staged change. The new index starts from the other stage's
// index and holds none of its staged changes.
func TestNewStageStartsFromAnotherStageIndex(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.git("add", "a.txt")
	r.git("commit", "-q", "-m", "a")
	r.write("a.txt", "a2\n")
	r.mustMS("agent-a", "add", "a.txt")
	if calls := r.traceArgs("agent-b", "status"); !readTreeResets(calls) {
		t.Fatalf("the new stage ran no read-tree --reset: %v", calls)
	}
	if got := r.stagedPaths("agent-b"); len(got) != 0 {
		t.Fatalf("the new stage holds staged changes %v", got)
	}
	if got := unstagedPaths(r, "agent-b"); !eq(got, []string{"a.txt"}) {
		t.Fatalf("unstaged %v, want [a.txt]", got)
	}
}

// TestRacyEntryOfTheSeedShowsAsChanged makes the seed's entry of f.txt
// racily clean: the entry, the seed's index file, and the file all get one
// time an hour ago, and the file gets new content of the same size. Git
// must still see the change in the new stage, which needs the copy of the
// seed to keep the seed's modification time. The seed stages g.txt, so the
// new index needs read-tree --reset.
func TestRacyEntryOfTheSeedShowsAsChanged(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.git("config", "core.trustctime", "false")
	r.write("f.txt", "aaaa\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(r.path("f.txt"), past, past); err != nil {
		t.Fatal(err)
	}
	r.write("g.txt", "g\n")
	r.mustMS("agent-a", "add", "g.txt")
	if err := os.Chtimes(r.stageIndex("agent-a"), past, past); err != nil {
		t.Fatal(err)
	}
	r.write("f.txt", "bbbb\n")
	if err := os.Chtimes(r.path("f.txt"), past, past); err != nil {
		t.Fatal(err)
	}
	if calls := r.traceArgs("agent-b", "status"); !readTreeResets(calls) {
		t.Fatalf("the new stage ran no read-tree --reset: %v", calls)
	}
	if got := unstagedPaths(r, "agent-b"); !eq(got, []string{"f.txt"}) {
		t.Fatalf("unstaged %v, want [f.txt]", got)
	}
}

// TestNoSkipWorktreeBitsAfterSparseCheckoutEnds ends a sparse checkout after
// a stage marked an entry skip-worktree. Neither a new stage nor the base
// move of that stage keeps the bit.
func TestNoSkipWorktreeBitsAfterSparseCheckoutEnds(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.sparseSetup(false)
	r.mustMS("agent-a", "status")
	if got := r.stageGit("agent-a", "ls-files", "-t", "d2/y.txt"); !strings.HasPrefix(got, "S ") {
		t.Fatalf("setup: d2/y.txt has no skip-worktree bit: %q", got)
	}
	r.git("sparse-checkout", "disable")
	r.mustMS("agent-b", "status")
	r.write("d1/x.txt", "x2\n")
	r.mustMS("agent-c", "add", "d1/x.txt")
	r.mustMS("agent-c", "commit", "-m", "x2")
	r.mustMS("agent-a", "status")
	for _, stage := range []string{"agent-a", "agent-b"} {
		if got := r.stageGit(stage, "ls-files", "-t"); strings.Contains("\n"+got, "\nS ") {
			t.Errorf("%s keeps skip-worktree bits:\n%s", stage, got)
		}
	}
}

// TestAssumeUnchangedOfTheDefaultIndexStaysOutOfANewStage marks a file
// assume-unchanged in the default index and changes it. A new stage sees the
// change.
func TestAssumeUnchangedOfTheDefaultIndexStaysOutOfANewStage(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "f\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.mustMS("agent-a", "status")
	r.git("update-index", "--assume-unchanged", "f.txt")
	r.write("f.txt", "f2\n")
	if got := unstagedPaths(r, "agent-b"); !eq(got, []string{"f.txt"}) {
		t.Fatalf("unstaged %v, want [f.txt]", got)
	}
	if got := r.stageGit("agent-b", "ls-files", "-v", "f.txt"); got != "H f.txt\n" {
		t.Fatalf("ls-files -v of the new stage: %q", got)
	}
}
