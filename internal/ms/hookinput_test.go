package ms

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// hookCallCases are hook inputs in the shape of each supported harness,
// with the values that the hook takes from them. Paths are relative to /w.
var hookCallCases = []struct {
	name, input                    string
	event, tool, session, use, dir string
	shape                          string
	files                          []string
}{
	{
		"Claude Code Edit",
		`{"hook_event_name":"PreToolUse","tool_name":"Edit","session_id":"s","tool_use_id":"t","cwd":"/w",` +
			`"tool_input":{"file_path":"a.go","old_string":"x","new_string":"y"}}`,
		eventPreTool, "Edit", "s", "t", "/w", "",
		[]string{wPath("a.go")},
	},
	{
		"Codex apply_patch",
		`{"hook_event_name":"PostToolUse","tool_name":"apply_patch","session_id":"s","tool_use_id":"t","cwd":"/w",` +
			`"tool_input":{"command":"*** Begin Patch\n*** Update File: a.go\n@@\n-x\n+y\n*** End Patch\n"}}`,
		eventPostTool, toolApplyPatch, "s", "t", "/w", "",
		[]string{wPath("a.go")},
	},
	{
		"Antigravity failed write",
		`{"hookEventName":"PostToolUse","conversationId":"c1","cwd":"/w","error":"exit status 1",` +
			`"toolCall":{"id":"t1","name":"write_to_file","args":{"TargetFile":"a.go","CodeContent":"x"}}}`,
		eventPostFailure, "write_to_file", "c1", "t1", "/w", "antigravity",
		[]string{wPath("a.go")},
	},
	{
		"Antigravity arguments as a string",
		`{"hookEventName":"PreToolUse","conversationId":"c1","cwd":"/w",` +
			`"toolCall":{"id":"t1","name":"replace_file_content","args":"{\"TargetFile\":\"b.go\"}"}}`,
		eventPreTool, "replace_file_content", "c1", "t1", "/w", "antigravity",
		[]string{wPath("b.go")},
	},
	{
		"Gemini CLI AfterTool",
		`{"hook_event_name":"AfterTool","session_id":"g","cwd":"/w","tool_name":"replace","tool_input":{"file_path":"a.go"}}`,
		eventPostTool, "replace", "g", "", "/w", "gemini",
		[]string{wPath("a.go")},
	},
	{
		"Mistral Vibe failed write",
		`{"hook_event_name":"post_tool","session_id":"v","cwd":"/w","tool_name":"file_system.write_file",` +
			`"tool_call_id":"c","tool_input":{"path":"a.go"},"tool_status":"failure"}`,
		eventPostFailure, "file_system.write_file", "v", "c", "/w", "vibe",
		[]string{wPath("a.go")},
	},
	{
		"Cline",
		`{"clineVersion":"3.36","hookName":"PostToolUse","taskId":"t1","workspaceRoots":["/w"],` +
			`"postToolUse":{"toolName":"write_to_file","parameters":{"path":"a.go"},"result":"ok","success":true}}`,
		eventPostTool, "write_to_file", "t1", "", "/w", "cline",
		[]string{wPath("a.go")},
	},
	{
		"Cursor",
		`{"hook_event_name":"preToolUse","conversation_id":"k","cursor_version":"2.3","tool_name":"Write",` +
			`"tool_use_id":"u","cwd":"/w","tool_input":{"file_path":"a.go"}}`,
		eventPreTool, "Write", "k", "u", "/w", "cursor",
		[]string{wPath("a.go")},
	},
	{
		"OpenHands",
		`{"event_type":"PreToolUse","session_id":"o","working_dir":"/w","tool_name":"file_editor",` +
			`"tool_input":{"command":"create","path":"a.go","file_text":"x"}}`,
		eventPreTool, "file_editor", "o", "", "/w", "openhands",
		[]string{wPath("a.go")},
	},
	{
		"Goose",
		`{"event":"PreToolUse","session_id":"g","working_dir":"/w","tool_name":"write","tool_call_id":"c",` +
			`"matcher_context":"write","tool_input":{"path":"a.go","content":"x"}}`,
		eventPreTool, "write", "g", "c", "/w", "goose",
		[]string{wPath("a.go")},
	},
	{
		"Grok",
		`{"hookEventName":"PreToolUse","hook_event_name":"PreToolUse","sessionId":"x","session_id":"x",` +
			`"toolName":"search_replace","tool_name":"search_replace","toolUseId":"u","cwd":"/w",` +
			`"tool_input":{"file_path":"a.go"}}`,
		eventPreTool, "search_replace", "x", "u", "/w", "",
		[]string{wPath("a.go")},
	},
	{
		"Copilot raw patch",
		`{"hook_event_name":"PreToolUse","session_id":"p","tool_name":"apply_patch","tool_use_id":"u","cwd":"/w",` +
			`"tool_input":"*** Begin Patch\n*** Add File: b.go\n+x\n*** End Patch\n"}`,
		eventPreTool, toolApplyPatch, "p", "u", "/w", "",
		[]string{wPath("b.go")},
	},
	{
		"OpenCode patchText",
		`{"hook_event_name":"PreToolUse","session_id":"o","tool_name":"apply_patch","tool_use_id":"x","cwd":"/w",` +
			`"tool_input":{"patchText":"*** Begin Patch\n*** Update File: a.go\n*** Move to: b.go\n*** End Patch\n"}}`,
		eventPreTool, toolApplyPatch, "o", "x", "/w", "",
		[]string{wPath("a.go")},
	},
	{
		"Hermes patch mode",
		`{"hook_event_name":"PreToolUse","session_id":"h","tool_name":"patch","tool_use_id":"x","cwd":"/w",` +
			`"tool_input":{"mode":"patch","patch":"*** Begin Patch\n*** Delete File: a.go\n*** End Patch\n"}}`,
		eventPreTool, "patch", "h", "x", "/w", "",
		[]string{wPath("a.go")},
	},
	{
		"Kimi Code Write",
		`{"hook_event_name":"PreToolUse","session_id":"k","tool_name":"Write","tool_call_id":"c","cwd":"/w",` +
			`"tool_input":{"path":"a.go","content":"x","mode":"append"}}`,
		eventPreTool, "Write", "k", "c", "/w", "",
		[]string{wPath("a.go")},
	},
	{
		"Shell tool",
		`{"hook_event_name":"PreToolUse","session_id":"s","tool_name":"Bash","tool_use_id":"t","cwd":"/w",` +
			`"tool_input":{"command":"ls"}}`,
		eventPreTool, "Bash", "s", "t", "/w", "", nil,
	},
}

