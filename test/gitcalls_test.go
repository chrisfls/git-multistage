package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// traceCalls runs git-multistage with Git's trace2 events on and returns the
// subcommand of each Git process that it started, in start order.
func (r *repo) traceCalls(agent string, args ...string) []string {
	r.t.Helper()
	trace := filepath.Join(r.t.TempDir(), "trace")
	if res := run(r.dir, agent, []string{"GIT_TRACE2_EVENT=" + trace}, args...); res.code != 0 {
		r.t.Fatalf("git-multistage %s:\n%s", strings.Join(args, " "), res)
	}
	return readTrace(r.t, trace)
}

// readTrace returns the subcommand of each Git process in a file of Git's
// trace2 events, in start order.
func readTrace(tb testing.TB, trace string) []string {
	tb.Helper()
	data, err := os.ReadFile(trace)
	if err != nil {
		tb.Fatal(err)
	}
	var subs []string
	for _, line := range strings.Split(string(data), "\n") {
		var e struct {
			Event string
			Argv  []string
		}
		if json.Unmarshal([]byte(line), &e) != nil || e.Event != "start" {
			continue
		}
		for i := 1; i < len(e.Argv); i++ {
			if e.Argv[i] == "-c" {
				i++
			} else if !strings.HasPrefix(e.Argv[i], "-") {
				subs = append(subs, e.Argv[i])
				break
			}
		}
	}
	return subs
}

func countCalls(subs []string, name string) (n int) {
	for _, s := range subs {
		if s == name {
			n++
		}
	}
	return n
}

// TestUnchangedStageReadsNoKeepCommitAndNoTree checks that status and diff
// --cached of an unchanged stage start no show of the keep ref and no
// write-tree, from the second command on.
func TestUnchangedStageReadsNoKeepCommitAndNoTree(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("new.txt", "n\n")
	r.mustMS("agent-a", "add", "new.txt")
	r.mustMS("agent-a", "status")
	for _, args := range [][]string{{"status"}, {"diff", "--cached"}, {"status"}} {
		subs := r.traceCalls("agent-a", args...)
		for _, name := range []string{"show", "write-tree", "mktree", "commit-tree", "update-ref"} {
			if n := countCalls(subs, name); n != 0 {
				t.Errorf("%v on an unchanged stage started %d %s: %v", args, n, name, subs)
			}
		}
	}
}

// TestCommitJSONListsStagedFilesWithRenames checks that `commit --json`
// lists the staged changes as `git show -M` does: a rename, an edit, an
// addition, and a deletion.
func TestCommitJSONListsStagedFilesWithRenames(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	long := lines(20, func(i int) string { return "line " + strings.Repeat("x", i) })
	for _, p := range []string{"old.txt", "edit.txt", "gone.txt", "dir/in.txt"} {
		r.write(p, p+"\n"+long)
	}
	r.git("add", ".")
	r.git("commit", "-q", "-m", "files")
	r.mustMS("agent-a", "mv", "old.txt", "moved/new.txt")
	r.mustMS("agent-a", "mv", "dir/in.txt", "out.txt")
	r.write("edit.txt", "edit.txt\n"+long+"more\n")
	r.write("added.txt", "added\n")
	r.mustMS("agent-a", "add", "edit.txt", "added.txt")
	r.mustMS("agent-a", "rm", "gone.txt")
	res := r.mustMS("agent-a", "commit", "--json", "-m", "mixed")
	var got struct {
		Commit struct {
			Files []struct {
				Status  string `json:"status"`
				Path    string `json:"path"`
				OldPath string `json:"old_path"`
			} `json:"files"`
		} `json:"commit"`
	}
	decode(t, res, &got)
	var gotLines []string
	for _, f := range got.Commit.Files {
		if f.OldPath != "" {
			gotLines = append(gotLines, f.Status+"\t"+f.OldPath+"\t"+f.Path)
		} else {
			gotLines = append(gotLines, f.Status+"\t"+f.Path)
		}
	}
	var want []string
	for _, l := range strings.Split(strings.TrimSpace(r.git("show", "--name-status", "-M", "--format=", "HEAD")), "\n") {
		parts := strings.Split(l, "\t")
		parts[0] = parts[0][:1]
		want = append(want, strings.Join(parts, "\t"))
	}
	if !eq(gotLines, want) || len(want) != 5 {
		t.Fatalf("commit --json files %q, git show -M %q", gotLines, want)
	}
}

