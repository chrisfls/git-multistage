package test

import (
	"context"
	"errors"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestCrashAtEachCommitStep kills a commit at each step of its transaction,
// then checks that the next command finishes or discards the commit, with
// exactly one commit in the end.
func TestCrashAtEachCommitStep(t *testing.T) {
	for _, point := range []string{"before-journal", "before-update-ref", "after-update-ref"} {
		t.Run(point, func(t *testing.T) {
			r := newRepo(t)
			r.write("a.txt", "a\n")
			r.mustMS("agent-a", "add", "a.txt")
			res := run(r.dir, "agent-a", []string{"GIT_MULTISTAGE_FAULT=" + point}, "commit", "-m", "a")
			if res.code != 86 {
				t.Fatalf("fault did not fire:\n%s", res)
			}
			landed := point == "after-update-ref"
			if landed {
				// The killed process held Git's index.lock, as a killed `git commit` would.
				doc := r.ms("", "doctor")
				if !strings.Contains(doc.out, "index.lock exists") {
					t.Fatalf("doctor does not report the lock:\n%s", doc)
				}
				if err := os.Remove(r.path(".git/index.lock")); err != nil {
					t.Fatal(err)
				}
			}
			st := r.mustMS("agent-a", "status")
			journaled := point != "before-journal"
			if journaled != strings.Contains(st.err, "interrupted commit") {
				t.Fatalf("status does not report the recovery:\n%s", st)
			}
			if landed {
				if got := r.stagedPaths("agent-a"); len(got) != 0 {
					t.Fatalf("stage keeps %v after a landed commit", got)
				}
			} else {
				if got := r.stagedPaths("agent-a"); !eq(got, []string{"a.txt"}) {
					t.Fatalf("staged %v after a failed commit", got)
				}
				if got := len(r.commits()); got != 1 {
					t.Fatalf("%d commits before the retry", got)
				}
				r.mustMS("agent-a", "commit", "-m", "a")
			}
			if got := len(r.commits()); got != 2 {
				t.Fatalf("%d commits, want 2", got)
			}
			if !eq(r.changedPaths("main"), []string{"a.txt"}) {
				t.Fatalf("commit changed %v", r.changedPaths("main"))
			}
			r.checkHealthy()
			r.checkClean()
		})
	}
}

func TestCrashDuringStaging(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	res := run(r.dir, "agent-a", []string{"GIT_MULTISTAGE_FAULT=after-add"}, "add", "a.txt")
	if res.code != 86 {
		t.Fatalf("fault did not fire:\n%s", res)
	}
	// The add was not adopted: the previous index is current.
	if got := r.stagedPaths("agent-a"); len(got) != 0 {
		t.Fatalf("staged %v", got)
	}
	entries, _ := os.ReadDir(r.path(".git/multistage/stages/agent-a"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "index-") && e.Name() != "index-1" || strings.HasSuffix(e.Name(), ".lock") {
			t.Fatalf("leftover %s", e.Name())
		}
	}
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-a", "commit", "-m", "a")
	r.checkClean()
}

// TestKillDuringCommitAtRandomTimes kills real commit processes at random
// moments. Whatever the moment, the change ends up in exactly one commit.
func TestKillDuringCommitAtRandomTimes(t *testing.T) {
	seed := time.Now().UnixNano()
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // random kill times need no cryptographic randomness
	for i := range 8 * iterations() {
		r := newRepo(t)
		r.write("a.txt", "a\n")
		r.mustMS("agent-a", "add", "a.txt")
		cmd := exec.CommandContext(context.Background(), bin, "commit", "-m", "a")
		cmd.Dir = r.dir
		cmd.Env = append(os.Environ(), "GIT_MULTISTAGE_ID=agent-a", "GIT_MULTISTAGE_TEST_DELAY_MS=40")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Duration(rng.Intn(120)) * time.Millisecond)
		_ = cmd.Process.Kill()             // the process may have exited already
		_ = cmd.Wait()                     // a killed process exits with an error
		time.Sleep(200 * time.Millisecond) // let orphaned Git children finish
		if err := os.Remove(r.path(".git/index.lock")); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		if len(r.stagedPaths("agent-a")) > 0 {
			r.mustMS("agent-a", "commit", "-m", "a")
		}
		touching := strings.Fields(r.git("rev-list", "main", "--", "a.txt"))
		if len(touching) != 1 {
			t.Fatalf("iteration %d: %d commits touch a.txt", i, len(touching))
		}
		r.checkHealthy()
		r.checkClean()
	}
}

// TestLimitationConcurrentWritesToOneFile shows the boundary of the tool:
// when two agents write the same file from the same old version, the second
// write erases the first edit on disk, and no index can recover an edit that
// is no longer on disk. git-multistage refuses the stale selection instead of
// staging the wrong content.
func TestLimitationConcurrentWritesToOneFile(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	orig := r.read("f.txt")
	r.write("f.txt", strings.Replace(orig, "l2\n", "A2\n", 1))
	idA := r.hunkID("agent-a", "f.txt", "A2")
	// agent-b read orig before agent-a wrote, and now writes its version.
	r.write("f.txt", strings.Replace(orig, "l7\n", "B7\n", 1))
	res := r.expectCode(4, "agent-a", "add", "f.txt", "--hunk", idA)
	if !strings.Contains(res.err, "unknown hunk "+idA) {
		t.Fatalf("error:\n%s", res)
	}
	if got := r.stagedPaths("agent-a"); len(got) != 0 {
		t.Fatalf("staged %v", got)
	}
	if strings.Contains(r.read("f.txt"), "A2") {
		t.Fatal("test setup: agent-a's edit should be lost on disk")
	}
}
