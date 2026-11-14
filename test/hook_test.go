package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	t.Parallel()
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
	t.Parallel()
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

// TestHookEditsOfOneFileDoNotWait has two agents edit one file at the same
// time; neither hook waits, and each record holds only its own change.
func TestHookEditsOfOneFileDoNotWait(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "1\n2\n3\n4\n5\n6\n7\n8\n9\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	file := r.path("f.txt")
	orig := r.read("f.txt")
	start := time.Now()
	r.claudeEditCall("PreToolUse", "a", file, "", "2\n", "TWO\n")
	r.claudeEditCall("PreToolUse", "b", file, "", "8\n", "EIGHT\n")
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("the PreToolUse hooks took %v", d)
	}
	r.write("f.txt", "1\nTWO\n3\n4\n5\n6\n7\n8\n9\n")
	r.write("f.txt", "1\nTWO\n3\n4\n5\n6\n7\nEIGHT\n9\n")
	r.claudeEditCall("PostToolUse", "b", file, "1\nTWO\n3\n4\n5\n6\n7\n8\n9\n", "8\n", "EIGHT\n")
	r.claudeEditCall("PostToolUse", "a", file, orig, "2\n", "TWO\n")
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "a")
	if got := r.git("show", "main:f.txt"); got != "1\nTWO\n3\n4\n5\n6\n7\n8\n9\n" {
		t.Fatalf("a committed %q", got)
	}
	r.mustMS("", "--stage", "claude-b", "add", "--edits")
	r.mustMS("", "--stage", "claude-b", "commit", "-m", "b")
	if got := r.git("show", "main:f.txt"); got != "1\nTWO\n3\n4\n5\n6\n7\nEIGHT\n9\n" {
		t.Fatalf("b committed %q", got)
	}
}

// TestAddEditsStagesAnErasedEdit erases a recorded edit on disk; the stage
// still stages it, and the disk keeps the later write.
func TestAddEditsStagesAnErasedEdit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "1\n2\n3\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.hookedEdit("a", "", "f.txt", "1\nA\n3\n")
	r.hookedEdit("b", "", "f.txt", "1\nB\n3\n")
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "a")
	if got := r.git("show", "main:f.txt"); got != "1\nA\n3\n" {
		t.Fatalf("committed %q", got)
	}
	if got := r.read("f.txt"); got != "1\nB\n3\n" {
		t.Fatalf("disk holds %q", got)
	}
}

// TestAddEditsStagesAnEditOfADeletedFile deletes the file after the edit; the
// stage still stages the edit.
func TestAddEditsStagesAnEditOfADeletedFile(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("x.txt", "1\n")
	r.git("add", "x.txt")
	r.git("commit", "-q", "-m", "x")
	r.hookedEdit("a", "", "x.txt", "1\n2\n")
	if err := os.Remove(r.path("x.txt")); err != nil {
		t.Fatal(err)
	}
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "a")
	if got := r.git("show", "main:x.txt"); got != "1\n2\n" {
		t.Fatalf("committed %q", got)
	}
}

// TestAddEditsStagesAnOldUncertainRecord stages a record that an earlier
// version marked as uncertain.
func TestAddEditsStagesAnOldUncertainRecord(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "1\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.hookedEdit("a", "", "f.txt", "1\n2\n")
	recs, err := filepath.Glob(r.path(".git/multistage/stages/claude-a/edits/*.json"))
	if err != nil || len(recs) != 1 {
		t.Fatalf("records %q: %v", recs, err)
	}
	data, err := os.ReadFile(recs[0])
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "{", `{"uncertain":true,`, 1))
	if err := os.WriteFile(recs[0], data, 0o644); err != nil {
		t.Fatal(err)
	}
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	if got := r.stagedPaths("claude-a"); !eq(got, []string{"f.txt"}) {
		t.Fatalf("staged %q", got)
	}
}

