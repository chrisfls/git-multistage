package test

import (
	"strconv"
	"strings"
	"testing"
)

// These tests reproduce defects found in the fourth review.

func TestJSONOutputKeepsStageWarnings(t *testing.T) {
	r := newRepo(t)
	var out struct {
		Warnings []string `json:"warnings"`
	}
	res := r.mustMS("agent-a", "status", "--json")
	decode(t, res, &out)
	if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "created stage agent-a") {
		t.Fatalf("status --json warnings = %q:\n%s", out.Warnings, res)
	}
	res = r.mustMS("agent-b", "status")
	if n := strings.Count(res.err, "created stage agent-b"); n != 1 {
		t.Fatalf("text mode printed the warning %d times:\n%s", n, res)
	}
}

func TestLogFiltersByThePathspec(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.mustMS("agent-a", "add", "a.txt")
	r.mustMS("agent-a", "commit", "-m", "add a")
	r.write("b.txt", "b\n")
	r.mustMS("agent-b", "add", "b.txt")
	r.mustMS("agent-b", "commit", "-m", "add b")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"log", "--format=%s"}, "add a\n"},
		{[]string{"log", "--format=%s", "a.txt"}, "add a\n"},
		{[]string{"log", "--format=%s", "--", "a.txt"}, "add a\n"},
		{[]string{"log", "--format=%s", "--", "b.txt"}, ""},
	} {
		if res := r.mustMS("agent-a", c.args...); res.out != c.want {
			t.Errorf("%s = %q, want %q", strings.Join(c.args, " "), res.out, c.want)
		}
	}
}

func TestHookAfterGlobalOptions(t *testing.T) {
	r := newRepo(t)
	file := r.path("README")
	call := func(event string, args ...string) {
		t.Helper()
		in := `{"hook_event_name":"` + event + `","tool_name":"Edit","session_id":"s","tool_use_id":"toolu_1",` +
			`"tool_input":{"file_path":` + strconv.Quote(file) + `}}`
		if res := runStdin(r.dir, "", nil, in, args...); res.code != 0 {
			t.Fatalf("%s %s:\n%s", strings.Join(args, " "), event, res)
		}
	}
	call("PreToolUse", "--stage", "mine", "hook", "claude")
	r.write("README", "edited\n")
	call("PostToolUse", "--stage=mine", "--json", "hook", "claude")
	r.mustMS("mine", "add", "--edits")
	if got := r.stagedPaths("mine"); !eq(got, []string{"README"}) {
		t.Fatalf("staged %q", got)
	}
}
