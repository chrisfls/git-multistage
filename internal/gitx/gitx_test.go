package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLostObjectWrite(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{
			name: "rename onto a concurrent object, as Windows CI reported it",
			stderr: "error: unable to write file .git/objects/e6/9de29bb2d1d6434b8b29ae775ad8c2e48c5391: Permission denied\n" +
				"fatal: unable to add (NULL) to database\n",
			want: true,
		},
		{
			name:   "absolute path with backslashes",
			stderr: `error: unable to write file D:\a\repo\.git\objects\e6\9de29bb2d1d6434b8b29ae775ad8c2e48c5391: Permission denied`,
			want:   true,
		},
		{
			name:   "sha256 object in a worktree's common directory",
			stderr: "error: unable to write file /r/.git/objects/47/3a0f4c3be8a93681a267e3b1e9a7dcda1185436fe141f7749120a303721813: Permission denied",
			want:   true,
		},
		{
			name:   "disk full is a real failure",
			stderr: "error: unable to write file .git/objects/e6/9de29bb2d1d6434b8b29ae775ad8c2e48c5391: No space left on device",
		},
		{
			name:   "unwritable object directory is a real failure",
			stderr: "error: insufficient permission for adding an object to repository database .git/objects",
		},
		{
			name:   "a file outside the object database",
			stderr: "error: unable to write file .git/index.lock: Permission denied",
		},
		{name: "no error output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lostObjectWrite(tt.stderr); got != tt.want {
				t.Fatalf("lostObjectWrite(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}

func initRepo(t *testing.T, format string) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.CommandContext(context.Background(), "git", "init", "-q", "--object-format="+format, dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestOpenKnowsTheEmptyObjects(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			dir := initRepo(t, format)
			top, err := Open(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			sub, err := Open(filepath.Join(dir, "sub"), "")
			if err != nil {
				t.Fatal(err)
			}
			if top.Prefix != "" || sub.Prefix != "sub/" || top.ObjectFormat != format {
				t.Fatalf("prefix %q and %q, format %q", top.Prefix, sub.Prefix, top.ObjectFormat)
			}
			tree, err := top.Line(Opt{Stdin: strings.NewReader("")}, "hash-object", "-t", "tree", "--stdin")
			if err != nil {
				t.Fatal(err)
			}
			if tree != top.EmptyTree || top.ZeroOID != strings.Repeat("0", len(tree)) {
				t.Fatalf("empty tree %s, want %s; zero %s", top.EmptyTree, tree, top.ZeroOID)
			}
			blob, err := top.EnsureEmptyBlob()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := top.Run(Opt{}, "cat-file", "-e", blob); err != nil {
				t.Fatalf("the empty blob is not in the object database: %v", err)
			}
		})
	}
}

// TestTipAndHead compares the joint read with the separate reads of HEAD
// and the tip.
func TestTipAndHead(t *testing.T) {
	t.Parallel()
	dir := initRepo(t, "sha1")
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i")
	tip := git("rev-parse", "HEAD")
	r, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	headFile := filepath.Join(dir, "HEAD")
	for _, tc := range []struct {
		name, ref, tip, head string
		setup                func()
	}{
		{"branch", "refs/heads/main", tip, "refs/heads/main", func() { git("symbolic-ref", "HEAD", "refs/heads/main") }},
		{"missing branch", "refs/heads/gone", "", "refs/heads/main", func() {}},
		{"unborn branch", "refs/heads/main", tip, "refs/heads/unborn", func() { git("symbolic-ref", "HEAD", "refs/heads/unborn") }},
		{"unborn branch beside a file named HEAD", "refs/heads/main", tip, "refs/heads/unborn", func() {
			if err := os.WriteFile(headFile, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"symbolic ref to a branch", "refs/heads/main", tip, "refs/heads/main", func() {
			if err := os.Remove(headFile); err != nil {
				t.Fatal(err)
			}
			git("symbolic-ref", "refs/heads/alias", "refs/heads/main")
			git("symbolic-ref", "HEAD", "refs/heads/alias")
		}},
		{"detached", "refs/heads/main", tip, "", func() { git("update-ref", "--no-deref", "HEAD", tip) }},
		{"detached stage", "HEAD", tip, "", func() {}},
	} {
		tc.setup()
		gotTip, gotHead, err := r.TipAndHead(tc.ref)
		if err != nil || gotTip != tc.tip || gotHead != tc.head {
			t.Fatalf("%s: TipAndHead(%s) = %q, %q, %v; want %q, %q", tc.name, tc.ref, gotTip, gotHead, err, tc.tip, tc.head)
		}
		sepHead, err1 := r.SymbolicHead()
		sepTip, err2 := r.ResolveRef(tc.ref)
		if err1 != nil || err2 != nil || sepTip != gotTip || sepHead != gotHead {
			t.Fatalf("%s: separate reads give %q, %q (%v, %v); the joint read %q, %q", tc.name, sepTip, sepHead, err1, err2, gotTip, gotHead)
		}
	}
}

func TestOpenReadsTheRefFormat(t *testing.T) {
	t.Parallel()
	r, err := Open(initRepo(t, "sha1"), "")
	if err != nil {
		t.Fatal(err)
	}
	if r.RefFormat != "files" {
		t.Fatalf("RefFormat = %q, want files", r.RefFormat)
	}
}
