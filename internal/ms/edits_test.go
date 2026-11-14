package ms

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// splitIndexes makes Git write split indexes, which only ls-files reads, so
// that the reads of the test start Git.
func splitIndexes(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.splitIndex")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
}

func TestIndexReadsAnswerFromEarlierReads(t *testing.T) {
	calls := traceGitCalls(t)
	splitIndexes(t)
	r := batchRepo(t)
	reads := &indexReads{}
	check := func(idx string, paths []string, wantHeld, wantCalls int) {
		t.Helper()
		calls()
		held, err := reads.read(r, idx, paths)
		if err != nil {
			t.Fatal(err)
		}
		if got := count(calls(), "ls-files"); got != wantCalls || len(held) != wantHeld {
			t.Fatalf("read %v: %d entries and %d ls-files, want %d and %d", paths, len(held), got, wantHeld, wantCalls)
		}
	}
	check(r.IndexPath, []string{"a", "nope"}, 1, 1)
	check(r.IndexPath, []string{"nope", "a"}, 1, 0)

	// A copy has the checksum of the index, so it needs no read; a path that
	// no read covered is the only one that starts Git.
	cp := filepath.Join(t.TempDir(), "copy")
	if err := copyFile(r.IndexPath, cp); err != nil {
		t.Fatal(err)
	}
	check(cp, []string{"a", "nope"}, 1, 0)
	check(cp, []string{"a", "empty", "nope"}, 2, 1)
	check(r.IndexPath, []string{"empty", "d/b"}, 2, 1)
	check(cp, []string{"empty", "d/b"}, 2, 0)

	// Another content has another checksum.
	if err := os.WriteFile(filepath.Join(r.Top, "z"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := r.Run(gitx.Opt{Index: cp, KeepFormat: true}, "update-index", "--add", "z"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	check(cp, []string{"a", "z"}, 2, 1)
}

func TestIndexReadsWithoutChecksumAreNotRemembered(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "index.skipHash")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	r := batchRepo(t)
	if sum := r.IndexChecksum(r.IndexPath); sum != "" {
		t.Fatalf("index.skipHash left an index checksum: %s", sum)
	}
	reads := &indexReads{}
	for range 2 {
		held, err := reads.read(r, r.IndexPath, []string{"a"})
		if err != nil || len(held) != 1 {
			t.Fatalf("read = %v, %v", held, err)
		}
	}
	if len(reads.bySum) != 0 {
		t.Fatalf("reads of an index without a checksum were remembered: %v", reads.bySum)
	}
}

func TestIndexViewFlushesInGroups(t *testing.T) {
	calls := traceGitCalls(t)
	r := batchRepo(t)
	v, err := r.newIndexView(r.IndexPath, []EditRecord{{Path: "a"}}, &indexReads{})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := v.get("a")
	if !ok {
		t.Fatal("the view does not hold a")
	}
	v.remove("a")
	if _, ok := v.get("a"); ok || !v.removed("a") {
		t.Fatal("the view still holds a after its removal")
	}
	for i := range 249 {
		v.set(fmt.Sprintf("p%03d", i), "", a.oid)
	}
	v.set("a", "100755", a.oid) // a removed path comes back, in record order
	calls()
	before, _ := os.ReadFile(r.IndexPath)
	if err := v.flush(); err != nil {
		t.Fatal(err)
	}
	if got := count(calls(), "update-index"); got != 3 {
		t.Fatalf("250 writes started %d update-index, want 3", got)
	}
	after, _ := os.ReadFile(r.IndexPath)
	if string(before) == string(after) {
		t.Fatal("flush did not change the index file")
	}
	out, err := r.Run(gitx.Opt{Index: r.IndexPath}, "ls-files", "-s")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 3+249 || !strings.HasPrefix(lines[0], "100755 "+a.oid+" 0\ta") {
		t.Fatalf("%d entries, first %q", len(lines), lines[0])
	}
}

func TestIndexViewFlushFailureLeavesTheFile(t *testing.T) {
	r := batchRepo(t)
	v, err := r.newIndexView(r.IndexPath, []EditRecord{{Path: "a"}}, &indexReads{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := v.get("a")
	v.set("fresh", "", a.oid)
	v.set("d", "", a.oid) // the index holds d/b
	before, _ := os.ReadFile(r.IndexPath)
	if err := v.flush(); err == nil {
		t.Fatal("a file where the index holds a directory was written")
	}
	after, _ := os.ReadFile(r.IndexPath)
	if string(before) != string(after) {
		t.Fatal("a failed flush changed the index file")
	}
}
