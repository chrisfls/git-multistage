package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrisfls/git-multistage/internal/filelock"
)

// attempts returns the attempts that `commit --json` reports.
func attempts(res result) (int, error) {
	var out struct{ Commit struct{ Attempts int } }
	err := json.Unmarshal([]byte(res.out), &out)
	return out.Commit.Attempts, err
}

// TestManyStagesCommitInTurns has eight stages commit at the same time.
// Each commit waits for its turn and lands without a replay.
func TestManyStagesCommitInTurns(t *testing.T) {
	for range iterations() {
		r := newRepo(t)
		const agents = 8
		for i := range agents {
			p := fmt.Sprintf("f%d.txt", i)
			r.write(p, p+"\n")
			r.mustMS(fmt.Sprintf("agent-%d", i), "add", p)
		}
		errs := make([]string, agents)
		parallel(agents, func(i int) {
			res := run(r.dir, fmt.Sprintf("agent-%d", i), nil, "commit", "--json", "-m", fmt.Sprintf("c%d", i))
			if res.code != 0 {
				errs[i] = res.String()
				return
			}
			if n, err := attempts(res); err != nil || n != 1 {
				errs[i] = fmt.Sprintf("attempts = %d (%v), want 1", n, err)
			}
		})
		failOnWorkerErrors(t, "agent-%d", errs)
		if got := len(r.commits()); got != 1+agents {
			t.Fatalf("%d commits, want %d", got, 1+agents)
		}
		r.checkHealthy()
		r.checkClean()
	}
}

// TestCommitGoesOnWithoutAHeldTurn holds the commit turn from the test past
// the commit's wait, shortened to 300 ms. The commit then lands without it.
func TestCommitGoesOnWithoutAHeldTurn(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	turn, err := filelock.Acquire(r.path(".git/multistage/commit.lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer turn.Release()
	start := time.Now()
	res := run(r.dir, "agent-a", []string{"GIT_MULTISTAGE_TEST_LOCK_TIMEOUT_MS=300"}, "commit", "-m", "a")
	if res.code != 0 {
		t.Fatalf("commit:\n%s", res)
	}
	if took := time.Since(start); took < 300*time.Millisecond {
		t.Fatalf("commit took %v; it did not wait for the turn", took)
	}
	if !eq(r.changedPaths("main"), []string{"a.txt"}) {
		t.Fatalf("commit changed %v", r.changedPaths("main"))
	}
	r.checkHealthy()
}

// TestKilledCommitGivesUpItsTurn kills a commit while it holds its turn. The
// next commit takes the turn at once, not after the 10 s wait.
func TestKilledCommitGivesUpItsTurn(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.write("b.txt", "b\n")
	r.mustMS("agent-b", "add", "b.txt")
	if res := run(r.dir, "agent-a", []string{"GIT_MULTISTAGE_FAULT=before-update-ref"}, "commit", "-m", "a"); res.code != 86 {
		t.Fatalf("faulted commit:\n%s", res)
	}
	start := time.Now()
	r.mustMS("agent-b", "commit", "-m", "b")
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("commit took %v; it waited for the turn of the killed commit", took)
	}
	if !eq(r.changedPaths("main"), []string{"b.txt"}) {
		t.Fatalf("commit changed %v", r.changedPaths("main"))
	}
	r.mustMS("agent-a", "commit", "-m", "a")
	r.checkHealthy()
	r.checkClean()
}

// gatedHook is a pre-commit hook that, when TURN_GATE is set, records its
// run, creates "$TURN_GATE.started", and waits while the file TURN_GATE exists.
const gatedHook = `if [ -n "$TURN_GATE" ]; then
  echo run >> "$TURN_GATE.runs"
  : > "$TURN_GATE.started"
  while [ -e "$TURN_GATE" ]; do sleep 0.05; done
fi
`

// gatedRepo returns a repository with the gated pre-commit hook, stages
// agent-a and agent-b with one staged file each, and a gate file.
func gatedRepo(t *testing.T) (r *repo, gate string) {
	t.Helper()
	r = newRepo(t)
	r.hook("pre-commit", gatedHook)
	for _, agent := range []string{"agent-a", "agent-b"} {
		r.write(agent+".txt", agent+"\n")
		r.mustMS(agent, "add", agent+".txt")
	}
	gate = filepath.Join(t.TempDir(), "gate")
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(gate) })
	return r, gate
}

