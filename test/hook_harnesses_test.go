package test

import (
	"encoding/json"
	"strings"
	"testing"
)

// harnessReplays are the pre-tool and post-tool hook inputs of one edit of
// f.txt in the documented shape of each harness, the hook command and
// variables that the harness's hook file uses, the stage that must record
// the edit, and a part of the first-edit message's output format.
// {{CWD}} is the repository and {{FILE}} the edited file.
var harnessReplays = []struct {
	name, editor string
	env          []string
	pre, post    string
	stage, out   string
}{
	{
		"Antigravity", "", nil,
		`{"hookEventName":"PreToolUse","conversationId":"c1","cwd":"{{CWD}}",` +
			`"toolCall":{"id":"t1","name":"replace_file_content","args":{"TargetFile":"{{FILE}}"}}}`,
		`{"hookEventName":"PostToolUse","conversationId":"c1","cwd":"{{CWD}}",` +
			`"toolCall":{"id":"t1","name":"replace_file_content","args":{"TargetFile":"{{FILE}}"}}}`,
		"antigravity-c1", "",
	},
	{
		"Gemini CLI", "gemini", nil,
		`{"hook_event_name":"BeforeTool","session_id":"g1","cwd":"{{CWD}}","tool_name":"replace","tool_input":{"file_path":"{{FILE}}"}}`,
		`{"hook_event_name":"AfterTool","session_id":"g1","cwd":"{{CWD}}","tool_name":"replace","tool_input":{"file_path":"{{FILE}}"}}`,
		"gemini-g1", `"hookEventName":"AfterTool"`,
	},
	{
		"Mistral Vibe", "vibe", nil,
		`{"hook_event_name":"pre_tool","session_id":"v1","cwd":"{{CWD}}","tool_name":"file_system.edit",` +
			`"tool_call_id":"c","tool_input":{"file_path":"{{FILE}}"}}`,
		`{"hook_event_name":"post_tool","session_id":"v1","cwd":"{{CWD}}","tool_name":"file_system.edit",` +
			`"tool_call_id":"c","tool_input":{"file_path":"{{FILE}}"},"tool_status":"success"}`,
		"vibe-v1", `"hook_specific_output":{"additional_context":`,
	},
	{
		"Cline", "cline", nil,
		`{"clineVersion":"3.36","hookName":"PreToolUse","taskId":"t1","workspaceRoots":["{{CWD}}"],` +
			`"preToolUse":{"toolName":"replace_in_file","parameters":{"path":"{{FILE}}"}}}`,
		`{"clineVersion":"3.36","hookName":"PostToolUse","taskId":"t1","workspaceRoots":["{{CWD}}"],` +
			`"postToolUse":{"toolName":"replace_in_file","parameters":{"path":"{{FILE}}"},"result":"ok","success":true}}`,
		"cline-t1", `"contextModification":`,
	},
	{
		"OpenHands", "", nil,
		`{"event_type":"PreToolUse","session_id":"o1","working_dir":"{{CWD}}","tool_name":"file_editor",` +
			`"tool_input":{"command":"str_replace","path":"{{FILE}}"}}`,
		`{"event_type":"PostToolUse","session_id":"o1","working_dir":"{{CWD}}","tool_name":"file_editor",` +
			`"tool_input":{"command":"str_replace","path":"{{FILE}}"},"tool_response":"ok"}`,
		"openhands-o1", `{"additionalContext":`,
	},
	{
		"Copilot CLI", "",
		[]string{"COPILOT_CLI=1"},
		`{"hook_event_name":"PreToolUse","session_id":"p1","cwd":"{{CWD}}","tool_name":"edit","tool_use_id":"u","tool_input":{"path":"{{FILE}}"}}`,
		`{"hook_event_name":"PostToolUse","session_id":"p1","cwd":"{{CWD}}","tool_name":"edit","tool_use_id":"u","tool_input":{"path":"{{FILE}}"}}`,
		"copilot-p1", `{"additionalContext":`,
	},
	{
		"MiniMax Code", "",
		[]string{"MINIMAX_PLUGIN_ROOT=/x", "CLAUDE_PLUGIN_ROOT=/x"},
		`{"hook_event_name":"PreToolUse","session_id":"m1","cwd":"{{CWD}}","tool_name":"edit","tool_use_id":"u","tool_input":{"file_path":"{{FILE}}"}}`,
		`{"hook_event_name":"PostToolUse","session_id":"m1","cwd":"{{CWD}}","tool_name":"edit","tool_use_id":"u","tool_input":{"file_path":"{{FILE}}"}}`,
		"minimax-m1", `"hookSpecificOutput"`,
	},
	{
		"Grok Build", "",
		[]string{"GROK_HOOK_EVENT=PreToolUse"},
		`{"hookEventName":"PreToolUse","hook_event_name":"PreToolUse","sessionId":"x1","session_id":"x1","cwd":"{{CWD}}",` +
			`"toolName":"search_replace","tool_name":"search_replace","toolUseId":"u","tool_input":{"file_path":"{{FILE}}"}}`,
		`{"hookEventName":"PostToolUse","hook_event_name":"PostToolUse","sessionId":"x1","session_id":"x1","cwd":"{{CWD}}",` +
			`"toolName":"search_replace","tool_name":"search_replace","toolUseId":"u","tool_input":{"file_path":"{{FILE}}"}}`,
		"grok-x1", `"hookSpecificOutput"`,
	},
	{
		"Cursor", "cursor", nil,
		`{"hook_event_name":"preToolUse","conversation_id":"k1","cursor_version":"2.3","tool_name":"Write","tool_use_id":"u",` +
			`"cwd":"{{CWD}}","tool_input":{"file_path":"{{FILE}}"}}`,
		`{"hook_event_name":"postToolUse","conversation_id":"k1","cursor_version":"2.3","tool_name":"Write","tool_use_id":"u",` +
			`"cwd":"{{CWD}}","tool_input":{"file_path":"{{FILE}}"},"tool_output":"ok"}`,
		"cursor-k1", `{"additional_context":`,
	},
	{
		"Kiro CLI", "kiro", nil,
		`{"hook_event_name":"PreToolUse","session_id":"r1","cwd":"{{CWD}}","tool_name":"fs_write",` +
			`"tool_input":{"command":"str_replace","path":"{{FILE}}"}}`,
		`{"hook_event_name":"PostToolUse","session_id":"r1","cwd":"{{CWD}}","tool_name":"fs_write",` +
			`"tool_input":{"command":"str_replace","path":"{{FILE}}"}}`,
		"kiro-r1", "git-multistage records your file edits in stage kiro-r1",
	},
}

