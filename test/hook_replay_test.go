package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// replayHook runs the command of hooks/hooks.json with the saved input
// testdata/hooks/<name>.json, after it replaces {{CWD}} with the repository
// and {{FILE}} with file.
func (r *repo) replayHook(name, file string) result {
	r.t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "hooks", name+".json"))
	if err != nil {
		r.t.Fatal(err)
	}
	quote := func(s string) string {
		q, _ := json.Marshal(s)
		return strings.Trim(string(q), `"`)
	}
	in := strings.NewReplacer("{{CWD}}", quote(r.dir), "{{FILE}}", quote(file)).Replace(string(data))
	if !json.Valid([]byte(in)) {
		r.t.Fatalf("%s is not valid JSON after the replacements:\n%s", name, in)
	}
	return runStdin(r.dir, "", nil, in, "hook")
}

// TestHookReplay replays Claude Code and Codex hook inputs in their
// documented shapes through `git multistage hook`, the command that
// hooks/hooks.json runs, around a real edit. The stage named by the input
// must record the edit, and add --edits must commit exactly it.
func TestHookReplay(t *testing.T) {
	hooks, err := os.ReadFile("../hooks/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(hooks), `"command": "git multistage hook"`) != 3 {
		t.Fatalf("hooks.json no longer runs `git multistage hook` for each event:\n%s", hooks)
	}
	for _, tc := range []struct {
		name, pre, post, path, stage, content string
	}{
		{
			"Claude Code Edit", "claude-pre-edit", "claude-post-edit", "f.txt",
			"claude-8f2c1d3e-5b6a-4c7d-9e0f-1a2b3c4d5e6f", "one\nTWO\nthree\n",
		},
		{
			"Claude Code subagent Write", "claude-subagent-pre-write", "claude-subagent-post-write", "new.txt",
			"claude-a7c41e9b2d", "new file\n",
		},
		{
			"Codex apply_patch", "codex-pre-apply-patch", "codex-post-apply-patch", "f.txt",
			"codex-019a7c2e-4f1b-7d30-9a8e-5c6b7d8e9f01", "one\nTWO\nthree\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRepo(t)
			r.write("f.txt", "one\ntwo\nthree\n")
			r.git("add", "f.txt")
			r.git("commit", "-q", "-m", "f")
			file := r.path(tc.path)
			if res := r.replayHook(tc.pre, file); res.code != 0 {
				t.Fatalf("PreToolUse:\n%s", res)
			}
			r.write(tc.path, tc.content)
			post := r.replayHook(tc.post, file)
			if post.code != 0 || !strings.Contains(post.out, `"hookEventName":"PostToolUse"`) || !strings.Contains(post.out, tc.stage) {
				t.Fatalf("PostToolUse does not announce stage %s:\n%s", tc.stage, post)
			}
			r.mustMS("", "--stage", tc.stage, "add", "--edits")
			r.mustMS("", "--stage", tc.stage, "commit", "-m", tc.name)
			if got := r.git("show", "HEAD:"+tc.path); got != tc.content {
				t.Fatalf("committed %q, want %q", got, tc.content)
			}
		})
	}
	for _, name := range []string{"claude-session-start", "codex-session-start"} {
		t.Run(name, func(t *testing.T) {
			r := newRepo(t)
			res := r.replayHook(name, "")
			if res.code != 0 || !strings.Contains(res.out, `"hookEventName":"SessionStart"`) || !strings.Contains(res.out, "Other agents may edit files") {
				t.Fatalf("SessionStart:\n%s", res)
			}
		})
	}
}
