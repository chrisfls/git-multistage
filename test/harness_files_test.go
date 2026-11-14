package test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// hookCommands returns every hook command in a parsed hook file: each
// string under a "command" key, and each "command" array joined by spaces.
func hookCommands(v any) []string {
	var out []string
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			if k != "command" {
				out = append(out, hookCommands(x)...)
				continue
			}
			switch c := x.(type) {
			case string:
				out = append(out, c)
			case []any:
				words := make([]string, 0, len(c))
				for _, w := range c {
					s, _ := w.(string)
					words = append(words, s)
				}
				out = append(out, strings.Join(words, " "))
			}
		}
	case []any:
		for _, x := range v {
			out = append(out, hookCommands(x)...)
		}
	}
	return out
}

func readJSON(t *testing.T, path string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return v
}

// TestHarnessHookFiles checks that each harness's hook file runs `git
// multistage hook` with that harness's editor name, at each event that
// records an edit.
func TestHarnessHookFiles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		path, editor string
		words        []string // the events and tool names that the file must name
	}{
		{"../hooks/cursor.json", "cursor", []string{"sessionStart", "preToolUse", "postToolUse", "postToolUseFailure", `"Write"`}},
		{"../.kimi-plugin/plugin.json", "kimi", []string{"SessionStart", "PreToolUse", "PostToolUse", "PostToolUseFailure", "Write|Edit"}},
		{"../.muse-plugin/plugin.json", "muse", []string{"SessionStart", "PreToolUse", "PostToolUse", "skills/git-multistage/SKILL.md"}},
		{"../plugins/gemini/hooks/hooks.json", "gemini", []string{"SessionStart", "BeforeTool", "AfterTool", "write_file|replace"}},
		{"../hooks/qwen.json", "qwen", []string{"SessionStart", "PreToolUse", "PostToolUse", "PostToolUseFailure", "edit|write_file"}},
		{"../hooks/kiro.json", "kiro", []string{"SessionStart", "PreToolUse", "PostToolUse", "fs_write"}},
	} {
		v := readJSON(t, tc.path)
		cmds := hookCommands(v)
		if len(cmds) < 3 {
			t.Errorf("%s: %d hook commands, want at least 3", tc.path, len(cmds))
		}
		for _, c := range cmds {
			if c != "git multistage hook "+tc.editor {
				t.Errorf("%s: hook command %q, want %q", tc.path, c, "git multistage hook "+tc.editor)
			}
		}
		data, _ := json.Marshal(v)
		for _, w := range tc.words {
			if !strings.Contains(string(data), w) {
				t.Errorf("%s does not name %s", tc.path, w)
			}
		}
	}
}

// TestHarnessManifests checks the manifests that point to other files: the
// files they name exist, and each manifest parses.
func TestHarnessManifests(t *testing.T) {
	t.Parallel()
	cursor, _ := readJSON(t, "../.cursor-plugin/plugin.json").(map[string]any)
	if cursor["hooks"] != "./hooks/cursor.json" || cursor["skills"] != "./skills/" {
		t.Errorf(".cursor-plugin/plugin.json names hooks %v and skills %v", cursor["hooks"], cursor["skills"])
	}
	for _, f := range []string{
		"../.cursor-plugin/marketplace.json", "../.muse-plugin/marketplace.json",
		"../plugins/gemini/gemini-extension.json", "../plugins/vibe/plugin.json",
	} {
		readJSON(t, f)
	}
	vibe, _ := readJSON(t, "../plugins/vibe/plugin.json").(map[string]any)
	ext, _ := vibe["extensions"].(map[string]any)
	if _, ok := ext["ai.mistral.vibe"]; !ok || vibe["$schema"] == nil {
		t.Error(`plugins/vibe/plugin.json needs "$schema" and extensions."ai.mistral.vibe", or Vibe skips its hooks`)
	}
	toml, err := os.ReadFile("../plugins/vibe/ai.mistral.vibe/hooks.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"session_start", "pre_tool", "post_tool"} {
		if !regexp.MustCompile(`type = "` + typ + `"\n(match = "re:\^\(edit\|write_file\)\$"\n)?command = "git multistage hook vibe"`).Match(toml) {
			t.Errorf("hooks.toml has no %s hook that runs `git multistage hook vibe`", typ)
		}
	}
}