// TestAddEditsOfAnotherStagesUncommittedLines edits a line that another stage
// added and has not committed: the edit does not fit until that stage commits.
func TestAddEditsOfAnotherStagesUncommittedLines(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "1\n2\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.hookedEdit("b", "", "f.txt", "1\nfunc {\n  x\n}\n2\n")
	r.hookedEdit("a", "", "f.txt", "1\nfunc {\n  y\n}\n2\n")
	res := r.expectCode(3, "", "--stage", "claude-a", "add", "--edits", "--json")
	if !strings.Contains(res.out, `"edit-conflict"`) {
		t.Fatalf("want edit-conflict:\n%s", res)
	}
	if !strings.Contains(r.mustMS("", "--stage", "claude-a", "status").out, "recorded edits") {
		t.Fatal("the records were not kept")
	}
	r.mustMS("", "--stage", "claude-b", "add", "--edits")
	r.mustMS("", "--stage", "claude-b", "commit", "-m", "b")
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "a")
	if got := r.git("show", "main:f.txt"); got != "1\nfunc {\n  y\n}\n2\n" {
		t.Fatalf("committed %q", got)
	}
}

// claudeEditCall runs the hook for a Claude Code Edit of file that replaces
// old with repl in orig, with the tool response that Claude Code sends.
func (r *repo) claudeEditCall(event, session, file, orig, old, repl string) result {
	in := map[string]any{
		"hook_event_name": event, "tool_name": "Edit", "session_id": session,
		"tool_use_id": "toolu_" + session,
		"tool_input":  map[string]any{"file_path": file, "old_string": old, "new_string": repl},
	}
	if event == "PostToolUse" {
		in["tool_response"] = map[string]any{"filePath": file, "oldString": old, "newString": repl, "originalFile": orig}
	}
	data, _ := json.Marshal(in)
	return runStdin(r.dir, "", nil, string(data), "hook", "claude")
}

// TestHookRecordsOnlyTheToolsChange changes another line of the file while a
// Claude Code Edit runs; the record holds only the Edit's change.
func TestHookRecordsOnlyTheToolsChange(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "1\n2\n3\n4\n5\n6\n7\n8\n9\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	file := r.path("f.txt")
	r.claudeEditCall("PreToolUse", "a", file, "", "2\n", "TWO\n")
	r.write("f.txt", "1\n2\n3\n4\n5\n6\n7\n8\nNINE\n") // another writer
	orig := r.read("f.txt")
	r.write("f.txt", "1\nTWO\n3\n4\n5\n6\n7\n8\nNINE\n") // the Edit
	if res := r.claudeEditCall("PostToolUse", "a", file, orig, "2\n", "TWO\n"); res.code != 0 {
		t.Fatalf("PostToolUse:\n%s", res)
	}
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "a")
	if got := r.git("show", "main:f.txt"); got != "1\nTWO\n3\n4\n5\n6\n7\n8\n9\n" {
		t.Fatalf("committed %q", got)
	}
}

// TestCodexHookRecordsOnlyThePatch changes another line while a Codex patch
// runs; the record holds only the patch's change. A patch that does not
// match the file falls back to the file after the call.
func TestCodexHookRecordsOnlyThePatch(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, patch, want string }{
		{"patch applies", "*** Begin Patch\n*** Update File: f.txt\n@@\n 1\n-2\n+TWO\n 3\n*** End Patch", "1\nTWO\n3\n4\n5\n6\n7\n8\n9\n"},
		{"patch does not match", "*** Begin Patch\n*** Update File: f.txt\n@@\n-zz\n+TWO\n*** End Patch", "1\nTWO\n3\n4\n5\n6\n7\n8\nNINE\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRepo(t)
			r.write("f.txt", "1\n2\n3\n4\n5\n6\n7\n8\n9\n")
			r.git("add", "f.txt")
			r.git("commit", "-q", "-m", "f")
			r.codexEdit("s", "", "exec-1", tc.patch, func() {
				r.write("f.txt", "1\nTWO\n3\n4\n5\n6\n7\n8\nNINE\n")
			})
			r.mustMS("", "--stage", "codex-s", "add", "--edits")
			r.mustMS("", "--stage", "codex-s", "commit", "-m", "s")
			if got := r.git("show", "main:f.txt"); got != tc.want {
				t.Fatalf("committed %q, want %q", got, tc.want)
			}
		})
	}
}