// TestHookHarnesses replays one edit in the hook input of each harness
// through `git multistage hook`. The stage that the harness's input names
// must record the edit, `add --edits` must commit exactly it, and the
// first-edit message must use the harness's output format.
func TestHookHarnesses(t *testing.T) {
	t.Parallel()
	for _, tc := range harnessReplays {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newRepo(t)
			r.write("f.txt", "one\ntwo\nthree\n")
			r.git("add", "f.txt")
			r.git("commit", "-q", "-m", "f")
			fill := func(s string) string {
				q := func(v string) string { b, _ := json.Marshal(v); return strings.Trim(string(b), `"`) }
				return strings.NewReplacer("{{CWD}}", q(r.dir), "{{FILE}}", q(r.path("f.txt"))).Replace(s)
			}
			args := []string{"hook"}
			if tc.editor != "" {
				args = append(args, tc.editor)
			}
			if res := runStdin(r.dir, "", tc.env, fill(tc.pre), args...); res.code != 0 {
				t.Fatalf("pre-tool hook:\n%s", res)
			}
			r.write("f.txt", "one\nTWO\nthree\n")
			post := runStdin(r.dir, "", tc.env, fill(tc.post), args...)
			if post.code != 0 || !strings.Contains(post.out, tc.out) {
				t.Fatalf("post-tool hook output lacks %q:\n%s", tc.out, post)
			}
			if tc.out != "" && !strings.Contains(post.out, tc.stage) {
				t.Fatalf("the first-edit message does not name stage %s:\n%s", tc.stage, post)
			}
			r.mustMS("", "--stage", tc.stage, "add", "--edits")
			r.mustMS("", "--stage", tc.stage, "commit", "-m", "edit")
			if got := r.git("show", "HEAD:f.txt"); got != "one\nTWO\nthree\n" {
				t.Fatalf("the commit holds %q", got)
			}
		})
	}
}

// TestHookHarnessNotices checks the session start notice of the harnesses
// that read no output after a tool call: it names the stage of the session.
func TestHookHarnessNotices(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	q, _ := json.Marshal(r.dir)
	agy := runStdin(r.dir, "", nil, `{"hookEventName":"SessionStart","conversationId":"c1","cwd":`+string(q)+`}`, "hook")
	if agy.code != 0 || !strings.HasPrefix(agy.out, `{"injectSteps":[{"systemMessage":{"systemMessage":`) ||
		!strings.Contains(agy.out, "stage antigravity-c1") {
		t.Fatalf("Antigravity's session start notice:\n%s", agy)
	}
	claude := runStdin(r.dir, "", nil, `{"hook_event_name":"SessionStart","session_id":"s1","cwd":`+string(q)+`}`, "hook")
	if claude.code != 0 || !strings.Contains(claude.out, `"hookSpecificOutput"`) || strings.Contains(claude.out, "claude-s1") {
		t.Fatalf("Claude Code's session start notice names a stage, or is missing:\n%s", claude)
	}
	if res := runStdin(r.dir, "", nil, `{}`, "hook", "Claude.Code"); res.code != 2 {
		t.Fatalf("hook Claude.Code exited with %d, want 2", res.code)
	}
}
