package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// iterations returns how many times the adversarial tests repeat, from
// MULTISTAGE_STRESS (default 1).
func iterations() int {
	if n, err := strconv.Atoi(os.Getenv("MULTISTAGE_STRESS")); err == nil && n > 0 {
		return n
	}
	return 1
}

// parallel runs f(i) for i in [0, n) at the same time, after a common start.
// writeFile writes a file and returns the error, for worker goroutines,
// which must not call t.Fatal.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func parallel(n int, f func(i int)) {
	var start, done sync.WaitGroup
	start.Add(1)
	for i := range n {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			f(i)
		}(i)
	}
	start.Done()
	done.Wait()
}

// commitBySubject maps each commit subject on main to its commit.
func (r *repo) commitBySubject() map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(r.git("log", "--format=%H %s", "main")), "\n") {
		oid, subject, _ := strings.Cut(line, " ")
		if _, dup := m[subject]; dup {
			r.t.Fatalf("duplicate commit subject %q", subject)
		}
		m[subject] = oid
	}
	return m
}

func TestParallelStagingOfDifferentFiles(t *testing.T) {
	r := newRepo(t)
	const n = 8
	for i := range n {
		r.write(fmt.Sprintf("f%d.txt", i), "content\n")
	}
	errs := make([]string, n)
	parallel(n, func(i int) {
		if res := r.ms(fmt.Sprintf("agent-%d", i), "add", fmt.Sprintf("f%d.txt", i)); res.code != 0 {
			errs[i] = res.String()
		}
	})
	for i := range n {
		if errs[i] != "" {
			t.Fatalf("agent-%d: %s", i, errs[i])
		}
		if got := r.stagedPaths(fmt.Sprintf("agent-%d", i)); !eq(got, []string{fmt.Sprintf("f%d.txt", i)}) {
			t.Fatalf("agent-%d staged %v", i, got)
		}
	}
}

// TestManyAgentsCommitConcurrently has several agents commit their own files
// to one branch at the same time, in several rounds, with random delays
// inside each commit.
func TestManyAgentsCommitConcurrently(t *testing.T) {
	for range iterations() {
		r := newRepo(t)
		const agents, rounds = 8, 3
		env := []string{"GIT_MULTISTAGE_TEST_DELAY_MS=15"}
		errs := make([]string, agents)
		parallel(agents, func(i int) {
			agent := fmt.Sprintf("agent-%d", i)
			for round := range rounds {
				p := fmt.Sprintf("%s/round-%d.txt", agent, round)
				if err := writeFile(r.path(p), fmt.Sprintf("%s %d\n", agent, round)); err != nil {
					errs[i] = err.Error()
					return
				}
				if res := run(r.dir, agent, env, "add", p); res.code != 0 {
					errs[i] = "add: " + res.String()
					return
				}
				if res := run(r.dir, agent, env, "commit", "-m", fmt.Sprintf("%s round %d", agent, round)); res.code != 0 {
					errs[i] = "commit: " + res.String()
					return
				}
			}
		})
		failOnWorkerErrors(t, "agent-%d", errs)
		bySubject := r.commitBySubject()
		if got := len(r.commits()); got != 1+agents*rounds {
			t.Fatalf("%d commits, want %d", got, 1+agents*rounds)
		}
		for i := range agents {
			for round := range rounds {
				subject := fmt.Sprintf("agent-%d round %d", i, round)
				c, ok := bySubject[subject]
				if !ok {
					t.Fatalf("commit %q is missing", subject)
				}
				want := []string{fmt.Sprintf("agent-%d/round-%d.txt", i, round)}
				if got := r.changedPaths(c); !eq(got, want) {
					t.Fatalf("commit %q changed %v, want %v", subject, got, want)
				}
			}
			if got := r.stagedPaths(fmt.Sprintf("agent-%d", i)); len(got) != 0 {
				t.Fatalf("agent-%d still has staged %v", i, got)
			}
		}
		r.checkHealthy()
		r.checkClean()
	}
}