// TestHookKeepsCRLFOfAnEditFromTheResponse records a Claude Code Edit of a
// committed CRLF file from its tool response.
func TestHookKeepsCRLFOfAnEditFromTheResponse(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("crlf.txt", "one\r\ntwo\r\nthree\r\n")
	r.git("add", "crlf.txt")
	r.git("commit", "-q", "-m", "crlf")
	r.git("config", "core.autocrlf", "true")
	file := r.path("crlf.txt")
	r.claudeEditCall("PreToolUse", "a", file, "", "two", "TWO")
	r.write("crlf.txt", "one\r\nTWO\r\nthree\r\n")
	r.claudeEditCall("PostToolUse", "a", file, "one\r\ntwo\r\nthree\r\n", "two", "TWO")
	r.mustMS("", "--stage", "claude-a", "add", "--edits")
	r.mustMS("", "--stage", "claude-a", "commit", "-m", "two")
	if got := r.git("cat-file", "blob", "main:crlf.txt"); got != "one\r\nTWO\r\nthree\r\n" {
		t.Fatalf("crlf.txt = %q", got)
	}
}

func TestHookIgnoresOtherToolsAndPaths(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	r := newRepo(t)
	r.write("old.txt", "old\n")
	r.write("gone.txt", "gone\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "files")
	patch := "*** Begin Patch\n*** Add File: src/new.txt\n+new\n*** Delete File: gone.txt\n" +
		"*** Update File: old.txt\n*** Move to: moved.txt\n@@\n-old\n+moved\n*** End Patch"
	r.codexEdit("s", "", "exec-9", patch, func() {
		r.write("src/new.txt", "new\n")
		if err := os.Remove(r.path("gone.txt")); err != nil {
			r.t.Fatal(err)
		}
		if err := os.Remove(r.path("old.txt")); err != nil {
			r.t.Fatal(err)
		}
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
	if msg := r.git("log", "-1", "--format=%B"); !strings.Contains(msg, "Multistage-Moved: old.txt -> moved.txt") {
		t.Fatalf("message:\n%s", msg)
	}
	r.checkClean()
}

// TestHookKeepsSubagentsApartUnderAnExportedStage sets GIT_MULTISTAGE_ID, as
// a person who exported it before starting Claude Code would. The main
// session records into that stage; a subagent still records into its own.
func TestHookKeepsSubagentsApartUnderAnExportedStage(t *testing.T) {
	t.Parallel()
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

// TestHookDetectsTheEditor runs the bare `hook` command, as the plugin's
// shared hooks file does, with Claude Code and Codex inputs.
func TestHookDetectsTheEditor(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.write("b.txt", "b\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "files")
	call := func(in map[string]any) {
		data, _ := json.Marshal(in)
		if res := runStdin(r.dir, "", nil, string(data), "hook"); res.code != 0 {
			t.Fatalf("hook:\n%s", res)
		}
	}
	claude := map[string]any{
		"tool_name": "Edit", "session_id": "s", "tool_use_id": "t1",
		"tool_input": map[string]any{"file_path": r.path("a.txt")},
	}
	codex := map[string]any{
		"tool_name": "apply_patch", "session_id": "x", "tool_use_id": "t2", "cwd": r.dir,
		"tool_input": map[string]any{"command": "*** Begin Patch\n*** Update File: b.txt\n@@\n-b\n+B\n*** End Patch"},
	}
	claude["hook_event_name"], codex["hook_event_name"] = "PreToolUse", "PreToolUse"
	call(claude)
	call(codex)
	r.write("a.txt", "A\n")
	r.write("b.txt", "B\n")
	claude["hook_event_name"], codex["hook_event_name"] = "PostToolUse", "PostToolUse"
	call(claude)
	call(codex)
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "codex-x", "add", "--edits")
	if got := r.stagedPaths("claude-s"); !eq(got, []string{"a.txt"}) {
		t.Fatalf("claude-s staged %v", got)
	}
	if got := r.stagedPaths("codex-x"); !eq(got, []string{"b.txt"}) {
		t.Fatalf("codex-x staged %v", got)
	}
}

// TestHookAnnouncesTheSharedCheckout runs the SessionStart hook inside and
// outside a Git worktree.
func TestHookAnnouncesTheSharedCheckout(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	start := func(cwd string) result {
		data, _ := json.Marshal(map[string]any{"hook_event_name": "SessionStart", "session_id": "s", "cwd": cwd})
		return runStdin(r.dir, "", nil, string(data), "hook")
	}
	res := start(r.dir)
	if res.code != 0 || !strings.Contains(res.out, `"hookEventName":"SessionStart"`) || !strings.Contains(res.out, "Other agents may edit files") {
		t.Fatalf("in a worktree:\n%s", res)
	}
	if res := start(t.TempDir()); res.code != 0 || res.out != "" {
		t.Fatalf("outside a worktree:\n%s", res)
	}
}

// TestHookLeavesARepositoryThatDidNotOptIn runs the hooks in a repository
// without multistage.shared: they print nothing and record nothing.
func TestHookLeavesARepositoryThatDidNotOptIn(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.git("config", "--unset", "multistage.shared")
	data, _ := json.Marshal(map[string]any{"hook_event_name": "SessionStart", "session_id": "s", "cwd": r.dir})
	if res := runStdin(r.dir, "", nil, string(data), "hook"); res.code != 0 || res.out != "" {
		t.Fatalf("SessionStart:\n%s", res)
	}
	file := r.path("README")
	if res := r.hookCall("PreToolUse", "Edit", "s", "", file); res.code != 0 || res.out != "" {
		t.Fatalf("PreToolUse:\n%s", res)
	}
	r.write("README", "changed\n")
	if res := r.hookCall("PostToolUse", "Edit", "s", "", file); res.code != 0 || res.out != "" {
		t.Fatalf("PostToolUse:\n%s", res)
	}
	if _, err := os.Stat(filepath.Join(r.dir, ".git", "multistage", "stages")); !os.IsNotExist(err) {
		t.Fatalf("the hook created stages: %v", err)
	}
}

// TestHookCleansThroughAFilter edits files that a clean filter changes,
// with and without the Edit's tool response; each commit holds the cleaned
// content.
func TestHookCleansThroughAFilter(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.git("config", "filter.up.clean", "tr a-z A-Z")
	r.write(".gitattributes", "*.up filter=up\n")
	r.write("e.up", "a\nb\nc\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "filter")
	r.hookedEdit("s", "", "f.up", "x\ny\n")
	file := r.path("e.up")
	r.claudeEditCall("PreToolUse", "s", file, "", "b\n", "q\n")
	r.write("e.up", "a\nb\nz\n") // another writer
	orig := r.read("e.up")
	r.write("e.up", "a\nq\nz\n") // the Edit
	if res := r.claudeEditCall("PostToolUse", "s", file, orig, "b\n", "q\n"); res.code != 0 {
		t.Fatalf("PostToolUse:\n%s", res)
	}
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "claude-s", "commit", "-m", "s")
	if got := r.git("show", "main:f.up"); got != "X\nY\n" {
		t.Fatalf("committed f.up %q", got)
	}
	if got := r.git("show", "main:e.up"); got != "A\nQ\nC\n" {
		t.Fatalf("committed e.up %q", got)
	}
}
