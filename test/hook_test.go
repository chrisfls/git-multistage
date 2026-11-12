package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// hookCall runs `git multistage hook claude` with a Claude Code-style input.
func (r *repo) hookCall(event, tool, session, agentID, file string, env ...string) result {
	in := map[string]any{
		"hook_event_name": event, "tool_name": tool, "session_id": session,
		"tool_use_id": "toolu_" + session + agentID + filepath.Base(file),
		"tool_input":  map[string]any{"file_path": file},
	}
	if agentID != "" {
		in["agent_id"] = agentID
	}
	data, _ := json.Marshal(in)
	return runStdin(r.dir, "", env, string(data), "hook", "claude")
}

// hookedEdit performs an edit the way a hooked Claude Code agent does.
func (r *repo) hookedEdit(session, agentID, path, content string) {
	r.t.Helper()
	file := r.path(path)
	if res := r.hookCall("PreToolUse", "Edit", session, agentID, file); res.code != 0 {
		r.t.Fatalf("PreToolUse:\n%s", res)
	}
	r.write(path, content)
	if res := r.hookCall("PostToolUse", "Edit", session, agentID, file); res.code != 0 {
		r.t.Fatalf("PostToolUse:\n%s", res)
	}
}

// TestHookSeparatesAdjacentEdits has a session and a subagent edit adjacent
// lines of one file; each stages only its own recorded edit.
func TestHookSeparatesAdjacentEdits(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "1\n2\n3\n4\n5\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	file := r.path("f.txt")
	r.hookCall("PreToolUse", "Edit", "sess1", "", file)
	r.write("f.txt", "1\n2\nA\n4\n5\n")
	post := r.hookCall("PostToolUse", "Edit", "sess1", "", file)
	if !strings.Contains(post.out, "claude-sess1") || !strings.Contains(post.out, "additionalContext") {
		t.Fatalf("first PostToolUse does not announce the stage:\n%s", post)
	}
	r.hookedEdit("sess1", "sub9", "f.txt", "1\n2\nA\nB\n5\n")
	var st statusJSON
	decode(t, r.mustMS("", "--stage", "claude-sub9", "status", "--json"), &st)
	if !strings.Contains(r.mustMS("", "--stage", "claude-sub9", "status").out, "recorded edits") {
		t.Fatal("status does not list recorded edits")
	}
	r.mustMS("", "--stage", "claude-sess1", "add", "--edits")
	r.mustMS("", "--stage", "claude-sub9", "add", "--edits")
	results := make([]result, 2)
	parallel(2, func(i int) {
		results[i] = r.ms("", "--stage", []string{"claude-sess1", "claude-sub9"}[i], "commit", "-m", []string{"A", "B"}[i])
	})
	for _, res := range results {
		if res.code != 0 {
			t.Fatalf("commit:\n%s", res)
		}
	}
	if got := r.git("show", "main:f.txt"); got != "1\n2\nA\nB\n5\n" {
		t.Fatalf("f.txt = %q", got)
	}
	bySubject := r.commitBySubject()
	for subject, mine := range map[string]string{"A": "+A", "B": "+B"} {
		diff := r.git("show", "-U0", "--format=", bySubject[subject])
		if strings.Count(diff, "\n+") != 2 || !strings.Contains(diff, mine+"\n") {
			t.Fatalf("commit %s:\n%s", subject, diff)
		}
	}
	r.checkClean()
}

func TestHookRecordsNewAndDeletedFiles(t *testing.T) {
	r := newRepo(t)
	file := r.path("src/new.txt")
	r.hookCall("PreToolUse", "Write", "s", "", file)
	r.write("src/new.txt", "new\n")
	r.hookCall("PostToolUse", "Write", "s", "", file)
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "claude-s", "commit", "-m", "new")
	if r.git("show", "main:src/new.txt") != "new\n" {
		t.Fatal("new file not committed")
	}
	r.checkClean()
}

