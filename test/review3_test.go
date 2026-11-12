package test

import (
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