// startCommit starts stage agent's `commit --json -m <agent>` with env and
// args, and returns a function that waits for it to end.
func startCommit(t *testing.T, r *repo, agent string, env []string, args ...string) (wait func() result) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), bin, append([]string{"commit", "--json", "-m", agent}, args...)...)
	cmd.Dir = r.dir
	cmd.Env = append(append(os.Environ(), "GIT_MULTISTAGE_ID="+agent), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return func() result {
		code := 0
		if err := cmd.Wait(); err != nil {
			code = -1
			errb.WriteString(err.Error())
		}
		return result{out.String(), errb.String(), code}
	}
}

// waitForFile waits up to 30 s until path exists.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not appear", path)
		}
	}
}

// landedIn checks that a background commit exited with code 0 after want
// attempts.
func landedIn(t *testing.T, name string, res result, want int) {
	t.Helper()
	if res.code != 0 {
		t.Fatalf("%s:\n%s", name, res)
	}
	if n, err := attempts(res); err != nil || n != want {
		t.Fatalf("%s: attempts = %d (%v), want %d", name, n, err, want)
	}
}

// hookRuns returns how often the gated hook ran with gate.
func hookRuns(t *testing.T, gate string) int {
	t.Helper()
	runs, err := os.ReadFile(gate + ".runs")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(runs), "run\n")
}

// TestSlowPreCommitHookGivesUpTheTurn holds stage a's pre-commit hook until
// stage b has committed. Stage a's commit then lands on top of stage b's
// without running the hook again.
func TestSlowPreCommitHookGivesUpTheTurn(t *testing.T) {
	r, gate := gatedRepo(t)
	waitA := startCommit(t, r, "agent-a", []string{"TURN_GATE=" + gate})
	waitForFile(t, gate+".started")
	start := time.Now()
	r.mustMS("agent-b", "commit", "-m", "agent-b")
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("stage b's commit took %v; it waited for the turn that stage a holds during its hook", took)
	}
	if err := os.Remove(gate); err != nil {
		t.Fatal(err)
	}
	landedIn(t, "stage a's commit", waitA(), 2)
	if n := hookRuns(t, gate); n != 1 {
		t.Fatalf("stage a's hook ran %d times, want 1", n)
	}
	if got := strings.Fields(r.git("log", "--format=%s", "-2", "main")); !eq(got, []string{"agent-a", "agent-b"}) {
		t.Fatalf("subjects %v, want [agent-a agent-b]", got)
	}
	r.checkHealthy()
	r.checkClean()
}

// TestHookedCommitTakesTheTurnAgain ends stage a's pre-commit hook while
// stage b holds its turn. Stage a waits for the turn before it moves the
// branch, so stage b lands without a replay.
func TestHookedCommitTakesTheTurnAgain(t *testing.T) {
	r, gate := gatedRepo(t)
	waitA := startCommit(t, r, "agent-a", []string{"TURN_GATE=" + gate})
	waitForFile(t, gate+".started")
	pauseDir := t.TempDir()
	pause := filepath.Join(pauseDir, "after-turn-tip")
	waitB := startCommit(t, r, "agent-b",
		[]string{"GIT_MULTISTAGE_TEST_PAUSE=after-turn-tip", "GIT_MULTISTAGE_TEST_PAUSE_DIR=" + pauseDir}, "--no-verify")
	waitForFile(t, pause)
	if err := os.Remove(gate); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // stage a's hook ends while stage b holds the turn
	// A replay of stage b would pause again; let it go on.
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			_ = os.Remove(pause)
			select {
			case <-done:
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()
	landedIn(t, "stage b's commit", waitB(), 1)
	landedIn(t, "stage a's commit", waitA(), 2)
	if n := hookRuns(t, gate); n != 1 {
		t.Fatalf("stage a's hook ran %d times, want 1", n)
	}
	r.checkHealthy()
	r.checkClean()
}