func TestHookLeaseBlocksASecondWriter(t *testing.T) {
	r := newRepo(t)
	file := r.path("README")
	short := "GIT_MULTISTAGE_LEASE_TIMEOUT_MS=200"
	if res := r.hookCall("PreToolUse", "Edit", "a", "", file); res.code != 0 {
		t.Fatalf("first writer:\n%s", res)
	}
	res := r.hookCall("PreToolUse", "Edit", "b", "", file, short)
	if res.code != 2 || !strings.Contains(res.err, "claude-a") {
		t.Fatalf("second writer was not blocked:\n%s", res)
	}
	r.write("README", "edited by a\n")
	r.hookCall("PostToolUse", "Edit", "a", "", file)
	if res := r.hookCall("PreToolUse", "Edit", "b", "", file, short); res.code != 0 {
		t.Fatalf("second writer after release:\n%s", res)
	}
}

func TestHookWaitsForTheLease(t *testing.T) {
	r := newRepo(t)
	file := r.path("README")
	r.hookCall("PreToolUse", "Edit", "a", "", file)
	var wg sync.WaitGroup
	var b result
	wg.Add(1)
	go func() {
		defer wg.Done()
		b = r.hookCall("PreToolUse", "Edit", "b", "", file)
	}()
	r.write("README", "a\n")
	r.hookCall("PostToolUse", "Edit", "a", "", file)
	wg.Wait()
	if b.code != 0 {
		t.Fatalf("the waiting writer failed:\n%s", b)
	}
}

func TestHookDetectsALostEdit(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "1\n2\n3\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.hookedEdit("a", "", "f.txt", "1\nA\n3\n")
	r.write("f.txt", "1\n2\n3\nother\n") // an unhooked writer erases the edit
	res := r.expectCode(3, "", "--stage", "claude-a", "add", "--edits")
	if !strings.Contains(res.err, "no longer holds the edit") {
		t.Fatalf("error:\n%s", res)
	}
	r.mustMS("", "--stage", "claude-a", "drop", "--edits")
	r.expectCode(1, "", "--stage", "claude-a", "add", "--edits")
}

func TestHookIgnoresOtherToolsAndPaths(t *testing.T) {
	r := newRepo(t)
	if res := r.hookCall("PreToolUse", "Bash", "a", "", r.path("README")); res.code != 0 || res.out != "" {
		t.Fatalf("Bash:\n%s", res)
	}
	outside := filepath.Join(t.TempDir(), "x.txt")
	for _, ev := range []string{"PreToolUse", "PostToolUse"} {
		if res := r.hookCall(ev, "Write", "a", "", outside); res.code != 0 || res.err != "" {
			t.Fatalf("outside a repository:\n%s", res)
		}
	}
}

// codexCall runs `git multistage hook codex` with a Codex-style input.
func (r *repo) codexCall(event, session, agentID, toolUseID, patch string) result {
	in := map[string]any{
		"hook_event_name": event, "tool_name": "apply_patch", "session_id": session,
		"tool_use_id": toolUseID, "cwd": r.dir,
		"tool_input": map[string]any{"command": patch},
	}
	if agentID != "" {
		in["agent_id"] = agentID
	}
	data, _ := json.Marshal(in)
	return runStdin(r.dir, "", nil, string(data), "hook", "codex")
}

// codexEdit runs the hooks around fn, the way Codex runs apply_patch.
func (r *repo) codexEdit(session, agentID, id, patch string, fn func()) {
	r.t.Helper()
	if res := r.codexCall("PreToolUse", session, agentID, id, patch); res.code != 0 {
		r.t.Fatalf("PreToolUse:\n%s", res)
	}
	fn()
	if res := r.codexCall("PostToolUse", session, agentID, id, patch); res.code != 0 {
		r.t.Fatalf("PostToolUse:\n%s", res)
	}
}

