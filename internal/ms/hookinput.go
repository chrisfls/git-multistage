package ms

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// Each harness sends its hook input in its own shape: snake_case or
// camelCase fields, Antigravity's toolCall, Cline's nested event object.
// readHookCall reads any of them into the shape of a Claude Code input,
// and keeps what the hook needs besides: the harness's own event name for
// the output, the editor that the shape names, and the tool input.

// hookCall is a hook input of any supported harness.
type hookCall struct {
	hookInput
	editor string // the editor of the call, which names its stage and its output format
	event  string // the event name that the harness sent
	shape  string // the editor that the input's shape names, or ""
	args   any    // the tool input, decoded
	patch  string // an apply_patch patch in the tool input, or ""
}

// Hook events.
const (
	eventSessionStart = "SessionStart"
	eventPreTool      = "PreToolUse"
	eventPostTool     = "PostToolUse"
	eventPostFailure  = "PostToolUseFailure"
)

// eventNames maps each harness's event names to the hook's.
var eventNames = map[string]string{
	"sessionStart": eventSessionStart, "session_start": eventSessionStart,
	"agentSpawn": eventSessionStart, "TaskStart": eventSessionStart,
	"preToolUse": eventPreTool, "BeforeTool": eventPreTool, "pre_tool": eventPreTool,
	"postToolUse": eventPostTool, "AfterTool": eventPostTool, "post_tool": eventPostTool,
	"postToolUseFailure": eventPostFailure,
}

// The field names of each value, in the order the hook reads them.
var (
	eventKeys    = []string{"hook_event_name", fieldEventName, "event_type", "event", "hookName"}
	toolKeys     = []string{"tool_name", "toolName"}
	argsKeys     = []string{"tool_input", "toolInput", "toolArgs", "tool_args", "args"}
	responseKeys = []string{"tool_response", "toolResponse", "toolResult", "tool_output"}
	sessionKeys  = []string{"session_id", "sessionId", "conversation_id", "conversationId", "taskId"}
	agentKeys    = []string{"agent_id", "agentId"}
	callKeys     = []string{"tool_use_id", "toolUseId", "tool_call_id", "toolCallId"}
	dirKeys      = []string{"cwd", "working_dir", "workspaceRoot"}
	rootsKeys    = []string{"workspace_roots", "workspaceRoots", "workspacePaths"}
	// patchKeys are the fields that hold an apply_patch patch, tried first.
	patchKeys = []string{"command", "patchText", "patch_text", "patch", "input"}
	// pathKeys are the fields that name the file of a file tool.
	pathKeys = []string{"file_path", "filePath", "path", "TargetFile", "target_file", "targetFile"}
)

// fileTools are the file tools of the supported harnesses, in lower case.
var fileTools = map[string]bool{
	"edit": true, "multiedit": true, "multi_edit": true, "write": true, "create": true,
	"applypatch": true, toolApplyPatch: true, "patch": true, "str_replace_editor": true,
	"str_replace": true, "search_replace": true, "write_file": true, "edit_file": true,
	"create_file": true, "write_to_file": true, "replace_file_content": true,
	"multi_replace_file_content": true, "replace": true, "replace_in_file": true,
	"file_editor": true, "fs_write": true, "fs_append": true,
}

// Fields of hook inputs and of Claude Code's hook output.
const (
	fieldEventName = "hookEventName"      // the event name in camelCase inputs and in hookSpecificOutput
	fieldOutput    = "hookSpecificOutput" // Claude Code's output for one event
	fieldContext   = "additionalContext"  // text for the model
)

// patchStart begins an apply_patch patch.
const patchStart = "*** Begin Patch"

