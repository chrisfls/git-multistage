package test

import (
	"fmt"
	"strings"
	"testing"
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
