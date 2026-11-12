package test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// These tests reproduce defects found in the third review.

// TestHunksKeepCommittedCRLF lists the hunks of a file whose committed blob
// has CRLF line endings under core.autocrlf: Git keeps CRLF for such a file,
// so the hunks are the changed lines only, and staging one leaves the other.
func TestHunksKeepCommittedCRLF(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "a\r\nb\r\nc\r\nd\r\ne\r\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "crlf")
	r.git("config", "core.autocrlf", "true")
	r.write("f.txt", "a\r\nB\r\nc\r\nd\r\nOTHER\r\n")
	var fj filesJSON
	decode(t, r.mustMS("a", "hunks", "--json", "f.txt"), &fj)
	if len(fj.Files) != 1 || len(fj.Files[0].Hunks) != 2 {
		t.Fatalf("want 2 hunks:\n%+v", fj)
	}
	r.mustMS("a", "add", "f.txt", "--hunk", r.hunkID("a", "f.txt", "B\r"))
	r.mustMS("a", "commit", "-m", "B")
	if got := r.git("show", "main:f.txt"); got != "a\r\nB\r\nc\r\nd\r\ne\r\n" {
		t.Fatalf("committed %q", got)
	}
}

// TestHunksIgnoreGitDiffOpts lists hunks with GIT_DIFF_OPTS in the caller's
// environment, which would otherwise override -U0.
func TestHunksIgnoreGitDiffOpts(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", lines(10, func(i int) string { return fmt.Sprintf("line%d", i) }))
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.write("f.txt", strings.Replace(r.read("f.txt"), "line5\n", "LINE5\n", 1))
	res := run(r.dir, "a", []string{"GIT_DIFF_OPTS=--unified=3"}, "hunks", "--json", "f.txt")
	var fj filesJSON
	decode(t, res, &fj)
	if res.code != 0 || len(fj.Files) != 1 || len(fj.Files[0].Hunks) != 1 || len(fj.Files[0].Hunks[0].Lines) != 2 {
		t.Fatalf("want one hunk of two lines:\n%s", res)
	}
}

// TestAddEditsKeepsRacyEntriesRacy rewrites a staged file in the second in
// which it was staged, with the same size. `add --edits` of another file must
// not make that entry look clean, or a later `add` keeps the old content.
func TestAddEditsKeepsRacyEntriesRacy(t *testing.T) {
	r := newRepo(t)
	r.write("g.txt", "g\n")
	r.git("add", "g.txt")
	r.git("commit", "-q", "-m", "g")
	r.hookedEdit("s", "", "g.txt", "g\ng2\n")
	for attempt := 0; ; attempt++ {
		start := time.Now().Unix()
		r.write("f.txt", "old\n")
		r.mustMS("", "--stage", "claude-s", "add", "f.txt")
		r.write("f.txt", "new\n")
		if time.Now().Unix() == start {
			break
		}
		if attempt == 5 {
			t.Skip("cannot stage and rewrite a file within one second")
		}
	}
	time.Sleep(time.Until(time.Unix(time.Now().Unix()+1, 0)) + 50*time.Millisecond)
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "claude-s", "add", "f.txt")
	r.mustMS("", "--stage", "claude-s", "commit", "-m", "f and g")
	if got := r.git("show", "main:f.txt"); got != "new\n" {
		t.Fatalf("committed f.txt = %q, want the rewritten content", got)
	}
}