// readHookCall reads the hook input of any supported harness.
func readHookCall(r io.Reader) (hookCall, error) {
	var m map[string]any
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return hookCall{}, err
	}
	c := hookCall{event: str(m, eventKeys...), shape: inputShape(m)}
	in := map[string]any{
		"session_id": str(m, sessionKeys...), "agent_id": str(m, agentKeys...),
		"tool_name": str(m, toolKeys...), "tool_use_id": str(m, callKeys...),
		"cwd": workDir(m), "tool_response": value(m, responseKeys...),
	}
	c.args = value(m, argsKeys...)
	if call, ok := m["toolCall"].(map[string]any); ok { // Antigravity
		in["tool_name"], in["tool_use_id"], c.args = str(call, "name"), str(call, "id"), call["args"]
	}
	nested, _ := m[lowerFirst(c.event)].(map[string]any)
	if nested != nil { // Cline
		in["tool_name"], c.args, in["tool_response"] = str(nested, "toolName"), nested["parameters"], nested["result"]
	}
	c.args = decodeArgs(c.args)
	in["hook_event_name"] = hookEvent(c.event, m, nested)
	if obj, ok := c.args.(map[string]any); ok {
		in["tool_input"] = obj
	}
	if in["tool_use_id"] == "" && in["tool_name"] != "" {
		in["tool_use_id"] = callKey(in["session_id"], in["tool_name"], c.args)
	}
	data, err := json.Marshal(in)
	if err == nil {
		err = json.Unmarshal(data, &c.hookInput)
	}
	c.patch = findPatch(c.args)
	return c, err
}

// hookEvent returns the hook's name for the event that a harness sent. A
// post-tool event that reports a failed call is a PostToolUseFailure.
func hookEvent(event string, m, nested map[string]any) string {
	if name, ok := eventNames[event]; ok {
		event = name
	}
	failed := str(m, "error") != "" || // Antigravity
		str(m, "tool_status") == "failure" || // Mistral Vibe
		nested["success"] == false // Cline
	if event == eventPostTool && failed {
		return eventPostFailure
	}
	return event
}

// inputShape returns the editor whose hook input has the shape of m, or "".
func inputShape(m map[string]any) string {
	has := func(k string) bool { _, ok := m[k]; return ok }
	switch {
	case has("toolCall") || has("conversationId"):
		return "antigravity"
	case has("cursor_version"):
		return "cursor"
	case has("clineVersion"):
		return "cline"
	case has("event_type"):
		return "openhands"
	case has("event") && !has("hook_event_name"):
		return "goose"
	}
	switch str(m, "hook_event_name") {
	case "pre_tool", "post_tool", "session_start", "session_end":
		return "vibe"
	case "BeforeTool", "AfterTool":
		return "gemini"
	}
	return ""
}

// harnessVars are the variables that harnesses set for their hook
// commands, and the editor that each names. A harness's own variable comes
// before the Claude Code variables that some of them also set.
var harnessVars = [][2]string{
	{"GROK_HOOK_EVENT", "grok"},
	{"COPILOT_CLI", "copilot"},
	{"MINIMAX_PLUGIN_ROOT", "minimax"},
	{"ZCODE_PLUGIN_ROOT", "zcode"},
	{"QODER_PLUGIN_ROOT", "qoder"},
	{"CODEBUDDY_PLUGIN_ROOT", "codebuddy"},
	{"DEVIN_PROJECT_DIR", "devin"},
	{"FACTORY_PROJECT_DIR", "droid"},
	{"GEMINI_SESSION_ID", "gemini"},
}

// pickEditor returns the editor of a hook call: the name in the hook
// command, else the input's shape, else a harness variable, else codex for
// a Codex apply_patch, else claude. Each rule gives every call of a session
// the same editor.
func pickEditor(name string, c hookCall, getenv func(string) string) string {
	if name != "" {
		return name
	}
	if c.shape != "" {
		return c.shape
	}
	for _, v := range harnessVars {
		if getenv(v[0]) != "" {
			return v[1]
		}
	}
	if c.Tool == toolApplyPatch && c.Input.Command != "" {
		return agentCodex
	}
	return agentClaude
}

// validEditor reports whether name can name an editor: lower-case letters,
// digits, and hyphens, starting with a letter.
func validEditor(name string) bool {
	if name == "" || len(name) > 32 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, ch := range name {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
			return false
		}
	}
	return true
}

