package ms

import "testing"

func TestAgentHints(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", "")
	t.Setenv("ORCA_AGENT_SESSION_ID", "")
	t.Setenv("PASEO_AGENT_ID", "")
	if got := forAgent("person", "agent"); got != "person" {
		t.Fatalf("without an agent: %q", got)
	}
	e := &Error{Message: "open .git/index.lock: operation not permitted"}
	if addCodexHint(e); e.Hint != "" {
		t.Fatalf("outside Codex: %q", e.Hint)
	}
	t.Setenv("CODEX_THREAD_ID", "t")
	if got := forAgent("person", "agent"); got != "agent" {
		t.Fatalf("in Codex: %q", got)
	}
	if addCodexHint(e); e.Hint != hintCodexSandbox {
		t.Fatalf("in Codex: %q", e.Hint)
	}
}
