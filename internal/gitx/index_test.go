package gitx

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// indexRepo creates a repository in the given object format whose index
// holds files in a directory, a path longer than 4095 bytes, and the three
// stages of a conflicted path. It returns a function that runs git in it,
// with stdin.
func indexRepo(t *testing.T, format string) (dir string, git func(stdin string, args ...string) string) {
	t.Helper()
	dir = initRepo(t, format)
	git = func(stdin string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
		cmd.Stdin = strings.NewReader(stdin)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	for _, f := range []string{"a", "d/x", "d/y", "e/z"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("", "add", ".")
	blob := strings.TrimSpace(git("x\n", "hash-object", "-w", "--stdin"))
	long := strings.Repeat(strings.Repeat("s", 200)+"/", 21) + "f"
	git("", "update-index", "--add", "--cacheinfo", "100755,"+blob+","+long)
	git(fmt.Sprintf("100644 %s 1\tc\n100644 %s 2\tc\n100644 %s 3\tc\n", blob, blob, blob), "update-index", "--index-info")
	return dir, git
}

// readAll formats every entry of the index file as `ls-files -s -z` prints
// it.
func readAll(t *testing.T, r *Repo, index string) string {
	t.Helper()
	entries, ok := r.ReadIndex(index, func([]byte) bool { return true })
	if !ok {
		t.Fatalf("ReadIndex(%s) handed the file to Git", index)
	}
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s %s %d\t%s\x00", e.Mode, e.OID, e.Stage, e.Path)
	}
	return b.String()
}

func indexVersion(t *testing.T, index string) uint32 {
	t.Helper()
	data, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	return binary.BigEndian.Uint32(data[4:8])
}

// TestReadIndexMatchesLsFiles compares the reader with `ls-files -s -z` for
// index versions 2, 3, and 4, in SHA-1 and SHA-256 repositories, with
// unmerged, intent-to-add, and skip-worktree entries and a long path.
func TestReadIndexMatchesLsFiles(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			dir, git := indexRepo(t, format)
			r, err := Open(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			check := func(want uint32) {
				t.Helper()
				if v := indexVersion(t, r.IndexPath); v != want {
					t.Fatalf("index version %d, want %d", v, want)
				}
				if got, ls := readAll(t, r, r.IndexPath), git("", "ls-files", "-s", "-z"); got != ls {
					t.Fatalf("version %d: ReadIndex gives\n%q\nls-files -s -z prints\n%q", want, got, ls)
				}
			}
			git("", "update-index", "--index-version", "2")
			check(2)
			git("", "update-index", "--index-version", "4")
			check(4)
			git("", "update-index", "--index-version", "2")
			if err := os.WriteFile(filepath.Join(dir, "n"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			git("", "add", "-N", "n")
			git("", "update-index", "--skip-worktree", "a")
			check(3)
			git("", "update-index", "--index-version", "4")
			check(4)
		})
	}
}

// TestReadIndexMatchesPaths checks that only the entries that match are
// returned, and that a missing index has no entries.
func TestReadIndexMatchesPaths(t *testing.T) {
	t.Parallel()
	dir, _ := indexRepo(t, "sha1")
	r, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := r.ReadIndex(r.IndexPath, func(name []byte) bool { return bytes.HasPrefix(name, []byte("d/")) })
	if !ok || len(entries) != 2 || entries[0].Path != "d/x" || entries[1].Path != "d/y" {
		t.Fatalf("ReadIndex of d/ = %v, %v", entries, ok)
	}
	if entries, ok := r.ReadIndex(filepath.Join(dir, "missing"), func([]byte) bool { return true }); !ok || entries != nil {
		t.Fatalf("ReadIndex of a missing file = %v, %v; want no entries", entries, ok)
	}
}

// TestReadIndexLeavesOddIndexesToGit checks that a split index, a sparse
// index, a required extension that the reader does not know, another
// version, and a file that ends early go to Git, and that an optional
// extension does not.
func TestReadIndexLeavesOddIndexesToGit(t *testing.T) {
	t.Parallel()
	dir, git := indexRepo(t, "sha1")
	r, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	all := func([]byte) bool { return true }
	plain, err := os.ReadFile(r.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	body := plain[:len(plain)-20]
	variant := func(name string, data []byte) string {
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	join := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	sum := make([]byte, 20)
	ext := func(sig string) []byte { return append([]byte(sig), 0, 0, 0, 0) }
	if _, ok := r.ReadIndex(variant("optional", join(body, ext("ABCD"), sum)), all); !ok {
		t.Fatal("an optional extension handed the file to Git")
	}
	version5 := join(plain)
	version5[7] = 5
	for name, data := range map[string][]byte{
		"required":  join(body, ext("abcd"), sum),
		"version 5": version5,
		"truncated": plain[:len(plain)/2],
		"empty":     nil,
	} {
		if entries, ok := r.ReadIndex(variant("index", data), all); ok {
			t.Fatalf("%s: ReadIndex = %v, true; want Git to read it", name, entries)
		}
	}

	git("", "-c", "core.splitIndex=true", "update-index", "--split-index")
	if entries, ok := r.ReadIndex(r.IndexPath, all); ok {
		t.Fatalf("split index: ReadIndex = %v, true; want Git to read it", entries)
	}
	git("", "update-index", "--no-split-index")
	git("", "update-index", "--force-remove", "c")
	git("", "commit", "-q", "-m", "files", "--no-verify")
	git("", "sparse-checkout", "set", "--cone", "--sparse-index", "d")
	if !strings.Contains(git("", "ls-files", "-s", "--sparse"), "040000") {
		t.Fatal("the index has no sparse directory entry")
	}
	if entries, ok := r.ReadIndex(r.IndexPath, all); ok {
		t.Fatalf("sparse index: ReadIndex = %v, true; want Git to read it", entries)
	}
}

// TestVarint checks the offset varint of Git's varint.c, and that a number
// that does not fit in an int is malformed.
func TestVarint(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   []byte
		v, n int
	}{
		{[]byte{0x00}, 0, 1},
		{[]byte{0x7f, 0xff}, 127, 1},
		{[]byte{0x80, 0x00}, 128, 2},
		{[]byte{0xff, 0x7f}, 16511, 2},
		{[]byte{0x80}, 0, 0},
		{[]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00}, 0, 0},
	} {
		if v, n := varint(tc.in); v != tc.v || n != tc.n {
			t.Errorf("varint(% x) = %d, %d; want %d, %d", tc.in, v, n, tc.v, tc.n)
		}
	}
}

// TestReadIndexHandsAnOverflowToGit reads a version 4 index whose first
// entry removes more bytes than an int holds; Git reads it, so the reader
// hands it to Git instead of failing.
func TestReadIndexHandsAnOverflowToGit(t *testing.T) {
	t.Parallel()
	r, err := Open(initRepo(t, "sha1"), "")
	if err != nil {
		t.Fatal(err)
	}
	entry := make([]byte, indexEntryFixed+20+2)
	binary.BigEndian.PutUint32(entry[24:], 0o100644)
	binary.BigEndian.PutUint16(entry[indexEntryFixed+20:], 1)
	data := bytes.Join([][]byte{
		[]byte("DIRC\x00\x00\x00\x04\x00\x00\x00\x01"), entry,
		{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00},
		[]byte("a\x00"), make([]byte, 20),
	}, nil)
	p := filepath.Join(t.TempDir(), "index")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if entries, ok := r.ReadIndex(p, func([]byte) bool { return true }); ok {
		t.Fatalf("ReadIndex = %v, true; want Git to read it", entries)
	}
}
