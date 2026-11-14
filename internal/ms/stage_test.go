package ms

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// batchRepo returns a repository whose HEAD holds a, an empty file, and d/b.
func batchRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	for p, content := range map[string]string{"a": "one\ntwo\n", "empty": "", "d/b": "bee"} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init"},
	} {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	r, err := OpenRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBatchReadsMatchSingleReads(t *testing.T) {
	t.Parallel()
	r := batchRepo(t)
	// d is a directory and nope is missing: neither is a blob entry.
	paths := []string{"a", "empty", "d/b", "d", "nope"}
	idx, err := r.lsIndexMany(r.IndexPath, paths)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := r.TreeOf("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	trees, err := r.lsTreeMany(tree, paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		one, ok, err := r.lsIndex(r.IndexPath, p)
		if err != nil {
			t.Fatal(err)
		}
		if got, found := idx[p]; found != ok || got != one {
			t.Errorf("lsIndexMany[%q] = %v, %v; lsIndex = %v, %v", p, got, found, one, ok)
		}
		one, ok, err = r.lsTree(tree, p)
		if err != nil {
			t.Fatal(err)
		}
		if got, found := trees[p]; found != ok || got != one {
			t.Errorf("lsTreeMany[%q] = %v, %v; lsTree = %v, %v", p, got, found, one, ok)
		}
	}
	if len(idx) != 3 || len(trees) != 3 {
		t.Errorf("want 3 entries, got %d in the index and %d in the tree", len(idx), len(trees))
	}
}

func TestCatBlobsReadsManyBlobs(t *testing.T) {
	t.Parallel()
	r := batchRepo(t)
	idx, err := r.lsIndexMany(r.IndexPath, []string{"a", "empty", "d/b"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.catBlobs(idx["a"].oid, idx["empty"].oid, idx["d/b"].oid, idx["a"].oid)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"one\ntwo\n", "", "bee", "one\ntwo\n"}
	for i, w := range want {
		if string(got[i]) != w {
			t.Errorf("blob %d = %q, want %q", i, got[i], w)
		}
	}
	if _, err := r.catBlobs(idx["a"].oid, r.ZeroOID); err == nil {
		t.Error("catBlobs accepted a missing object")
	}
}

// stagedPathsRepo returns a repository with a stage s that has a staged
// change of a, and a function that opens the repository again, as a new
// process does.
func stagedPathsRepo(t *testing.T) (*Repo, func() *Repo) {
	t.Helper()
	r := batchRepo(t)
	if err := os.WriteFile(filepath.Join(r.Top, "a"), []byte("one\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := r.OpenStage("s", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.AddPaths([]string{"a"}, false, false); err != nil {
		t.Fatal(err)
	}
	l.Close()
	return r, func() *Repo {
		t.Helper()
		again, err := OpenRepo(r.Top)
		if err != nil {
			t.Fatal(err)
		}
		return again
	}
}

// readStaged reads the staged paths of stage s with a new Repo, as a new
// process does, and checks their number and the number of diff-index it
// started. It returns the paths.
func readStaged(t *testing.T, r *Repo, reopen func() *Repo, calls func() []string, wantPaths, wantDiffs int) []string {
	t.Helper()
	calls()
	paths, err := reopen().stageStagedPaths(filepath.Join(r.stagesDir(), "s"))
	if err != nil {
		t.Fatal(err)
	}
	if got := count(calls(), "diff-index"); got != wantDiffs || len(paths) != wantPaths {
		t.Fatalf("read %v with %d diff-index, want %d paths and %d diff-index", paths, got, wantPaths, wantDiffs)
	}
	return paths
}

func TestStagedPathsAreRemembered(t *testing.T) {
	calls := traceGitCalls(t)
	r, reopen := stagedPathsRepo(t)
	readStaged(t, r, reopen, calls, 1, 1)
	if paths := readStaged(t, r, reopen, calls, 1, 0); paths[0] != "a" {
		t.Fatalf("a second read of an unchanged stage: %v", paths)
	}

	// A changed index runs diff-index again, and the new paths replace the old.
	if err := os.WriteFile(filepath.Join(r.Top, "empty"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := r.OpenStage("s", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.AddPaths([]string{"empty"}, false, false); err != nil {
		t.Fatal(err)
	}
	l.Close()
	readStaged(t, r, reopen, calls, 2, 1)
	readStaged(t, r, reopen, calls, 2, 0)
}

func TestStagedPathsWithWrongCountAreAMiss(t *testing.T) {
	calls := traceGitCalls(t)
	r, reopen := stagedPathsRepo(t)
	readStaged(t, r, reopen, calls, 1, 1)
	file := r.stagedPathsFile("s")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	header, rest, _ := strings.Cut(string(data), "\n")
	f := strings.Fields(header)
	wrong := strings.Join([]string{f[0], f[1], "2"}, " ") + "\n" + rest
	if err := os.WriteFile(file, []byte(wrong), 0o644); err != nil {
		t.Fatal(err)
	}
	readStaged(t, r, reopen, calls, 1, 1)
	// The read rewrote the file, and a file cut short is a miss too.
	if err := os.WriteFile(file, []byte(header+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readStaged(t, r, reopen, calls, 1, 1)
	readStaged(t, r, reopen, calls, 1, 0)
}

func TestStagedPathsChangedBaseIsAMiss(t *testing.T) {
	calls := traceGitCalls(t)
	r, reopen := stagedPathsRepo(t)
	dir := filepath.Join(r.stagesDir(), "s")
	if _, err := reopen().stageStagedPaths(dir); err != nil {
		t.Fatal(err)
	}
	file := r.stagedPathsFile("s")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	f := strings.SplitN(string(data), " ", 3)
	f[1] = strings.Repeat("0", len(f[1]))
	if err := os.WriteFile(file, []byte(strings.Join(f, " ")), 0o644); err != nil {
		t.Fatal(err)
	}
	calls()
	if _, err := reopen().stageStagedPaths(dir); err != nil {
		t.Fatal(err)
	}
	if got := count(calls(), "diff-index"); got != 1 {
		t.Fatalf("a file of another base started %d diff-index, want 1", got)
	}
}

func TestStagedPathsWithoutChecksumAreNotRemembered(t *testing.T) {
	calls := traceGitCalls(t)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "index.skipHash")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	r := batchRepo(t)
	if err := os.WriteFile(filepath.Join(r.Top, "a"), []byte("one\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := r.OpenStage("s", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.AddPaths([]string{"a"}, false, false); err != nil {
		t.Fatal(err)
	}
	l.Close()
	if sum := r.IndexChecksum(filepath.Join(l.Dir, l.M.Index)); sum != "" {
		t.Fatalf("index.skipHash left an index checksum: %s", sum)
	}
	dir := filepath.Join(r.stagesDir(), "s")
	for range 2 {
		calls()
		if paths, err := r.stageStagedPaths(dir); err != nil || len(paths) != 1 {
			t.Fatalf("stageStagedPaths = %v, %v", paths, err)
		}
		if got := count(calls(), "diff-index"); got != 1 {
			t.Fatalf("a read without a checksum started %d diff-index, want 1", got)
		}
	}
	if _, err := os.Stat(r.stagedPathsFile("s")); err == nil {
		t.Fatal("a file was written for an index without a checksum")
	}
}

func TestDropDeletesTheStagedPathsFile(t *testing.T) {
	r, reopen := stagedPathsRepo(t)
	if _, err := reopen().stageStagedPaths(filepath.Join(r.stagesDir(), "s")); err != nil {
		t.Fatal(err)
	}
	file := r.stagedPathsFile("s")
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
	l, err := r.OpenStage("s", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Drop(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("the file of a dropped stage is still there: %v", err)
	}
	// A reader that writes after the drop removes its own file.
	writeStagedPaths(file, filepath.Join(r.stagesDir(), "s"), "ab", "cd", []string{"x"})
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("a write for a missing stage left its file: %v", err)
	}
}
