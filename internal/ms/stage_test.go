package ms

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
