package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chrisfls/git-multistage/internal/filelock"
)

// These tests reproduce defects found in the fourth review.

func TestJSONOutputKeepsStageWarnings(t *testing.T) {
	r := newRepo(t)
	var out struct {
		Warnings []string `json:"warnings"`
	}
	res := r.mustMS("agent-a", "status", "--json")
	decode(t, res, &out)
	if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "created stage agent-a") {
		t.Fatalf("status --json warnings = %q:\n%s", out.Warnings, res)
	}
	res = r.mustMS("agent-b", "status")
	if n := strings.Count(res.err, "created stage agent-b"); n != 1 {
		t.Fatalf("text mode printed the warning %d times:\n%s", n, res)
	}
}

func TestLogFiltersByThePathspec(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-a", "commit", "-m", "add a")
	r.write("b.txt", "b\n")
	r.mustMS("agent-b", "add", "b.txt")
	r.mustMS("agent-b", "commit", "-m", "add b")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"log", "--format=%s"}, "add a\n"},
		{[]string{"log", "--format=%s", "a.txt"}, "add a\n"},
		{[]string{"log", "--format=%s", "--", "a.txt"}, "add a\n"},
		{[]string{"log", "--format=%s", "--", "b.txt"}, ""},
	} {
		if res := r.mustMS("agent-a", c.args...); res.out != c.want {
			t.Errorf("%s = %q, want %q", strings.Join(c.args, " "), res.out, c.want)
		}
	}
}

func TestHookAfterGlobalOptions(t *testing.T) {
	r := newRepo(t)
	file := r.path("README")
	call := func(event string, args ...string) {
		t.Helper()
		in := `{"hook_event_name":"` + event + `","tool_name":"Edit","session_id":"s","tool_use_id":"toolu_1",` +
			`"tool_input":{"file_path":` + strconv.Quote(file) + `}}`
		if res := runStdin(r.dir, "", nil, in, args...); res.code != 0 {
			t.Fatalf("%s %s:\n%s", strings.Join(args, " "), event, res)
		}
	}
	call("PreToolUse", "--stage", "mine", "hook", "claude")
	r.write("README", "edited\n")
	call("PostToolUse", "--stage=mine", "--json", "hook", "claude")
	r.mustMS("mine", "add", "--edits")
	if got := r.stagedPaths("mine"); !eq(got, []string{"README"}) {
		t.Fatalf("staged %q", got)
	}
}

// expireLeases makes every edit lease of the repository expire, as 15
// seconds of waiting on an approval prompt would.
func (r *repo) expireLeases() {
	r.t.Helper()
	paths, err := filepath.Glob(r.path(".git/multistage/leases/*.json"))
	if err != nil || len(paths) == 0 {
		r.t.Fatalf("no leases: %v", err)
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			r.t.Fatal(err)
		}
		var l map[string]any
		if err := json.Unmarshal(data, &l); err != nil {
			r.t.Fatal(err)
		}
		l["expires"] = 0
		data, _ = json.Marshal(l)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
}

func TestTakingAnExpiredLeaseMakesTheEditUncertain(t *testing.T) {
	r := newRepo(t)
	r.write("notes.txt", "one\ntwo\nthree\n")
	r.git("add", "notes.txt")
	r.git("commit", "-q", "-m", "notes")
	patchA := "*** Begin Patch\n*** Update File: notes.txt\n@@\n-two\n+TWO\n*** End Patch"
	patchB := "*** Begin Patch\n*** Update File: notes.txt\n@@\n-three\n+THREE\n*** End Patch"
	if res := r.codexCall("PreToolUse", "a", "", "exec-a", patchA); res.code != 0 {
		t.Fatalf("A PreToolUse:\n%s", res)
	}
	r.expireLeases()
	if res := r.codexCall("PreToolUse", "b", "", "exec-b", patchB); res.code != 0 {
		t.Fatalf("B PreToolUse:\n%s", res)
	}
	r.write("notes.txt", "one\nTWO\nthree\n")
	r.codexCall("PostToolUse", "a", "", "exec-a", patchA)
	r.write("notes.txt", "one\nTWO\nTHREE\n")
	r.codexCall("PostToolUse", "b", "", "exec-b", patchB)
	res := r.expectCode(3, "", "--stage", "codex-b", "add", "--edits")
	if !strings.Contains(res.err, "another writer may have changed") {
		t.Fatalf("add --edits:\n%s", res)
	}
	r.expectCode(3, "", "--stage", "codex-a", "add", "--edits")
}

