package gitx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// body returns a file content that Git's rename detection tells apart from
// the content of another seed.
func body(seed string) string {
	var b strings.Builder
	for i := range 12 {
		fmt.Fprintf(&b, "%s line %d\n", seed, i)
	}
	return b.String()
}

// diffRepo makes a repository with commits that hold renames, a move into a
// directory, deletions, additions, mode changes, and a file that replaces a
// directory. It returns the repository and each pair of commits to diff.
func diffRepo(t *testing.T, format string) (dir string, pairs [][2]string) {
	t.Helper()
	dir = initRepo(t, format)
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(full, mode); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(msg string) string {
		t.Helper()
		git("add", "-A")
		git("commit", "-q", "--allow-empty", "-m", msg)
		return git("rev-parse", "HEAD")
	}
	write("a.txt", body("a"), 0o644)
	write("b.txt", body("b"), 0o644)
	write("c.txt", body("c"), 0o644)
	write("run.sh", body("run"), 0o644)
	write("edit.txt", body("edit"), 0o644)
	write("same.txt", body("same"), 0o644)
	write("x/y", body("xy"), 0o644)
	write("a-b", body("ab"), 0o644)
	write("a/b", body("slash"), 0o644)
	c0 := commit("c0")

	// Renames, a move into a directory, a rename with an edit, a deletion,
	// an addition, a mode change, and names that sort around "a/".
	if err := os.Rename(filepath.Join(dir, "a.txt"), filepath.Join(dir, "z.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "d2", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "b.txt"), filepath.Join(dir, "d2", "deep", "b.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "c.txt")); err != nil {
		t.Fatal(err)
	}
	write("new.txt", body("new"), 0o644)
	write("run.sh", body("run"), 0o755)
	write("edit.txt", body("edit")+"added\n", 0o644)
	if err := os.Rename(filepath.Join(dir, "edit.txt"), filepath.Join(dir, "a.b")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "a-b"), filepath.Join(dir, "a0")); err != nil {
		t.Fatal(err)
	}
	c1 := commit("c1")

	// A rename that changes the mode, and a file that replaces a directory.
	write("same.txt", body("same"), 0o755)
	if err := os.Rename(filepath.Join(dir, "same.txt"), filepath.Join(dir, "other.sh")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "x")); err != nil {
		t.Fatal(err)
	}
	write("x", body("xy"), 0o644)
	c2 := commit("c2")

	// A commit that changes nothing, and the reverse of the first pair.
	c3 := commit("c3")
	return dir, [][2]string{{c0, c1}, {c1, c2}, {c2, c3}, {c1, c0}, {c0, c2}, {c0, c0}}
}

// TestDiffTreesDerivesTheDiffWithoutRenames compares the diff without renames
// that DiffTrees derives from a remembered diff with renames, with the diff
// that `git diff-tree --no-renames` prints.
func TestDiffTreesDerivesTheDiffWithoutRenames(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			dir, pairs := diffRepo(t, format)
			r, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			renamed := 0
			for _, p := range pairs {
				withRenames, err := r.DiffTrees(p[0], p[1], true)
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range withRenames {
					if e.Status == 'R' {
						renamed++
					}
				}
				// The derived diff starts no `git`.
				git := r.git
				r.git = filepath.Join(dir, "no-such-git")
				derived, err := r.DiffTrees(p[0], p[1], false)
				r.git = git
				if err != nil {
					t.Fatalf("%v: %v", p, err)
				}
				// `fresh` has nothing remembered, so it runs `git diff-tree`.
				want, err := fresh.diffTrees(p[0], p[1], false)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(derived, want) {
					t.Errorf("diff %v without renames:\nderived %+v\ngit     %+v", p, derived, want)
				}
			}
			if renamed < 4 {
				t.Fatalf("the pairs hold %d renames, want at least 4", renamed)
			}
		})
	}
}

func TestDiffTreesRemembersEachResult(t *testing.T) {
	t.Parallel()
	dir, pairs := diffRepo(t, "sha1")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, b := pairs[0][0], pairs[0][1]
	first, err := r.DiffTrees(a, b, true)
	if err != nil {
		t.Fatal(err)
	}
	r.git = filepath.Join(dir, "no-such-git")
	// A commit and its tree name the same diff.
	treeA, treeB := r.KnownTree(a), r.KnownTree(b)
	if treeA != "" || treeB != "" {
		t.Fatalf("the trees of %s and %s are known without a read", a, b)
	}
	second, err := r.DiffTrees(a, b, true)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("second read: %+v, %v; first %+v", second, err, first)
	}
	// A caller that changes its result leaves the remembered one alone.
	second[0].Path = "changed"
	if third, _ := r.DiffTrees(a, b, true); !reflect.DeepEqual(third, first) {
		t.Fatalf("a changed result reached the memo: %+v", third)
	}
	// A ref can move, so its diff is not remembered.
	if _, err := r.DiffTrees("HEAD", b, true); err == nil {
		t.Fatal("a diff of HEAD came from the memo, not from git")
	}
	if _, err := r.DiffTrees(a, b, false); err != nil {
		t.Fatalf("the diff without renames needs the one with renames: %v", err)
	}
	// A failed read is not remembered.
	if _, err := r.DiffTrees(strings.Repeat("1", len(r.EmptyTree)), b, true); err == nil {
		t.Fatal("a diff of a missing object succeeded")
	}
}

func TestDiffTreesKeysCommitsByTheirKnownTrees(t *testing.T) {
	t.Parallel()
	dir, pairs := diffRepo(t, "sha1")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, b := pairs[0][0], pairs[0][1]
	treeA, err := r.TreeOf(a)
	if err != nil {
		t.Fatal(err)
	}
	treeB, err := r.TreeOf(b)
	if err != nil {
		t.Fatal(err)
	}
	first, err := r.DiffTrees(treeA, treeB, true)
	if err != nil {
		t.Fatal(err)
	}
	r.git = filepath.Join(dir, "no-such-git")
	byCommit, err := r.DiffTrees(a, b, true)
	if err != nil || !reflect.DeepEqual(first, byCommit) {
		t.Fatalf("commits of known trees: %+v, %v; trees %+v", byCommit, err, first)
	}
}