// TestCommitStartsNoReadsAfterTheBranchMoves checks that a commit whose tree
// is the staged tree starts no rev-parse after update-ref, and no diff-tree,
// read-tree, write-tree, or diff-index at all, once the stage and the default
// index have remembered trees.
func TestCommitStartsNoReadsAfterTheBranchMoves(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f1.txt", "a\nb\nc\n")
	r.write("f2.txt", "a\nb\nc\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "files")
	for i, content := range []string{"x\n", "y\n"} {
		r.write("f1.txt", content)
		r.mustMS("agent-a", "add", "f1.txt")
		r.mustMS("agent-a", "status")
		subs := r.traceCalls("agent-a", "commit", "-m", "c"+string(rune('0'+i)))
		moved := slices.Index(subs, "update-ref")
		if moved < 0 {
			t.Fatalf("commit %d started no update-ref: %v", i, subs)
		}
		for _, name := range []string{"read-tree", "write-tree", "diff-index"} {
			if n := countCalls(subs, name); n != 0 {
				t.Errorf("commit %d started %d %s: %v", i, n, name, subs)
			}
		}
		if n := countCalls(subs, "diff-tree"); n != 1 {
			t.Errorf("commit %d started %d diff-tree, want the one of the staged changes: %v", i, n, subs)
		}
		for _, name := range []string{"rev-parse", "diff-tree", "symbolic-ref"} {
			if slices.Contains(subs[moved+1:], name) {
				t.Errorf("commit %d started %s after update-ref: %v", i, name, subs)
			}
		}
		r.checkIndexSynced()
		r.checkHealthy()
		if staged := r.stagedPaths("agent-a"); len(staged) != 0 {
			t.Errorf("commit %d left %v staged", i, staged)
		}
	}
}

// TestSyncAfterACommitKeepsItsResults checks the default index after a commit
// whose sync derives the tree of the index: an entry that someone staged by
// hand is kept, an intent-to-add entry for an empty file that the commit
// adds stays, and an intent-to-add entry at another path stays out of the
// commit.
func TestSyncAfterACommitKeepsItsResults(t *testing.T) {
	t.Parallel()
	t.Run("entry staged by hand", func(t *testing.T) {
		t.Parallel()
		r := newRepo(t)
		r.write("f1.txt", "a\nb\nc\n")
		r.write("f2.txt", "a\nb\nc\n")
		r.git("add", ".")
		r.git("commit", "-q", "-m", "files")
		r.write("f1.txt", "a\nX\nc\n")
		r.write("f2.txt", "a\nY\nc\n")
		r.mustMS("agent-a", "add", "f1.txt", "f2.txt")
		r.write("f1.txt", "a\nNATIVE\nc\n")
		r.git("add", "f1.txt")
		r.write("f1.txt", "a\nX\nc\n")
		res := r.mustMS("agent-a", "commit", "--json", "-m", "one")
		var got struct {
			Commit struct {
				Sync struct {
					Updated, Kept []string
				} `json:"index_sync"`
			} `json:"commit"`
		}
		decode(t, res, &got)
		if !eq(got.Commit.Sync.Updated, []string{"f2.txt"}) || !eq(got.Commit.Sync.Kept, []string{"f1.txt"}) {
			t.Fatalf("sync updated %v and kept %v", got.Commit.Sync.Updated, got.Commit.Sync.Kept)
		}
		if got := r.git("status", "--porcelain"); got != "MM f1.txt\n" {
			t.Fatalf("status after the commit: %q", got)
		}
		if got := r.git("show", ":f1.txt"); got != "a\nNATIVE\nc\n" {
			t.Fatalf("the hand-staged entry holds %q", got)
		}
		if got := r.git("show", ":f2.txt"); got != "a\nY\nc\n" {
			t.Fatalf("the synced entry holds %q", got)
		}
	})
	t.Run("intent-to-add entry of an empty file that the commit adds", func(t *testing.T) {
		t.Parallel()
		r := newRepo(t)
		r.write("empty.txt", "")
		r.git("add", "-N", "empty.txt")
		r.mustMS("agent-a", "add", "empty.txt")
		r.write("f.txt", "f\n")
		r.mustMS("agent-a", "add", "f.txt")
		r.mustMS("agent-a", "commit", "-m", "two")
		if got := r.git("status", "--porcelain"); got != "DA empty.txt\n" {
			t.Fatalf("status after the commit: %q", got)
		}
		if got := r.git("ls-tree", "-r", "--name-only", "HEAD"); got != "README\nempty.txt\nf.txt\n" {
			t.Fatalf("the commit holds %q", got)
		}
	})
	t.Run("intent-to-add entry at another path", func(t *testing.T) {
		t.Parallel()
		r := newRepo(t)
		r.write("other.txt", "n\n")
		r.git("add", "-N", "other.txt")
		r.write("f.txt", "f\n")
		r.mustMS("agent-a", "add", "f.txt")
		r.mustMS("agent-a", "commit", "-m", "three")
		if got := r.git("status", "--porcelain"); got != " A other.txt\n" {
			t.Fatalf("status after the commit: %q", got)
		}
	})
	t.Run("sparse checkout", func(t *testing.T) {
		t.Parallel()
		r := newRepo(t)
		r.write("in/a.txt", "a\n")
		r.write("out/b.txt", "b\n")
		r.git("add", ".")
		r.git("commit", "-q", "-m", "dirs")
		r.git("sparse-checkout", "set", "--cone", "in")
		r.write("in/a.txt", "a2\n")
		r.mustMS("agent-a", "add", "in/a.txt")
		r.mustMS("agent-a", "commit", "-m", "sparse")
		if got := r.git("ls-files", "-t", "out/b.txt"); got != "S out/b.txt\n" {
			t.Fatalf("out/b.txt in the default index: %q", got)
		}
		if got := r.git("status", "--porcelain"); got != "" {
			t.Fatalf("status after the commit: %q", got)
		}
		r.checkIndexSynced()
	})
}

// TestStageCommandsReadRefsFromFiles checks that status and commit read HEAD
// and the branch tip from the ref files, and that a command after the first
// one remembers the version check: they start no rev-parse other than the
// setup call, no symbolic-ref, and no git version.
func TestStageCommandsReadRefsFromFiles(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("new.txt", "n\n")
	r.mustMS("agent-a", "add", "new.txt")
	for _, args := range [][]string{{"status"}, {"commit", "-m", "x"}} {
		subs := r.traceCalls("agent-a", args...)
		if n := countCalls(subs, "rev-parse"); n != 1 {
			t.Errorf("%v started %d rev-parse, want only the setup call: %v", args, n, subs)
		}
		for _, name := range []string{"symbolic-ref", "version"} {
			if n := countCalls(subs, name); n != 0 {
				t.Errorf("%v started %d %s: %v", args, n, name, subs)
			}
		}
	}
}

// TestIndexReadsStartNoLsFiles checks that the edit hooks, add --edits, add
// of a tracked and of a new file, and commit read index entries and index
// spellings from the index file: with core.ignoreCase set, they start no
// ls-files.
func TestIndexReadsStartNoLsFiles(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.git("config", "core.ignoreCase", "true")
	r.write("f.txt", "1\n")
	r.write("g.txt", "1\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "files")
	hook := func(event string) []string {
		t.Helper()
		trace := filepath.Join(t.TempDir(), "trace")
		if res := r.hookCall(event, "Edit", "s", "", r.path("f.txt"), "GIT_TRACE2_EVENT="+trace); res.code != 0 {
			t.Fatalf("%s:\n%s", event, res)
		}
		return readTrace(t, trace)
	}
	calls := map[string][]string{"PreToolUse": hook("PreToolUse")}
	r.write("f.txt", "2\n")
	calls["PostToolUse"] = hook("PostToolUse")
	calls["add --edits"] = r.traceCalls("claude-s", "add", "--edits")
	r.write("g.txt", "2\n")
	calls["add g.txt"] = r.traceCalls("claude-s", "add", "g.txt")
	r.write("new.txt", "n\n")
	calls["add new.txt"] = r.traceCalls("claude-s", "add", "new.txt")
	calls["commit"] = r.traceCalls("claude-s", "commit", "-m", "x")
	for name, subs := range calls {
		if n := countCalls(subs, "ls-files"); n != 0 {
			t.Errorf("%s started %d ls-files: %v", name, n, subs)
		}
	}
	if got := r.changedPaths("HEAD"); !eq(got, []string{"f.txt", "g.txt", "new.txt"}) {
		t.Fatalf("the commit changed %v", got)
	}
}
