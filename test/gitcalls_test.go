package test

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	data, err := os.ReadFile(trace)
	if err != nil {
		r.t.Fatal(err)
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
