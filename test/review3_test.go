package test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chrisfls/git-multistage/internal/filelock"
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

// TestAddEditsRefusesARestoredDeletion records an edit that deletes lines;
// another writer then puts the lines back. The edit is lost, so `add --edits`
// must refuse it instead of staging the deletion.
func TestAddEditsRefusesARestoredDeletion(t *testing.T) {
	for _, tc := range []struct{ name, before, after string }{
		{"middle", "a\nb\nc\nd\ne\n", "a\nb\ne\n"},
		{"start", "a\nb\nc\nd\ne\n", "c\nd\ne\n"},
		{"end", "a\nb\nc\nd\ne\n", "a\nb\nc\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRepo(t)
			r.write("f.txt", tc.before)
			r.git("add", "f.txt")
			r.git("commit", "-q", "-m", "f")
			r.hookedEdit("s", "", "f.txt", tc.after)
			r.write("f.txt", tc.before)
			res := r.expectCode(3, "", "--stage", "claude-s", "add", "--edits")
			if !strings.Contains(res.out+res.err, "no longer holds the edit") {
				t.Fatalf("want edit-lost:\n%s", res)
			}
		})
	}
}

// TestAddEditsKeepsADeletionNextToAnotherInsertion deletes lines; another
// writer inserts a line elsewhere. The deletion survives and is staged.
func TestAddEditsKeepsADeletionNextToAnotherInsertion(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "a\nb\nc\nd\ne\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.hookedEdit("s", "", "f.txt", "a\nb\ne\n")
	r.write("f.txt", "a\nX\nb\ne\n")
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "claude-s", "commit", "-m", "delete")
	if got := r.git("show", "main:f.txt"); got != "a\nb\ne\n" {
		t.Fatalf("committed %q", got)
	}
}

