package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// refRepo creates a repository in the given object format with two commits
// on main, a branch, and an annotated tag, and returns a function that runs
// git in a directory.
func refRepo(t *testing.T, format string) (dir string, git func(dir string, args ...string) (string, error)) {
	t.Helper()
	dir = initRepo(t, format)
	git = func(d string, args ...string) (string, error) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", d}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	for _, args := range [][]string{
		{"symbolic-ref", "HEAD", "refs/heads/main"},
		{"commit", "-q", "--allow-empty", "-m", "one"},
		{"branch", "feature"},
		{"tag", "-a", "-m", "tag", "v1"},
		{"commit", "-q", "--allow-empty", "-m", "two"},
	} {
		if out, err := git(dir, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir, git
}

// gitRef returns what Git reads for ref: the commit of `rev-parse --verify`,
// "" when it fails, and for HEAD the branch of `symbolic-ref`, or "HEAD" when
// HEAD is detached.
func gitRef(t *testing.T, git func(string, ...string) (string, error), dir, ref string) (name, oid string) {
	t.Helper()
	oid, _ = git(dir, "rev-parse", "-q", "--verify", ref+"^{commit}")
	name = ref
	if ref == "HEAD" {
		if b, err := git(dir, "symbolic-ref", "-q", "HEAD"); err == nil {
			name = b
		}
	}
	return name, oid
}

// TestReadRefFilesMatchesGit compares the reader with Git for loose, packed,
// peeled, symbolic, chained, detached, and unborn refs, a branch name that is
// a directory or lies below a file, and a linked worktree, in SHA-1 and
// SHA-256 repositories.
func TestReadRefFilesMatchesGit(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			dir, git := refRepo(t, format)
			must := func(d string, args ...string) {
				t.Helper()
				if out, err := git(d, args...); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
			}
			r, err := Open(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			check := func(r *Repo, d, step string, refs ...string) {
				t.Helper()
				for _, ref := range refs {
					wantName, wantOID := gitRef(t, git, d, ref)
					name, oid, ok := r.readRefFiles(ref)
					if !ok || oid != wantOID || (ref == "HEAD" && name != wantName) {
						t.Fatalf("%s: readRefFiles(%s) = %q, %q, %v; Git reads %q, %q", step, ref, name, oid, ok, wantName, wantOID)
					}
				}
			}
			all := []string{"HEAD", "refs/heads/main", "refs/heads/feature", "refs/heads/gone", "refs/heads/dir", "refs/heads/main/x"}
			must(dir, "branch", "dir/x")
			check(r, dir, "loose", all...)
			must(dir, "pack-refs", "--all")
			if data, _ := os.ReadFile(filepath.Join(dir, ".git", "packed-refs")); !strings.Contains(string(data), "\n^") {
				t.Fatalf("packed-refs has no peeled line:\n%s", data)
			}
			check(r, dir, "packed", all...)
			must(dir, "commit", "-q", "--allow-empty", "-m", "three")
			check(r, dir, "loose over packed", all...)
			must(dir, "symbolic-ref", "refs/heads/alias", "refs/heads/feature")
			must(dir, "symbolic-ref", "HEAD", "refs/heads/alias")
			check(r, dir, "chained symbolic refs", "HEAD", "refs/heads/alias")
			must(dir, "symbolic-ref", "HEAD", "refs/heads/unborn")
			check(r, dir, "unborn", "HEAD", "refs/heads/unborn")
			must(dir, "checkout", "-q", "--detach", "main")
			check(r, dir, "detached", "HEAD")

			wt := filepath.Join(t.TempDir(), "wt")
			must(dir, "worktree", "add", "-q", "-b", "wtb", wt, "feature")
			w, err := Open(wt, "")
			if err != nil {
				t.Fatal(err)
			}
			check(w, wt, "linked worktree", "HEAD", "refs/heads/wtb", "refs/heads/main")
		})
	}
}