func TestWriteThroughADanglingSymlinkIsRecorded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	r := newRepo(t)
	if err := os.Symlink("docs/AGENTS.md", r.path("CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	r.git("add", ".")
	r.git("commit", "-q", "-m", "link")
	file := r.path("CLAUDE.md")
	r.hookCall("PreToolUse", "Write", "s", "", file)
	r.write("docs/AGENTS.md", "- one\n")
	r.hookCall("PostToolUse", "Write", "s", "", file)
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	if got := r.stagedPaths("claude-s"); !eq(got, []string{"docs/AGENTS.md"}) {
		t.Fatalf("staged %q", got)
	}
}

func TestHookAcceptsAPathInAnotherCase(t *testing.T) {
	r := newRepo(t)
	if !caseInsensitiveFS(t, r.dir) {
		t.Skip("the file system is case-sensitive")
	}
	r.write("f.txt", "1\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	// The test's temporary directory, such as TestHook.../001, in upper case.
	parent := filepath.Dir(r.dir)
	file := filepath.Join(filepath.Dir(parent), strings.ToUpper(filepath.Base(parent)), filepath.Base(r.dir), "f.txt")
	if _, err := os.Stat(file); err != nil {
		t.Skip("the file system is case-sensitive")
	}
	r.hookCall("PreToolUse", "Edit", "s", "", file)
	r.write("f.txt", "2\n")
	r.hookCall("PostToolUse", "Edit", "s", "", file)
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	if got := r.stagedPaths("claude-s"); !eq(got, []string{"f.txt"}) {
		t.Fatalf("staged %q", got)
	}
}

func TestHookKeepsCRLFOfACommittedFile(t *testing.T) {
	r := newRepo(t)
	r.write("crlf.txt", "one\r\ntwo\r\nthree\r\n")
	r.git("add", "crlf.txt")
	r.git("commit", "-q", "-m", "crlf")
	r.git("config", "core.autocrlf", "true")
	r.hookedEdit("s", "", "crlf.txt", "one\r\nTWO\r\nthree\r\n")
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "claude-s", "commit", "-m", "two")
	if got := r.git("cat-file", "blob", "main:crlf.txt"); got != "one\r\nTWO\r\nthree\r\n" {
		t.Fatalf("crlf.txt = %q", got)
	}
}

// TestAgenticTrustsAToolOnlyAsAnAncestor fakes the process chain, so the
// result does not depend on the processes that run the test.
func TestAgenticTrustsAToolOnlyAsAnAncestor(t *testing.T) {
	r := newRepo(t)
	for _, c := range []struct {
		ancestors string
		env       []string
		want      string // "" for exit code 2
	}{
		{"zsh", []string{"CLAUDE_CODE_SESSION_ID=stale", "PASEO_AGENT_ID=agent1"}, "paseo-agent1"},
		{"zsh", []string{"CLAUDE_CODE_SESSION_ID=stale", "PASEO_AGENT_ID=agent2"}, "paseo-agent2"},
		{"codex", []string{"CLAUDE_CODE_SESSION_ID=stale", "PASEO_AGENT_ID=p"}, "paseo-p"},
		{"zsh,claude", []string{"CLAUDE_CODE_SESSION_ID=c1", "PASEO_AGENT_ID=p"}, "claude-c1"},
		{"", []string{"CODEX_THREAD_ID=x1", "ORCA_AGENT_SESSION_ID=o1"}, "orca-o1"},
		{"codex", []string{"CODEX_THREAD_ID=x1", "ORCA_AGENT_SESSION_ID=o1"}, "codex-x1"},
		{"", []string{"CLAUDE_CODE_SESSION_ID=c1", "CODEX_THREAD_ID=x1", "PASEO_AGENT_ID=p"}, "paseo-p"},
		{"", []string{"CLAUDE_CODE_SESSION_ID=c1", "CODEX_THREAD_ID=x1"}, ""},
		{"", []string{"CLAUDE_CODE_SESSION_ID=c1"}, "claude-c1"},
		{"error", []string{"CLAUDE_CODE_SESSION_ID=c1", "PASEO_AGENT_ID=p"}, "claude-c1"},
		{"error", []string{"CLAUDE_CODE_SESSION_ID=c1", "CODEX_THREAD_ID=x1", "PASEO_AGENT_ID=p"}, ""},
	} {
		env := append([]string{"GIT_MULTISTAGE_TEST_ANCESTORS=" + c.ancestors}, c.env...)
		res := run(r.dir, "", env, "--agentic", "status", "--json")
		var st struct{ Status struct{ Stage string } }
		if c.want == "" {
			if res.code != 2 {
				t.Errorf("ancestors %q, %v: want exit 2\n%s", c.ancestors, c.env, res)
			}
			continue
		}
		decode(t, res, &st)
		if st.Status.Stage != c.want {
			t.Errorf("ancestors %q, %v: stage %q, want %q", c.ancestors, c.env, st.Status.Stage, c.want)
		}
	}
}

// stageGen reads the current index generation of a stage.
func (r *repo) stageGen(stage string) int {
	r.t.Helper()
	var m struct {
		Gen int `json:"gen"`
	}
	data, err := os.ReadFile(r.path(".git/multistage/stages/" + stage + "/meta.json"))
	if err != nil {
		r.t.Fatal(err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		r.t.Fatal(err)
	}
	return m.Gen
}

// TestOrphanedLockOfTheCurrentIndexIsBypassed leaves the lock file of the
// current index generation, as a Git child of a killed git-multistage holds
// it. Each command must move the stage to a new generation instead of
// failing on the lock, keep the staged changes, and leave the orphan's lock
// file alone.
func TestOrphanedLockOfTheCurrentIndexIsBypassed(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "f\n")
	r.write("g.txt", "g\n")
	r.mustMS("a", "add", "f.txt")
	orphan := func() string {
		t.Helper()
		p := r.path(".git/multistage/stages/a/index-" + strconv.Itoa(r.stageGen("a")) + ".lock")
		if err := os.WriteFile(p, []byte("half-written"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	var orphans []string
	for _, args := range [][]string{
		{"status"},
		{"add", "g.txt"},
		{"restore", "--staged", "g.txt"},
		{"add", "g.txt"},
	} {
		orphans = append(orphans, orphan())
		r.mustMS("a", args...)
	}
	if got := r.stagedPaths("a"); !eq(got, []string{"f.txt", "g.txt"}) {
		t.Fatalf("staged %q", got)
	}
	orphans = append(orphans, orphan())
	r.mustMS("a", "commit", "-m", "f and g")
	if got := r.changedPaths("HEAD"); !eq(got, []string{"f.txt", "g.txt"}) {
		t.Fatalf("committed %q", got)
	}
	for _, p := range orphans {
		if got, err := os.ReadFile(p); err != nil || string(got) != "half-written" {
			t.Fatalf("the orphaned lock file %s changed: %q, %v", p, got, err)
		}
	}
	r.write("f.txt", "F\n")
	r.mustMS("a", "add", "f.txt")
	orphan()
	r.mustMS("a", "drop", "--force")
}

// TestCopyIDSurvivesStagingAnotherCopy lists identical hunks, stages one
// copy, then stages another copy by its listed ID. Staging a copy does not
// change the working tree or the stage's base, so the other listed IDs stay
// valid, also when only one copy is left and the copy staged changed the
// line count.
func TestCopyIDSurvivesStagingAnotherCopy(t *testing.T) {
	for _, tc := range []struct{ name, updated, want string }{
		{"three copies", "a\ny\nb\ny\nc\ny\nd\n", "a\ny\nb\ny\nc\nx\nd\n"},
		{"two copies of a longer change", "a\ny\nY\nb\ny\nY\nc\nx\nd\n", "a\ny\nY\nb\ny\nY\nc\nx\nd\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRepo(t)
			r.write("f.txt", "a\nx\nb\nx\nc\nx\nd\n")
			r.git("add", "f.txt")
			r.git("commit", "-q", "-m", "f")
			r.write("f.txt", tc.updated)
			var fj filesJSON
			decode(t, r.mustMS("s", "hunks", "--json", "f.txt"), &fj)
			h := fj.Files[0].Hunks
			r.mustMS("s", "add", "f.txt", "--hunk", h[0].ID)
			r.mustMS("s", "add", "f.txt", "--hunk", h[1].ID)
			r.mustMS("s", "commit", "-m", "two copies")
			if got := r.git("show", "main:f.txt"); got != tc.want {
				t.Fatalf("committed %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCopyIDIsStaleAfterAPeriodicShift lists identical hunks in repeated
// blocks; another stage then commits the deletion of the lines above them,
// so each copy now has the position and the neighbors that the next copy
// had. The listed ID of the first copy must not stage another copy.
func TestCopyIDIsStaleAfterAPeriodicShift(t *testing.T) {
	r := newRepo(t)
	block, yblock := "h\nx\nt\n", "h\ny\nt\n"
	r.write("f.txt", "A\nB\nC\n"+block+block+block)
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.write("f.txt", "A\nB\nC\n"+yblock+yblock+yblock)
	var fa filesJSON
	decode(t, r.mustMS("a", "hunks", "--json", "f.txt"), &fa)
	first := fa.Files[0].Hunks[0].ID
	r.write("f.txt", yblock+yblock+yblock)
	var fb filesJSON
	decode(t, r.mustMS("b", "hunks", "--json", "f.txt"), &fb)
	r.mustMS("b", "add", "f.txt", "--hunk", fb.Files[0].Hunks[0].ID)
	r.mustMS("b", "commit", "-m", "delete ABC")
	r.expectCode(4, "a", "add", "f.txt", "--hunk", first)
}

// TestUndeletableEditRecordsWarn stages a recorded edit while its record
// cannot be deleted. The warning about it prints once, in text and in JSON.
// Later commands warn instead of failing, and never apply the record again;
// once the record can be deleted, the next command deletes it.
func TestUndeletableEditRecordsWarn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only directory on Windows does not stop deletion")
	}
	for _, asJSON := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[asJSON], func(t *testing.T) {
			r := newRepo(t)
			r.write("f.txt", "a\n")
			r.git("add", "f.txt")
			r.git("commit", "-q", "-m", "f")
			r.hookedEdit("s", "", "f.txt", "a\nb\n")
			edits := r.path(".git/multistage/stages/claude-s/edits")
			if err := os.Chmod(edits, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(edits, 0o755) })
			const warning = "cannot delete the staged edit records"
			args := []string{"--stage", "claude-s", "add", "--edits"}
			if asJSON {
				args = append(args, "--json")
			}
			res := r.mustMS("", args...)
			if asJSON {
				var out struct {
					Warnings []string `json:"warnings"`
				}
				decode(t, res, &out)
				if n := strings.Count(strings.Join(out.Warnings, "\n"), warning); n != 1 {
					t.Fatalf("JSON warnings = %q", out.Warnings)
				}
			} else if n := strings.Count(res.err, warning); n != 1 {
				t.Fatalf("printed the warning %d times:\n%s", n, res)
			}
			res = r.mustMS("", "--stage", "claude-s", "status")
			if n := strings.Count(res.err, warning); n != 1 {
				t.Fatalf("status printed the warning %d times:\n%s", n, res)
			}
			r.mustMS("", "--stage", "claude-s", "commit", "-m", "b")
			r.write("f.txt", "a\nb\nc\n")
			r.expectCode(1, "", "--stage", "claude-s", "add", "--edits")
			if got := r.stagedPaths("claude-s"); len(got) != 0 {
				t.Fatalf("staged %q again", got)
			}
			if err := os.Chmod(edits, 0o755); err != nil {
				t.Fatal(err)
			}
			res = r.mustMS("", "--stage", "claude-s", "status")
			if strings.Contains(res.err, warning) {
				t.Fatalf("warned after the records became deletable:\n%s", res)
			}
			if left, _ := filepath.Glob(filepath.Join(edits, "*.json")); len(left) != 0 {
				t.Fatalf("records left: %q", left)
			}
		})
	}
}

// startPaused starts git-multistage in the background, paused at each of
// the named points; the pause files appear in dir.
func (r *repo) startPaused(dir string, points []string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	r.t.Helper()
	var out bytes.Buffer
	cmd := exec.CommandContext(context.Background(), bin, args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "GIT_MULTISTAGE_TEST_PAUSE="+strings.Join(points, ","), "GIT_MULTISTAGE_TEST_PAUSE_DIR="+dir)
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd, &out
}

// waitPause waits until a paused process reaches the pause file p.
func waitPause(t *testing.T, p string) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	t.Fatalf("no process reached %s", filepath.Base(p))
}

// resume lets a paused process go on past the pause file p.
func resume(t *testing.T, p string) {
	t.Helper()
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
}

// TestWaiterRecreatesADroppedStage drops a stage while another process
// waits for its lock. The drop renames the stage directory away, so the
// waiter finds no lock file and creates the stage again. The waiter must
// lock the stage only after the drop ended, and hold the lock file at the
// stage path, which another process then cannot lock.
func TestWaiterRecreatesADroppedStage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot rename a directory that holds an open file, so drop releases the lock before the rename")
	}
	r := newRepo(t)
	r.mustMS("a", "status")
	pauses := t.TempDir()
	at := func(point string) string { return filepath.Join(pauses, point) }
	drop, dropOut := r.startPaused(pauses, []string{"before-drop-rename"}, "--stage", "a", "drop")
	waitPause(t, at("before-drop-rename"))
	waiter, waiterOut := r.startPaused(pauses, []string{"before-stage-lock", "recreate-dropped-stage", "after-stage-lock"}, "--stage", "a", "status")
	waitPause(t, at("before-stage-lock"))
	resume(t, at("before-stage-lock"))
	// The waiter is one file open away from its lock wait.
	time.Sleep(300 * time.Millisecond)
	resume(t, at("before-drop-rename"))
	if err := drop.Wait(); err != nil {
		t.Fatalf("drop: %v\n%s", err, dropOut)
	}
	waitPause(t, at("recreate-dropped-stage"))
	resume(t, at("recreate-dropped-stage"))
	waitPause(t, at("after-stage-lock"))
	if _, err := filelock.Acquire(r.path(".git/multistage/stages/a/lock"), 50*time.Millisecond); !errors.Is(err, filelock.ErrBusy) {
		t.Fatalf("the waiter does not hold the lock at the stage path: %v", err)
	}
	resume(t, at("after-stage-lock"))
	if err := waiter.Wait(); err != nil {
		t.Fatalf("waiter: %v\n%s", err, waiterOut)
	}
	if !strings.Contains(waiterOut.String(), "created stage a") {
		t.Fatalf("the waiter did not create the stage again:\n%s", waiterOut)
	}
	if left, _ := filepath.Glob(r.path(".git/multistage/stages/.dropped-*")); len(left) != 0 {
		t.Fatalf("drop left %q", left)
	}
	r.mustMS("a", "status")
}

// TestStagedCopyIDSurvivesRestoringAnotherCopy stages identical hunks,
// lists them with `hunks --cached`, restores one copy, then restores another
// copy by its listed ID. Restoring a copy does not change the stage's base,
// so the other listed IDs stay valid.
func TestStagedCopyIDSurvivesRestoringAnotherCopy(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "a\nx\nb\nx\nc\nx\nd\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.write("f.txt", "a\ny\nb\ny\nc\ny\nd\n")
	r.mustMS("s", "add", "f.txt")
	var fj filesJSON
	decode(t, r.mustMS("s", "hunks", "--cached", "--json", "f.txt"), &fj)
	h := fj.Files[0].Hunks
	if len(h) != 3 {
		t.Fatalf("want 3 staged hunks:\n%+v", fj)
	}
	r.mustMS("s", "restore", "--staged", "f.txt", "--hunk", h[0].ID)
	r.mustMS("s", "restore", "--staged", "f.txt", "--hunk", h[1].ID)
	r.mustMS("s", "commit", "-m", "third copy")
	if got, want := r.git("show", "main:f.txt"), "a\nx\nb\nx\nc\ny\nd\n"; got != want {
		t.Fatalf("committed %q, want %q", got, want)
	}
}

// TestStagedCopyIDIsStaleAfterANewBase lists staged identical hunks; another
// stage then commits a change to the same file, so the stage moves to a new
// base. The listed copy IDs must be unknown now.
func TestStagedCopyIDIsStaleAfterANewBase(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "a\nx\nb\nx\nc\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.write("f.txt", "a\ny\nb\ny\nc\n")
	r.mustMS("a", "add", "f.txt")
	var fj filesJSON
	decode(t, r.mustMS("a", "hunks", "--cached", "--json", "f.txt"), &fj)
	first := fj.Files[0].Hunks[0].ID
	patch := "diff --git a/f.txt b/f.txt\n--- a/f.txt\n+++ b/f.txt\n@@ -1,2 +1,2 @@\n-a\n+A\n x\n"
	if res := runStdin(r.dir, "b", nil, patch, "apply", "-"); res.code != 0 {
		t.Fatalf("apply:\n%s", res)
	}
	r.mustMS("b", "commit", "-m", "A")
	r.expectCode(4, "a", "restore", "--staged", "f.txt", "--hunk", first)
}

// TestDoctorReportsAnOrphanedStageIndexLock leaves the lock file of a stage's
// index, as a Git child of a killed git-multistage leaves it. doctor must
// warn with the stage and the path, and leave the file in place.
func TestDoctorReportsAnOrphanedStageIndexLock(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "f\n")
	r.mustMS("a", "add", "f.txt")
	name := "index-" + strconv.Itoa(r.stageGen("a")) + ".lock"
	lock := r.path(".git/multistage/stages/a/" + name)
	if err := os.WriteFile(lock, []byte("half-written"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := r.mustMS("", "doctor")
	// The temporary directory can be reached through a symbolic link, such
	// as /var on macOS, so match the path's end.
	if !regexp.MustCompile(`warning: stage a: \S*` + regexp.QuoteMeta(filepath.Join("stages", "a", name)) + " exists").MatchString(doc.out) {
		t.Fatalf("doctor does not report the lock %s:\n%s", lock, doc)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("doctor removed the lock: %v", err)
	}
}

// TestAgenticBoundsAHangingProcessTableRead puts a ps that hangs first on
// PATH. --agentic must stop waiting for it and decide as when the process
// table cannot be read: the single tool variable wins over the orchestrator.
func TestAgenticBoundsAHangingProcessTableRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reads the process table without ps")
	}
	r := newRepo(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"CLAUDE_CODE_SESSION_ID=abc", "PASEO_AGENT_ID=p",
	}
	start := time.Now()
	res := run(r.dir, "", env, "--agentic", "status", "--json")
	if res.code != 0 || !strings.Contains(res.out, `"stage": "claude-abc"`) {
		t.Fatalf("--agentic with a hanging ps:\n%s", res)
	}
	if d := time.Since(start); d > 15*time.Second {
		t.Fatalf("--agentic waited %s for ps", d)
	}
}

// TestDeletionNextToANeighbourEdit deletes lines with a hooked edit; another
// agent then changes the line below or above them. The deletion is still on
// disk, so add --edits must stage it.
func TestDeletionNextToANeighbourEdit(t *testing.T) {
	for _, tc := range []struct{ name, after, other string }{
		{"line below changed", "a\nb\ne\n", "a\nb\nE\n"},
		{"line above changed", "a\nb\ne\n", "a\nB\ne\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRepo(t)
			r.write("f.txt", "a\nb\nc\nd\ne\n")
			r.git("add", "f.txt")
			r.git("commit", "-q", "-m", "f")
			r.hookedEdit("s", "", "f.txt", tc.after)
			r.hookedEdit("t", "", "f.txt", tc.other)
			if res := r.ms("", "--stage", "claude-s", "add", "--edits"); res.code != 0 {
				t.Fatalf("add --edits refused the surviving deletion:\n%s", res)
			}
			r.mustMS("", "--stage", "claude-s", "commit", "-m", "del")
			if c := r.git("show", "HEAD:f.txt"); c != "a\nb\ne\n" {
				t.Fatalf("committed %q", c)
			}
		})
	}
}