// TestAgentsCommitDifferentHunksOfOneFile has agents edit separate sections
// of one file, stage only their hunk, and commit at the same time.
func TestAgentsCommitDifferentHunksOfOneFile(t *testing.T) {
	for range iterations() {
		r := newRepo(t)
		const agents = 5
		// Sections of four lines; each agent edits the first line of its section.
		r.write("shared.txt", lines(agents*4, func(i int) string { return "line " + strconv.Itoa(i) }))
		r.git("add", "shared.txt")
		r.git("commit", "-q", "-m", "shared")
		var fileMu sync.Mutex // agents never write the same file at the same moment
		errs := make([]string, agents)
		parallel(agents, func(i int) {
			agent := fmt.Sprintf("agent-%d", i)
			mine := "edited by " + agent
			fileMu.Lock()
			content, err := os.ReadFile(r.path("shared.txt"))
			if err == nil {
				err = writeFile(r.path("shared.txt"), strings.Replace(string(content), fmt.Sprintf("line %d\n", i*4+1), mine+"\n", 1))
			}
			fileMu.Unlock()
			if err != nil {
				errs[i] = err.Error()
				return
			}
			id, err := findHunk(r.dir, agent, "shared.txt", mine)
			if err != nil {
				errs[i] = err.Error()
				return
			}
			if res := run(r.dir, agent, nil, "add", "shared.txt", "--hunk", id); res.code != 0 {
				errs[i] = res.String()
				return
			}
			if res := run(r.dir, agent, []string{"GIT_MULTISTAGE_TEST_DELAY_MS=10"}, "commit", "-m", agent); res.code != 0 {
				errs[i] = res.String()
			}
		})
		failOnWorkerErrors(t, "agent-%d", errs)
		final := r.git("show", "main:shared.txt")
		bySubject := r.commitBySubject()
		for i := range agents {
			agent := fmt.Sprintf("agent-%d", i)
			if !strings.Contains(final, "edited by "+agent+"\n") {
				t.Fatalf("final file lacks %s's edit:\n%s", agent, final)
			}
			diff := r.git("show", "-U0", "--format=", bySubject[agent])
			if got := strings.Count(diff, "\n+edited by "); got != 1 || !strings.Contains(diff, "+edited by "+agent+"\n") {
				t.Fatalf("commit of %s:\n%s", agent, diff)
			}
		}
		if final != r.read("shared.txt") {
			t.Fatalf("committed file differs from the working tree")
		}
		r.checkHealthy()
		r.checkClean()
	}
}

func TestCommitOrderDiffersFromStagingOrder(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.write("b.txt", "b\n")
	r.mustMS("agent-b", "add", "b.txt")
	r.mustMS("agent-b", "commit", "-m", "b")
	r.mustMS("agent-a", "commit", "-m", "a")
	bySubject := r.commitBySubject()
	if !eq(r.changedPaths(bySubject["a"]), []string{"a.txt"}) || !eq(r.changedPaths(bySubject["b"]), []string{"b.txt"}) {
		t.Fatal("a commit carries another agent's file")
	}
	if r.git("rev-parse", "main~1") != r.git("rev-parse", bySubject["b"]) {
		t.Fatal("a was not committed on top of b")
	}
	r.checkClean()
}

// TestStagedChangesReplayOntoNewerCommits stages from an old base while other
// agents commit nearby lines of the same file.
func TestStagedChangesReplayOntoNewerCommits(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", lines(12, func(i int) string { return "l" + strconv.Itoa(i) }))
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.write("f.txt", strings.Replace(r.read("f.txt"), "l3\n", "A3\n", 1))
	r.mustMS("agent-a", "add", "f.txt")
	// Two lines below, agent-b commits twice while agent-a waits.
	r.write("f.txt", strings.Replace(r.read("f.txt"), "l5\n", "B5\n", 1))
	r.mustMS("agent-b", "add", "f.txt", "--hunk", r.hunkID("agent-b", "f.txt", "B5"))
	r.mustMS("agent-b", "commit", "-m", "b5")
	r.write("f.txt", strings.Replace(r.read("f.txt"), "l9\n", "B9\n", 1))
	r.mustMS("agent-b", "add", "f.txt", "--hunk", r.hunkID("agent-b", "f.txt", "B9"))
	r.mustMS("agent-b", "commit", "-m", "b9")
	r.mustMS("agent-a", "commit", "-m", "a3")
	got := r.git("show", "main:f.txt")
	for _, want := range []string{"A3\n", "B5\n", "B9\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("final file lacks %q:\n%s", want, got)
		}
	}
	diff := r.git("show", "-U0", "--format=", "main")
	if strings.Contains(diff, "B5") || strings.Contains(diff, "B9") || !strings.Contains(diff, "+A3") {
		t.Fatalf("agent-a's commit:\n%s", diff)
	}
	r.checkClean()
}