// TestAddHunksIsAllOrNothing selects hunks of a tracked file and of an
// ignored new file. The ignored file is refused, so nothing is staged.
func TestAddHunksIsAllOrNothing(t *testing.T) {
	r := newRepo(t)
	r.write(".gitignore", "ign.txt\n")
	r.write("a.txt", "a\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "a")
	r.write("a.txt", "a\nA\n")
	r.write("ign.txt", "secret\n")
	a, ign := r.hunkID("s", "a.txt", "A"), r.hunkID("s", "ign.txt", "secret")
	r.expectCode(1, "s", "add", "a.txt", "ign.txt", "--hunk", a, "--hunk", ign)
	if got := r.stagedPaths("s"); len(got) != 0 {
		t.Fatalf("staged %v after a refusal", got)
	}
}

// TestRestoreHunksOfSeveralFiles unstages hunks of two files in one call:
// all of one file and one of two hunks of the other.
func TestRestoreHunksOfSeveralFiles(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.write("b.txt", "1\n2\n3\n4\n5\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "ab")
	r.write("a.txt", "a\nA\n")
	r.write("b.txt", "ONE\n2\n3\n4\nFIVE\n")
	r.mustMS("s", "add", "a.txt", "b.txt")
	var fj filesJSON
	decode(t, r.mustMS("s", "hunks", "--cached", "--json", "a.txt", "b.txt"), &fj)
	var ids []string
	for _, f := range fj.Files {
		for _, h := range f.Hunks {
			if f.Path == "a.txt" || slices.Contains(h.Lines, "+FIVE") {
				ids = append(ids, "--hunk", h.ID)
			}
		}
	}
	r.mustMS("s", append([]string{"restore", "--staged", "a.txt", "b.txt"}, ids...)...)
	if got := r.stagedPaths("s"); !eq(got, []string{"b.txt"}) {
		t.Fatalf("staged %v", got)
	}
	r.mustMS("s", "commit", "-m", "one")
	if got := r.git("show", "main:b.txt"); got != "ONE\n2\n3\n4\n5\n" {
		t.Fatalf("committed %q", got)
	}
}

// TestShiftedCopyIDIsStale lists three identical hunks; another stage then
// commits a deletion that moves the second copy to where the first was.
// The first copy's ID must be unknown now, not name the second copy.
func TestShiftedCopyIDIsStale(t *testing.T) {
	r := newRepo(t)
	base := "p1\np2\nx\np4\np5\nx\np7\np8\nx\np10\n"
	r.write("f", base)
	r.git("add", "f")
	r.git("commit", "-q", "-m", "base")
	r.write("f", strings.ReplaceAll(base, "x\n", "y\n"))
	var fj filesJSON
	decode(t, r.mustMS("a", "hunks", "--json", "f"), &fj)
	if len(fj.Files) != 1 || len(fj.Files[0].Hunks) != 3 {
		t.Fatalf("want 3 hunks:\n%+v", fj)
	}
	first := fj.Files[0].Hunks[0].ID
	patch := "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,4 +1 @@\n-p1\n-p2\n-x\n p4\n"
	if res := runStdin(r.dir, "b", nil, patch, "apply", "-"); res.code != 0 {
		t.Fatalf("apply:\n%s", res)
	}
	r.mustMS("b", "commit", "-m", "delete 1-3")
	r.write("f", "p4\np5\ny\np7\np8\ny\np10\n")
	r.expectCode(4, "a", "add", "f", "--hunk", first)
}

// TestAddEditsAfterACrashSkipsAppliedRecords records two edits of one line.
// `add --edits` dies after it adopted the index and before it deleted the
// records; the next `add --edits` must not apply them again.
func TestAddEditsAfterACrashSkipsAppliedRecords(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "a\nb\nc\n")
	r.write("g.txt", "g\n")
	r.git("add", ".")
	r.git("commit", "-q", "-m", "fg")
	r.hookedEdit("s", "", "f.txt", "a\nB\nc\n")
	r.hookedEdit("s", "", "f.txt", "a\nBB\nc\n")
	res := run(r.dir, "", []string{"GIT_MULTISTAGE_FAULT=after-adopt-edits"}, "--stage", "claude-s", "add", "--edits")
	if res.code != 86 {
		t.Fatalf("want the fault exit 86:\n%s", res)
	}
	r.hookedEdit("s", "", "g.txt", "g\nG\n")
	r.mustMS("", "--stage", "claude-s", "add", "--edits")
	r.mustMS("", "--stage", "claude-s", "commit", "-m", "fg")
	if got := r.git("show", "main:f.txt") + r.git("show", "main:g.txt"); got != "a\nBB\nc\ng\nG\n" {
		t.Fatalf("committed %q", got)
	}
}

// TestOrphanedIndexLockIsKept leaves the lock file of the next index
// generation, as a Git child of a killed git-multistage holds it. The next
// command must neither delete it nor reuse its generation, so the orphan
// cannot later rename a lock file of the new command.
func TestOrphanedIndexLockIsKept(t *testing.T) {
	r := newRepo(t)
	r.mustMS("a", "status")
	dir := r.path(".git/multistage/stages/a")
	var m struct {
		Gen int `json:"gen"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(dir, fmt.Sprintf("index-%d.lock", m.Gen+1))
	if err := os.WriteFile(orphan, []byte("half-written"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.write("f.txt", "f\n")
	r.mustMS("a", "add", "f.txt")
	r.mustMS("a", "status")
	if got, err := os.ReadFile(orphan); err != nil || string(got) != "half-written" {
		t.Fatalf("the orphaned lock file changed: %q, %v", got, err)
	}
	r.mustMS("a", "commit", "-m", "f")
	if got := r.git("show", "main:f.txt"); got != "f\n" {
		t.Fatalf("committed %q", got)
	}
}

// TestWholeFileAddOfASharedFileIsRefused stages a whole file that another
// stage has staged changes in: exit 5 with kind shared-path, and nothing
// staged, unless --allow-shared is passed.
func TestWholeFileAddOfASharedFileIsRefused(t *testing.T) {
	r := newRepo(t)
	r.write("f.txt", "1\n2\n3\n4\n5\n")
	r.git("add", "f.txt")
	r.git("commit", "-q", "-m", "f")
	r.write("f.txt", "ONE\n2\n3\n4\n5\n")
	r.mustMS("a", "add", "f.txt")
	r.write("f.txt", "ONE\n2\n3\n4\nFIVE\n")
	res := r.expectCode(5, "b", "--json", "add", "f.txt")
	if !strings.Contains(res.out, `"shared-path"`) {
		t.Fatalf("want kind shared-path:\n%s", res)
	}
	if got := r.stagedPaths("b"); len(got) != 0 {
		t.Fatalf("staged %v after a refusal", got)
	}
	r.mustMS("b", "add", "f.txt", "--allow-shared")
	if got := r.stagedPaths("b"); !eq(got, []string{"f.txt"}) {
		t.Fatalf("staged %v", got)
	}
}

// TestBusyStageExits6 holds a stage's lock in this process; a
// git-multistage process of the same stage gives up with exit 6.
func TestBusyStageExits6(t *testing.T) {
	r := newRepo(t)
	r.mustMS("a", "status")
	lk, err := filelock.Acquire(r.path(".git/multistage/stages/a/lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	res := run(r.dir, "a", []string{"GIT_MULTISTAGE_TEST_LOCK_TIMEOUT_MS=200"}, "--json", "status")
	if res.code != 6 || !strings.Contains(res.out, `"busy"`) {
		t.Fatalf("want exit 6, kind busy:\n%s", res)
	}
	lk.Release()
	r.mustMS("a", "status")
}
