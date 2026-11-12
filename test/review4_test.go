package test

import (
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