func TestCodexHookSeparatesSessionAndSubagent(t *testing.T) {
	r := newRepo(t)
	r.write("notes.txt", "one\ntwo\nthree\nfour\n")
	r.git("add", "notes.txt")
	r.git("commit", "-q", "-m", "notes")
	r.codexEdit("sess", "", "exec-1", "*** Begin Patch\n*** Update File: notes.txt\n@@\n-two\n+TWO\n*** End Patch",
		func() { r.write("notes.txt", "one\nTWO\nthree\nfour\n") })
	r.codexEdit("sess", "sub-7", "exec-2", "*** Begin Patch\n*** Update File: notes.txt\n@@\n-three\n+THREE\n*** End Patch",
		func() { r.write("notes.txt", "one\nTWO\nTHREE\nfour\n") })
	for stage, subject := range map[string]string{"codex-sess": "two", "codex-sub-7": "three"} {
		r.mustMS("", "--stage", stage, "add", "--edits")
		r.mustMS("", "--stage", stage, "commit", "-m", subject)
	}
	if got := r.git("show", "main:notes.txt"); got != "one\nTWO\nTHREE\nfour\n" {
		t.Fatalf("notes.txt = %q", got)
	}
	bySubject := r.commitBySubject()
	if d := r.git("show", "-U0", "--format=", bySubject["three"]); strings.Contains(d, "+TWO") || !strings.Contains(d, "+THREE") {
		t.Fatalf("the subagent's commit:\n%s", d)
	}
	r.checkClean()
}

func TestCodexHookMultiFilePatch(t *testing.T) {
	r := newRepo(t)
	r.write("old.txt", "old\n")
	r.write("gone.txt", "gone\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "files")
	patch := "*** Begin Patch\n*** Add File: src/new.txt\n+new\n*** Delete File: gone.txt\n" +
		"*** Update File: old.txt\n*** Move to: moved.txt\n@@\n-old\n+moved\n*** End Patch"
	r.codexEdit("s", "", "exec-9", patch, func() {
		r.write("src/new.txt", "new\n")
		os.Remove(r.path("gone.txt"))
		os.Remove(r.path("old.txt"))
		r.write("moved.txt", "moved\n")
	})
	r.mustMS("", "--stage", "codex-s", "add", "--edits")
	r.mustMS("", "--stage", "codex-s", "commit", "-m", "patch")
	out := r.git("show", "--name-status", "--format=", "--no-renames", "main")
	for _, want := range []string{"A\tsrc/new.txt", "D\tgone.txt", "D\told.txt", "A\tmoved.txt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("commit lacks %q:\n%s", want, out)
		}
	}
	r.checkClean()
}

// TestHookKeepsSubagentsApartUnderAnExportedStage sets GIT_MULTISTAGE_ID, as
// a person who exported it before starting Claude Code would. The main
// session records into that stage; a subagent still records into its own.
func TestHookKeepsSubagentsApartUnderAnExportedStage(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "1\n2\n3\n4\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	env := "GIT_MULTISTAGE_ID=mine"
	file := r.path("f.txt")
	r.hookCall("PreToolUse", "Edit", "s", "", file, env)
	r.write("f.txt", "1\nTWO\n3\n4\n")
	r.hookCall("PostToolUse", "Edit", "s", "", file, env)
	r.hookCall("PreToolUse", "Edit", "s", "sub1", file, env)
	r.write("f.txt", "1\nTWO\n3\nFOUR\n")
	r.hookCall("PostToolUse", "Edit", "s", "sub1", file, env)
	r.mustMS("", "--stage", "mine", "add", "--edits")
	r.mustMS("", "--stage", "claude-sub1", "add", "--edits")
	if d := r.mustMS("", "--stage", "mine", "diff", "--cached").out; !strings.Contains(d, "+TWO") || strings.Contains(d, "FOUR") {
		t.Fatalf("stage mine:\n%s", d)
	}
	if d := r.mustMS("", "--stage", "claude-sub1", "diff", "--cached").out; !strings.Contains(d, "+FOUR") || strings.Contains(d, "TWO") {
		t.Fatalf("stage claude-sub1:\n%s", d)
	}
}