// callFiles returns the files that a call edits, as absolute paths: the
// files of its patch, else the path of a file tool.
func callFiles(c hookCall) []string {
	if c.patch != "" {
		return patchFiles(c.patch, c.Cwd)
	}
	tool := strings.ToLower(c.Tool)
	if i := strings.LastIndexByte(tool, '.'); i >= 0 {
		tool = tool[i+1:] // Mistral Vibe: file_system.edit
	}
	if !fileTools[tool] {
		return nil
	}
	if p := str(asMap(c.args), pathKeys...); p != "" {
		return []string{absPath(p, c.Cwd)}
	}
	return nil
}

// patchFiles returns the files of a patch's Update File, Add File, and
// Delete File sections. The hook of an Update File section with Move to
// records the move.
func patchFiles(patch, cwd string) []string {
	var files []string
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimRight(line, "\r")
		for _, prefix := range []string{"*** Update File: ", "*** Add File: ", "*** Delete File: "} {
			if p, ok := strings.CutPrefix(line, prefix); ok && strings.TrimSpace(p) != "" {
				files = append(files, absPath(strings.TrimSpace(p), cwd))
			}
		}
	}
	return files
}

// findPatch returns the apply_patch patch that a tool input holds, or "".
func findPatch(args any) string {
	if s, ok := args.(string); ok && strings.Contains(s, patchStart) {
		return s
	}
	m := asMap(args)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range append(slices.Clone(patchKeys), keys...) {
		if s, ok := m[k].(string); ok && strings.Contains(s, patchStart) {
			return s
		}
	}
	return ""
}

// decodeArgs decodes a tool input that a harness sends as a JSON string,
// such as Copilot's toolArgs. Any other string, such as the raw patch of
// Copilot's apply_patch, stays a string.
func decodeArgs(args any) any {
	s, ok := args.(string)
	if !ok {
		return args
	}
	var m map[string]any
	if json.Unmarshal([]byte(s), &m) == nil {
		return m
	}
	return s
}

// callKey keys the events of one call that has no tool call ID by its
// session, tool, and tool input, which the pre-tool and post-tool inputs
// repeat. Encoding sorts the keys of a map, so the key is stable.
func callKey(session, tool, args any) string {
	data, _ := json.Marshal([]any{session, tool, args})
	sum := sha256.Sum256(data)
	return "call-" + hex.EncodeToString(sum[:8])
}

// workDir returns the directory of the hook call.
func workDir(m map[string]any) string {
	if d := str(m, dirKeys...); d != "" {
		return d
	}
	for _, k := range rootsKeys {
		if roots, ok := m[k].([]any); ok && len(roots) > 0 {
			if d, ok := roots[0].(string); ok {
				return d
			}
		}
	}
	return ""
}

func absPath(p, cwd string) string {
	if !filepath.IsAbs(p) {
		return filepath.Join(cwd, p)
	}
	return p
}

// str returns the first non-empty string under keys in m.
func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// value returns the first value under keys in m.
func value(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// message returns the output that gives msg to the model, in the format
// that the call's harness reads, or "" when the harness reads no output
// at the call's event. A harness without a format of its own gets Claude
// Code's.
func (c hookCall) message(msg string) string {
	var v any
	switch c.editor {
	case "copilot", "openhands":
		v = map[string]string{fieldContext: msg}
	case "cursor":
		v = map[string]string{"additional_context": msg}
	case "antigravity": // reads {} after a tool call
		if c.Event != eventSessionStart {
			return ""
		}
		v = map[string]any{"injectSteps": []any{map[string]any{"systemMessage": map[string]string{"systemMessage": msg}}}}
	case "vibe":
		v = map[string]any{"hook_specific_output": map[string]string{"additional_context": msg}}
	case "kiro":
		return msg
	case "cline":
		v = map[string]string{"contextModification": msg}
	default:
		v = map[string]any{fieldOutput: map[string]string{fieldEventName: c.event, fieldContext: msg}}
	}
	out, _ := json.Marshal(v)
	return string(out)
}

// noticeNamesStage reports whether the session start notice of editor
// names the stage, because the harness reads no output after a tool call.
func noticeNamesStage(editor string) bool {
	return editor == "antigravity" || editor == "kiro"
}
