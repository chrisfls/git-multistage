package test

import (
	"encoding/json"
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
