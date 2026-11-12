package test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestPluginFiles checks the Claude Code and Codex plugin files: every
// manifest parses, the shared hooks file runs the bare `hook` command, and
// the plugin's skill names itself and has a description.
func TestPluginFiles(t *testing.T) {
	for _, f := range []string{
		"../.claude-plugin/plugin.json", "../.claude-plugin/marketplace.json",
		"../.codex-plugin/plugin.json", "../.agents/plugins/marketplace.json", "../hooks/hooks.json",
	} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	hooks, _ := os.ReadFile("../hooks/hooks.json")
	if strings.Count(string(hooks), `"git multistage hook"`) != 3 || !strings.Contains(string(hooks), "SessionStart") {
		t.Fatalf("hooks.json does not run `git multistage hook` at session start and around edits:\n%s", hooks)
	}
	skill, err := os.ReadFile("../skills/git-multistage/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	// A Windows checkout may convert the file to CRLF.
	parts := strings.SplitN(strings.ReplaceAll(string(skill), "\r\n", "\n"), "---\n", 3)
	if len(parts) != 3 || !strings.Contains(parts[1], "name: git-multistage\n") || !strings.Contains(parts[1], "description: ") {
		t.Fatal("skills/git-multistage/SKILL.md must start with front matter that holds `name: git-multistage` and a description")
	}
}
