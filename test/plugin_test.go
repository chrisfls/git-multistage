package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestPluginFiles checks the Claude Code and Codex plugin files: every
// manifest parses, the shared hooks file runs the bare `hook` command, and
// the plugin's skill names itself and has a description.
func TestPluginFiles(t *testing.T) {
	t.Parallel()
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
	if strings.Count(string(hooks), `"git multistage hook"`) != 4 || !strings.Contains(string(hooks), "SubagentStart") {
		t.Fatalf("hooks.json does not run `git multistage hook` at session and subagent start and around edits:\n%s", hooks)
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
	// The agent reads the description to decide when to load the skill, so
	// it says when, in one sentence.
	_, desc, _ := strings.Cut(parts[1], "description: ")
	desc, _, _ = strings.Cut(desc, "\n")
	if !strings.HasPrefix(desc, "Use ") || strings.Count(desc, ". ") > 0 || !strings.HasSuffix(desc, ".") {
		t.Fatalf("the skill description must be one sentence that starts with `Use `: %q", desc)
	}
}

// TestPluginVersion checks that every plugin manifest carries one version.
// An installed plugin updates only when that version changes.
func TestPluginVersion(t *testing.T) {
	t.Parallel()
	versions := map[string]string{}
	for _, f := range []string{
		"../.claude-plugin/plugin.json", "../.claude-plugin/marketplace.json", "../.codex-plugin/plugin.json",
		"../.cursor-plugin/plugin.json", "../.cursor-plugin/marketplace.json", "../.kimi-plugin/plugin.json",
		"../.muse-plugin/plugin.json", "../.muse-plugin/marketplace.json", "../.hermes-plugin/plugin.yaml",
		"../package.json", "../plugins/vibe/plugin.json", "../plugins/gemini/gemini-extension.json",
	} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		// A manifest's own version and its marketplace entries' versions.
		for i, m := range regexp.MustCompile(`(?m)^\s*"?version"?\s*:\s*"?([0-9][^",\s]*)`).FindAllSubmatch(data, -1) {
			versions[fmt.Sprintf("%s#%d", f, i)] = string(m[1])
		}
		if !bytes.Contains(data, []byte("version")) {
			t.Errorf("%s names no version", f)
		}
	}
	want := versions["../.claude-plugin/plugin.json#0"]
	for _, v := range versions {
		if want == "" || v != want {
			t.Fatalf("the plugin versions differ: %v", versions)
		}
	}
}
