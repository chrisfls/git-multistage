package test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestPluginFiles checks the Claude Code and Codex plugin files: every
// manifest parses, the shared hooks file runs the bare `hook` command, and
// the plugin's skill carries the same text as the agent guide.
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
	if strings.Count(string(hooks), `"git multistage hook"`) != 2 {
		t.Fatalf("hooks.json does not run `git multistage hook` before and after edits:\n%s", hooks)
	}
	skill, err := os.ReadFile("../skills/git-multistage/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	guide, err := os.ReadFile("../docs/agent-guide.md")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(skill), "---\n", 3)
	if len(parts) != 3 || strings.TrimPrefix(parts[2], "\n") != string(guide) {
		t.Fatal("skills/git-multistage/SKILL.md must hold docs/agent-guide.md after its front matter; copy the guide into it")
	}
}
