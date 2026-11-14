package ms

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIndexV2(t *testing.T) {
	t.Parallel()
	r := batchRepo(t)
	dir := t.TempDir()
	v2 := filepath.Join(dir, "v2")
	if err := copyFile(r.IndexPath, v2); err != nil {
		t.Fatal(err)
	}
	git(t, r.Top, nil, "update-index", "--skip-worktree", "a")
	v3 := filepath.Join(dir, "v3")
	if err := copyFile(r.IndexPath, v3); err != nil {
		t.Fatal(err)
	}
	short := filepath.Join(dir, "short")
	if err := os.WriteFile(short, []byte("DIRC"), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{v2: true, v3: false, short: false, filepath.Join(dir, "missing"): false} {
		if got := indexV2(path); got != want {
			t.Errorf("indexV2(%s) = %v, want %v", filepath.Base(path), got, want)
		}
	}
}

// openStage opens stage id and closes it.
func openStage(t *testing.T, r *Repo, id string) *Stage {
	t.Helper()
	l, err := r.OpenStage(id, false)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
	return l
}

func TestNewestStageIndexIsOfTheStageThatChangedLast(t *testing.T) {
	t.Parallel()
	r := batchRepo(t)
	a, b := openStage(t, r, "a"), openStage(t, r, "b")
	if err := os.MkdirAll(filepath.Join(r.stagesDir(), ".dropped-x"), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for dir, at := range map[string]time.Time{
		a.Dir: now.Add(-time.Hour), b.Dir: now.Add(-time.Minute), filepath.Join(r.stagesDir(), ".dropped-x"): now,
	} {
		if err := os.Chtimes(dir, at, at); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.newestStageIndex("c"); got != b.IndexPath() {
		t.Errorf("newestStageIndex(c) = %s, want the index of b", got)
	}
	if got := r.newestStageIndex("b"); got != a.IndexPath() {
		t.Errorf("newestStageIndex(b) = %s, want the index of a", got)
	}
}

func TestNewStageWithoutTheSeedFileReadsTheTree(t *testing.T) {
	t.Parallel()
	r := batchRepo(t)
	a := openStage(t, r, "a")
	if err := os.Remove(a.IndexPath()); err != nil {
		t.Fatal(err)
	}
	b := openStage(t, r, "b")
	want, err := r.TreeOf("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got := git(t, r.Top, []string{"GIT_INDEX_FILE=" + b.IndexPath()}, "write-tree"); got != want {
		t.Fatalf("the new stage index holds %s, want %s", got, want)
	}
}