// TestReadRefFilesLeavesOddRefsToGit checks the cases that the reader hands
// to Git, and that ResolveRef and TipAndHead still give Git's answer there.
func TestReadRefFilesLeavesOddRefsToGit(t *testing.T) {
	t.Parallel()
	dir, git := refRepo(t, "sha1")
	r, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(dir, ".git")
	write := func(rel, data string) {
		t.Helper()
		p := filepath.Join(gitDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	main, _ := git(dir, "rev-parse", "main")
	write("refs/heads/garbage", "not an id\n")
	write("refs/heads/zero", strings.Repeat("0", 40)+"\n")
	write("refs/heads/upper", strings.ToUpper(main)+"\n")
	write("refs/heads/remote", "ref: refs/remotes/origin/main\n")
	for i := range 6 {
		write("refs/heads/loop"+string(rune('0'+i)), "ref: refs/heads/loop"+string(rune('0'+i+1))+"\n")
	}
	write("refs/heads/loop6", main+"\n")
	for _, ref := range []string{
		"refs/heads/garbage", "refs/heads/zero", "refs/heads/upper", "refs/heads/remote",
		"refs/heads/loop0", "refs/tags/v1", "refs/heads/a..b", "refs/heads/x.lock", "refs/heads/.x", "main",
	} {
		if name, oid, ok := r.readRefFiles(ref); ok {
			t.Fatalf("readRefFiles(%s) = %q, %q, true; want Git to read it", ref, name, oid)
		}
	}
	if oid, err := r.ResolveRef("refs/heads/upper"); err != nil || oid != main {
		t.Fatalf("ResolveRef of an upper-case ID = %q, %v; want %s", oid, err, main)
	}
}

// TestReadRefFilesMalformedPackedRefs hands a packed-refs file with an
// unknown line to Git.
func TestReadRefFilesMalformedPackedRefs(t *testing.T) {
	t.Parallel()
	dir, git := refRepo(t, "sha1")
	r, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := git(dir, "pack-refs", "--all"); err != nil {
		t.Fatal(out, err)
	}
	p := filepath.Join(dir, ".git", "packed-refs")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(data, "junk\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	if name, oid, ok := r.readRefFiles("refs/heads/gone"); ok {
		t.Fatalf("readRefFiles with a malformed packed-refs = %q, %q, true", name, oid)
	}
}

// TestReadRefFilesSymlinkHead hands a HEAD that is a symbolic link, as
// core.preferSymlinkRefs writes it, to Git.
func TestReadRefFilesSymlinkHead(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	dir, git := refRepo(t, "sha1")
	r, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	head := filepath.Join(dir, ".git", "HEAD")
	if err := os.Remove(head); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("refs/heads/feature", head); err != nil {
		t.Fatal(err)
	}
	if name, oid, ok := r.readRefFiles("HEAD"); ok {
		t.Fatalf("readRefFiles of a symbolic-link HEAD = %q, %q, true", name, oid)
	}
	wantName, wantOID := gitRef(t, git, dir, "HEAD")
	tip, head2, err := r.TipAndHead("refs/heads/feature")
	if err != nil || head2 != wantName || tip != wantOID {
		t.Fatalf("TipAndHead with a symbolic-link HEAD = %q, %q, %v; Git reads %q, %q", tip, head2, err, wantOID, wantName)
	}
}

// TestReadRefFilesReftable hands every ref of the reftable storage to Git.
func TestReadRefFilesReftable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.CommandContext(context.Background(), "git", "init", "-q", "--ref-format=reftable", dir).CombinedOutput(); err != nil {
		t.Skipf("git init --ref-format=reftable: %v\n%s", err, out)
	}
	r, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if name, oid, ok := r.readRefFiles("HEAD"); ok {
		t.Fatalf("readRefFiles in a reftable repository = %q, %q, true", name, oid)
	}
	if head, err := r.SymbolicHead(); err != nil || head != "refs/heads/master" && head != "refs/heads/main" {
		t.Fatalf("SymbolicHead in a reftable repository = %q, %v", head, err)
	}
}

func TestValidRefName(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"refs/heads/main": true, "refs/heads/feat/x-1_2": true, "refs/heads/a@b": true,
		"": false, "@": false, "refs/heads/": false, "refs//heads": false, "refs/heads/a.": false,
		"refs/heads/a..b": false, "refs/heads/.a": false, "refs/heads/a.lock": false,
		"refs/heads/a@{1}": false, "refs/heads/a b": false, "refs/heads/a~1": false, "refs/heads/a^": false,
		"refs/heads/a:b": false, "refs/heads/a?": false, "refs/heads/a*": false, "refs/heads/a[": false,
		`refs/heads/a\b`: false, "refs/heads/a\x01": false, "refs/heads/a\x7f": false,
	} {
		if got := validRefName(name); got != want {
			t.Errorf("validRefName(%q) = %v, want %v", name, got, want)
		}
	}
}