// wPath returns name in the directory /w of hookCallCases.
func wPath(name string) string { return filepath.Join(filepath.FromSlash("/w"), name) }

// TestReadHookCall reads one hook input in the shape of each supported
// harness, and checks the values that the hook takes from it.
func TestReadHookCall(t *testing.T) {
	t.Parallel()
	for _, tc := range hookCallCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := readHookCall(strings.NewReader(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			got := []string{c.Event, c.Tool, c.SessionID, c.Cwd, c.shape}
			want := []string{tc.event, tc.tool, tc.session, tc.dir, tc.shape}
			if !slices.Equal(got, want) {
				t.Fatalf("event, tool, session, directory, shape = %q, want %q", got, want)
			}
			if tc.use != "" && c.ToolUseID != tc.use || tc.use == "" && !strings.HasPrefix(c.ToolUseID, "call-") {
				t.Fatalf("tool call ID = %q, want %q or a call key", c.ToolUseID, tc.use)
			}
			if files := callFiles(c); !slices.Equal(files, tc.files) {
				t.Fatalf("files = %q, want %q", files, tc.files)
			}
		})
	}
}

// TestCallKey checks that the pre-tool and post-tool inputs of one call
// without a tool call ID get the same key, and other calls another.
func TestCallKey(t *testing.T) {
	t.Parallel()
	read := func(event, path string) string {
		c, err := readHookCall(strings.NewReader(`{"event_type":"` + event + `","session_id":"o","working_dir":"/w",` +
			`"tool_name":"file_editor","tool_input":{"command":"create","path":"` + path + `"}}`))
		if err != nil {
			t.Fatal(err)
		}
		return c.ToolUseID
	}
	if pre, post := read(eventPreTool, "a.go"), read(eventPostTool, "a.go"); pre != post {
		t.Fatalf("pre-tool key %q differs from post-tool key %q", pre, post)
	}
	if read(eventPreTool, "a.go") == read(eventPreTool, "b.go") {
		t.Fatal("calls that edit different files got the same key")
	}
}