func TestRepeatedCompareAndSwapFailures(t *testing.T) {
	r := newRepo(t)
	r.write("mine.txt", "mine\n")
	r.mustMS("agent-a", "add", "mine.txt")
	res := run(r.dir, "agent-a", []string{"GIT_MULTISTAGE_TEST_RACE=6"}, "commit", "--json", "-m", "mine")
	if res.code != 0 {
		t.Fatalf("commit:\n%s", res)
	}
	var out struct {
		Commit struct {
			Attempts int
			Commit   string
		}
	}
	decode(t, res, &out)
	if out.Commit.Attempts != 7 {
		t.Fatalf("attempts = %d, want 7", out.Commit.Attempts)
	}
	if got := len(r.commits()); got != 1+6+1 {
		t.Fatalf("%d commits, want 8", got)
	}
	if !eq(r.changedPaths("main"), []string{"mine.txt"}) {
		t.Fatalf("commit changed %v", r.changedPaths("main"))
	}
	if got := len(strings.Fields(r.git("ls-tree", "--name-only", "main"))); got != 1+6+1 {
		t.Fatalf("tree has %d files", got)
	}
	r.checkHealthy()
	// The race commits moved the branch with update-ref and no reflog
	// message, leaving the default index behind; the commit's sync catches
	// that lag up because the reflog proves that nothing touched the index.
	r.checkIndexSynced()
}

func TestConflictingEditsFailAndKeepStagedChanges(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "one\ntwo\nthree\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.write("f.txt", "one\nA\nthree\n")
	r.mustMS("agent-a", "add", "f.txt")
	r.write("f.txt", "one\nB\nthree\n")
	r.mustMS("agent-b", "add", "f.txt", "--allow-shared")
	r.mustMS("agent-b", "commit", "-m", "b")
	head := r.git("rev-parse", "main")
	res := r.expectCode(3, "agent-a", "commit", "-m", "a")
	if !strings.Contains(res.err, "f.txt") {
		t.Fatalf("conflict does not name the path:\n%s", res)
	}
	if r.git("rev-parse", "main") != head {
		t.Fatal("a failed commit moved the branch")
	}
	if diff := r.mustMS("agent-a", "diff", "--cached").out; !strings.Contains(diff, "+A") {
		t.Fatalf("staged change lost:\n%s", diff)
	}
	var st statusJSON
	decode(t, r.mustMS("agent-a", "status", "--json"), &st)
	if !st.Status.Freshness.Stale {
		t.Fatal("status does not report the stage as stale")
	}
}

func TestIdenticalEditsCommitOnce(t *testing.T) {
	r := newRepo(t)
	r.write("README", "same edit\n")
	r.mustMS("agent-a", "add", "README")
	r.mustMS("agent-b", "add", "README", "--allow-shared")
	r.mustMS("agent-b", "commit", "-m", "b")
	res := r.mustMS("agent-a", "commit", "--json", "-m", "a")
	if !strings.Contains(res.out, `"already-applied"`) {
		t.Fatalf("commit:\n%s", res)
	}
	if got := len(r.commits()); got != 2 {
		t.Fatalf("%d commits, want 2", got)
	}
	if got := r.stagedPaths("agent-a"); len(got) != 0 {
		t.Fatalf("stage keeps %v", got)
	}
	r.checkClean()
}