// TestSharedHooksFile checks that the Claude Code hooks file, which many
// harnesses read, selects their file tools. A root hooks.json would make
// Antigravity merge the file into it at install and skip the result.
func TestSharedHooksFile(t *testing.T) {
	t.Parallel()
	shared, err := os.ReadFile("../hooks/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("../hooks.json"); err == nil {
		t.Error("the repository holds a root hooks.json, which breaks the Antigravity install")
	}
	var v struct {
		Hooks map[string][]struct{ Matcher string }
	}
	if err := json.Unmarshal(shared, &v); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"PreToolUse", "PostToolUse"} {
		if len(v.Hooks[event]) != 1 {
			t.Fatalf("hooks/hooks.json has %d %s entries, want 1", len(v.Hooks[event]), event)
		}
		names := strings.Split(v.Hooks[event][0].Matcher, "|")
		for _, tool := range []string{
			"Edit", "MultiEdit", "Write", "apply_patch", "write_to_file", "replace_file_content", "Create",
			"ApplyPatch", "search_replace", "file_editor", "write", "edit",
		} {
			if !slices.Contains(names, tool) {
				t.Errorf("the %s matcher of hooks/hooks.json does not select %s", event, tool)
			}
		}
	}
}

// TestSkillCopies checks that the plugin folders that harnesses copy at
// install hold the skill as it is.
func TestSkillCopies(t *testing.T) {
	t.Parallel()
	read := func(p string) string {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n") // a Windows checkout may convert the files
	}
	want := read("../skills/git-multistage/SKILL.md")
	for _, p := range []string{"../plugins/gemini/skills/git-multistage/SKILL.md", "../plugins/vibe/skills/git-multistage/SKILL.md"} {
		if read(p) != want {
			t.Errorf("%s differs from skills/git-multistage/SKILL.md; copy the skill again", p)
		}
	}
}

// clineModes returns the mode of each file in hooks/cline: from the Git
// index, or from the disk in an export without .git.
func clineModes(t *testing.T) map[string]string {
	t.Helper()
	modes := map[string]string{}
	out, err := exec.CommandContext(context.Background(), "git", "-C", "..", "ls-files", "-s", "--", "hooks/cline").Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if f := strings.Fields(line); len(f) == 4 {
				modes[path.Base(f[3])] = f[0]
			}
		}
		return modes
	}
	entries, err := os.ReadDir("../hooks/cline")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		modes[e.Name()] = "100644"
		if info.Mode()&0o111 != 0 || runtime.GOOS == "windows" {
			modes[e.Name()] = "100755"
		}
	}
	return modes
}

// TestClineHooks checks that Cline's hook files run the hook and are
// executable, so a checkout on macOS or Linux can run them.
func TestClineHooks(t *testing.T) {
	t.Parallel()
	modes := clineModes(t)
	names := make([]string, 0, len(modes))
	for name := range modes {
		names = append(names, name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"PostToolUse", "PreToolUse", "TaskStart"}) {
		t.Errorf("hooks/cline holds %q, want PostToolUse, PreToolUse, and TaskStart", names)
	}
	for _, name := range names {
		data, err := os.ReadFile("../hooks/cline/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if modes[name] != "100755" || !strings.HasPrefix(string(data), "#!/bin/sh\n") ||
			!strings.Contains(string(data), "\nexec git multistage hook cline\n") {
			t.Errorf("hooks/cline/%s is not an executable script that runs `git multistage hook cline`", name)
		}
	}
}