// TestPickEditor checks the order in which a bare hook command picks its
// editor.
func TestPickEditor(t *testing.T) {
	t.Parallel()
	read := func(input string) hookCall {
		c, err := readHookCall(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	claude := read(`{"hook_event_name":"PreToolUse","tool_name":"Edit","session_id":"s","tool_input":{"file_path":"a"}}`)
	codex := read(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","session_id":"s",` +
		`"tool_input":{"command":"*** Begin Patch\n*** Add File: a\n+x\n*** End Patch\n"}}`)
	opencode := read(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","session_id":"s",` +
		`"tool_input":{"patchText":"*** Begin Patch\n*** Add File: a\n+x\n*** End Patch\n"}}`)
	agy := read(`{"hookEventName":"SessionStart","conversationId":"c1","cwd":"/w"}`)
	env := func(vars ...string) func(string) string {
		return func(k string) string {
			if slices.Contains(vars, k) {
				return "1"
			}
			return ""
		}
	}
	for _, tc := range []struct {
		name, arg string
		call      hookCall
		env       func(string) string
		want      string
	}{
		{"name in the command", "kimi", agy, env(), "kimi"},
		{"input shape", "", agy, env("COPILOT_CLI"), "antigravity"},
		{"harness variable before Claude Code's", "", claude, env("CLAUDE_PLUGIN_ROOT", "MINIMAX_PLUGIN_ROOT"), "minimax"},
		{"Codex patch", "", codex, env(), agentCodex},
		{"patch outside command", "", opencode, env(), agentClaude},
		{"Claude Code", "", claude, env("CLAUDE_PLUGIN_ROOT"), agentClaude},
	} {
		if got := pickEditor(tc.arg, tc.call, tc.env); got != tc.want {
			t.Errorf("%s: editor %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestValidEditor(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"claude": true, "open-hands": true, "pi2": true, "": false, "Claude": false,
		"claude.code": false, "2pi": false, "a b": false, strings.Repeat("a", 33): false,
	} {
		if validEditor(name) != want {
			t.Errorf("validEditor(%q) = %v, want %v", name, !want, want)
		}
	}
}

// TestHookMessage checks the output format of each harness.
func TestHookMessage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ editor, event, native, want string }{
		{"claude", eventPostTool, eventPostTool, `{"hookSpecificOutput":{"additionalContext":"m","hookEventName":"PostToolUse"}}`},
		{"gemini", eventPostTool, "AfterTool", `{"hookSpecificOutput":{"additionalContext":"m","hookEventName":"AfterTool"}}`},
		{"copilot", eventPostTool, eventPostTool, `{"additionalContext":"m"}`},
		{"openhands", eventSessionStart, eventSessionStart, `{"additionalContext":"m"}`},
		{"cursor", eventPostTool, "postToolUse", `{"additional_context":"m"}`},
		{"antigravity", eventSessionStart, eventSessionStart, `{"injectSteps":[{"systemMessage":{"systemMessage":"m"}}]}`},
		{"antigravity", eventPostTool, eventPostTool, ``},
		{"vibe", eventPostTool, "post_tool", `{"hook_specific_output":{"additional_context":"m"}}`},
		{"kiro", eventSessionStart, eventSessionStart, `m`},
		{"cline", eventPostTool, eventPostTool, `{"contextModification":"m"}`},
	} {
		c := hookCall{editor: tc.editor, event: tc.native}
		c.Event = tc.event
		if got := c.message("m"); got != tc.want {
			t.Errorf("%s at %s: %s, want %s", tc.editor, tc.native, got, tc.want)
		}
	}
	for editor, want := range map[string]bool{"antigravity": true, "kiro": true, "claude": false, "gemini": false} {
		if noticeNamesStage(editor) != want {
			t.Errorf("noticeNamesStage(%q) = %v, want %v", editor, !want, want)
		}
	}
}
