package test

import (
	"strings"
	"testing"
)

// TestBranchMismatchHint commits a stage after HEAD moved to another
// branch. A person gets a hint to check out the branch again; an agent gets
// one to stop and report it.
func TestBranchMismatchHint(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("README", "changed\n")
	r.mustMS("", "--stage", "a", "add", "README")
	r.git("switch", "-q", "-c", "other")
	person := r.ms("", "--stage", "a", "commit", "-m", "x")
	if person.code != 5 || !strings.Contains(person.err, "hint: check out main again") {
		t.Fatalf("without an agent:\n%s", person)
	}
	agent := run(r.dir, "", []string{"CLAUDE_CODE_SESSION_ID=s"}, "--stage", "a", "commit", "-m", "x")
	if agent.code != 5 || !strings.Contains(agent.err, "hint: stop and report it") || strings.Contains(agent.err, "check out") {
		t.Fatalf("in an agent:\n%s", agent)
	}
}