// externalCommit commits a change to main without touching the working tree,
// as a push from elsewhere followed by a fetch-and-update would.
func (r *repo) externalCommit(msg string, update func(index string)) {
	r.t.Helper()
	idx := r.path(".git/external-index")
	defer func() { _ = os.Remove(idx) }()
	env := append(os.Environ(), "GIT_INDEX_FILE="+idx)
	gitIdx := func(args ...string) string {
		cmd := gitCmd(r.dir, env, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			r.t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	gitIdx("read-tree", "main")
	update(idx)
	tree := gitIdx("write-tree")
	parent := strings.TrimSpace(r.git("rev-parse", "main"))
	c := gitIdx("commit-tree", tree, "-p", parent, "-m", msg)
	r.git("update-ref", "refs/heads/main", c, parent)
}

func TestRenamedUpstreamFollowsRename(t *testing.T) {
	r := newRepo(t)
	r.write("x.txt", lines(10, func(i int) string { return "x" + strconv.Itoa(i) }))
	r.git("add", "x.txt")
	r.git("commit", "-q", "-m", "x")
	r.write("x.txt", strings.Replace(r.read("x.txt"), "x2\n", "X2\n", 1))
	r.mustMS("agent-a", "add", "x.txt")
	blob := strings.TrimSpace(r.git("rev-parse", "main:x.txt"))
	r.externalCommit("rename", func(idx string) {
		env := append(os.Environ(), "GIT_INDEX_FILE="+idx)
		if err := gitCmd(r.dir, env, "update-index", "--force-remove", "x.txt").Run(); err != nil {
			t.Fatal(err)
		}
		if err := gitCmd(r.dir, env, "update-index", "--add", "--cacheinfo", "100644", blob, "y.txt").Run(); err != nil {
			t.Fatal(err)
		}
	})
	r.mustMS("agent-a", "commit", "-m", "edit x")
	if got := r.git("show", "main:y.txt"); !strings.Contains(got, "X2\n") {
		t.Fatalf("y.txt = %q", got)
	}
	if !eq(r.changedPaths("main"), []string{"y.txt"}) {
		t.Fatalf("commit changed %v", r.changedPaths("main"))
	}
}

func TestDeletedUpstreamConflicts(t *testing.T) {
	r := newRepo(t)
	r.write("x.txt", "x\n")
	r.git("add", "x.txt")
	r.git("commit", "-q", "-m", "x")
	r.write("x.txt", "changed\n")
	r.mustMS("agent-a", "add", "x.txt")
	r.externalCommit("delete", func(idx string) {
		if err := gitCmd(r.dir, append(os.Environ(), "GIT_INDEX_FILE="+idx), "update-index", "--force-remove", "x.txt").Run(); err != nil {
			t.Fatal(err)
		}
	})
	r.expectCode(3, "agent-a", "commit", "-m", "edit x")
	if diff := r.mustMS("agent-a", "diff", "--cached").out; !strings.Contains(diff, "+changed") {
		t.Fatalf("staged change lost:\n%s", diff)
	}
}

// TestNativeGitActivityDuringAgentCommits runs ordinary `git add` and
// `git commit` while agents commit to the same branch.
func TestNativeGitActivityDuringAgentCommits(t *testing.T) {
	for range iterations() {
		r := newRepo(t)
		const agents, rounds, natives = 4, 3, 6
		errs := make([]string, agents+1)
		parallel(agents+1, func(i int) {
			if i == agents {
				errs[i] = r.nativeCommits(natives)
				return
			}
			agent := fmt.Sprintf("agent-%d", i)
			for round := range rounds {
				p := fmt.Sprintf("%s-%d.txt", agent, round)
				if err := writeFile(r.path(p), "agent\n"); err != nil {
					errs[i] = err.Error()
					return
				}
				if res := r.ms(agent, "add", p); res.code != 0 {
					errs[i] = res.String()
					return
				}
				if res := r.ms(agent, "commit", "-m", fmt.Sprintf("%s %d", agent, round)); res.code != 0 {
					errs[i] = res.String()
					return
				}
			}
		})
		failOnWorkerErrors(t, "worker %d", errs)
		if got := len(r.commits()); got != 1+agents*rounds+natives {
			t.Fatalf("%d commits, want %d:\n%s", got, 1+agents*rounds+natives, r.git("log", "--oneline"))
		}
		r.checkHealthy()
		r.checkClean()
	}
}

// nativeCommits commits n files with plain git, and returns an error text,
// or "". A native commit fails while another process holds index.lock, so
// each one is retried a bounded number of times.
func (r *repo) nativeCommits(n int) string {
	for i := range n {
		p := fmt.Sprintf("native-%d.txt", i)
		if err := writeFile(r.path(p), "native\n"); err != nil {
			return err.Error()
		}
		committed := false
		for attempt := 0; attempt < 200 && !committed; attempt++ {
			_, err1 := r.gitTry("add", p)
			_, err2 := r.gitTry("commit", "-q", "-m", "native "+strconv.Itoa(i), "--", p)
			log, err3 := r.gitTry("log", "--format=%s", "main")
			committed = err1 == nil && err2 == nil ||
				err3 == nil && strings.Contains(log, "native "+strconv.Itoa(i)+"\n")
			if !committed {
				time.Sleep(10 * time.Millisecond)
			}
		}
		if !committed {
			return "native commit " + strconv.Itoa(i) + " did not land after 200 attempts"
		}
	}
	return ""
}

// failOnWorkerErrors fails the test at the first worker that reported an
// error; format names the worker by its index.
func failOnWorkerErrors(t *testing.T, format string, errs []string) {
	t.Helper()
	for i, e := range errs {
		if e != "" {
			t.Fatalf(format+": %s", i, e)
		}
	}
}

func TestNativeStagedChangesArePreserved(t *testing.T) {
	r := newRepo(t)
	r.write("p.txt", lines(10, func(i int) string { return "p" + strconv.Itoa(i) }))
	r.git("add", "p.txt")
	r.git("commit", "-q", "-m", "p")
	// A person stages a change natively.
	r.write("p.txt", strings.Replace(r.read("p.txt"), "p1\n", "HUMAN\n", 1))
	r.git("add", "p.txt")
	staged := r.git("ls-files", "-s", "p.txt")
	// An agent commits a different file, then another hunk of p.txt.
	r.write("q.txt", "q\n")
	r.mustMS("agent-a", "add", "q.txt")
	r.mustMS("agent-a", "commit", "-m", "q")
	if got := r.git("ls-files", "-s", "p.txt"); got != staged {
		t.Fatalf("native staged entry changed: %s -> %s", staged, got)
	}
	r.write("p.txt", strings.Replace(r.read("p.txt"), "p8\n", "AGENT\n", 1))
	r.mustMS("agent-a", "add", "p.txt", "--hunk", r.hunkID("agent-a", "p.txt", "AGENT"))
	res := r.mustMS("agent-a", "commit", "-m", "p8")
	if !strings.Contains(res.err, "natively staged") {
		t.Fatalf("no warning about the kept entry:\n%s", res)
	}
	if got := r.git("ls-files", "-s", "p.txt"); got != staged {
		t.Fatalf("native staged entry changed: %s -> %s", staged, got)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "p.txt\n" {
		t.Fatalf("default index differs from HEAD at %q", got)
	}
	if r.git("show", "main:p.txt") == r.read("p.txt") || strings.Contains(r.git("show", "main:p.txt"), "HUMAN") {
		t.Fatal("the native change leaked into the agent's commit")
	}
}

func TestLinkedWorktreesHaveSeparateStages(t *testing.T) {
	r := newRepo(t)
	wtDir := t.TempDir() + "/linked tree"
	r.git("worktree", "add", "-q", "-b", "feature", wtDir)
	wt := &repo{t: t, dir: wtDir}
	r.write("main.txt", "main\n")
	wt.write("feature.txt", "feature\n")
	r.mustMS("agent-a", "add", "main.txt")
	wt.mustMS("agent-a", "add", "feature.txt")
	results := make([]result, 2)
	parallel(2, func(i int) {
		if i == 0 {
			results[i] = r.ms("agent-a", "commit", "-m", "on main")
		} else {
			results[i] = wt.ms("agent-a", "commit", "-m", "on feature")
		}
	})
	for _, res := range results {
		if res.code != 0 {
			t.Fatalf("commit:\n%s", res)
		}
	}
	if !eq(r.changedPaths("main"), []string{"main.txt"}) || !eq(r.changedPaths("feature"), []string{"feature.txt"}) {
		t.Fatal("commits went to the wrong branch")
	}
	r.checkClean()
	wt.checkClean()
}
