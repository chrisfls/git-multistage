package test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrisfls/git-multistage/internal/filelock"
)

// commitAttempts returns the attempts that `commit --json` reports.
func commitAttempts(t *testing.T, res result) int {
	t.Helper()
	var out struct{ Commit struct{ Attempts int } }
	decode(t, res, &out)
	return out.Commit.Attempts
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
			if n := commitAttempts(t, res); n != 1 {
				errs[i] = fmt.Sprintf("attempts = %d, want 1", n)
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

// startGatedCommit starts stage agent's `commit --json` with TURN_GATE set
// to gate, waits until its pre-commit hook starts, and returns a function
// that waits for the commit to end.
func startGatedCommit(t *testing.T, r *repo, agent, gate string) (wait func() result) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), bin, "commit", "--json", "-m", agent)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "GIT_MULTISTAGE_ID="+agent, "TURN_GATE="+gate)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(gate + ".started"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatalf("the hook of %s did not start:\n%s", agent, errb.String())
		}
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

// TestSlowPreCommitHookGivesUpTheTurn holds stage a's pre-commit hook until
// stage b has committed. Stage a's commit then lands on top of stage b's
// without running the hook again.
func TestSlowPreCommitHookGivesUpTheTurn(t *testing.T) {
	r := newRepo(t)
	r.hook("pre-commit", `if [ -n "$TURN_GATE" ]; then
  echo run >> "$TURN_GATE.runs"
  : > "$TURN_GATE.started"
  while [ -e "$TURN_GATE" ]; do sleep 0.05; done
fi
`)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.write("b.txt", "b\n")
	r.mustMS("agent-b", "add", "b.txt")
	gate := filepath.Join(t.TempDir(), "gate")
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(gate) }()
	waitA := startGatedCommit(t, r, "agent-a", gate)
	start := time.Now()
	r.mustMS("agent-b", "commit", "-m", "agent-b")
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("stage b's commit took %v; it waited for the turn that stage a holds during its hook", took)
	}
	if err := os.Remove(gate); err != nil {
		t.Fatal(err)
	}
	res := waitA()
	if res.code != 0 {
		t.Fatalf("stage a's commit:\n%s", res)
	}
	if n := commitAttempts(t, res); n != 2 {
		t.Fatalf("attempts = %d, want 2", n)
	}
	runs, err := os.ReadFile(gate + ".runs")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(runs), "run\n"); got != 1 {
		t.Fatalf("stage a's hook ran %d times, want 1", got)
	}
	if got := strings.Fields(r.git("log", "--format=%s", "-2", "main")); !eq(got, []string{"agent-a", "agent-b"}) {
		t.Fatalf("subjects %v, want [agent-a agent-b]", got)
	}
	r.checkHealthy()
	r.checkClean()
}
