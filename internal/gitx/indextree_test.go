//go:build !windows

package gitx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	treeA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	treeB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// writeIndex writes a file that has the shape of an index: a header, a body,
// and a hash of 20 bytes, each equal to sum.
func writeIndex(t *testing.T, path string, body string, sum byte) string {
	t.Helper()
	data := append([]byte("DIRC\x00\x00\x00\x02\x00\x00\x00\x00"), body...)
	data = append(data, bytes.Repeat([]byte{sum}, 20)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return strings.Repeat(string("0123456789abcdef"[sum>>4])+string("0123456789abcdef"[sum&15]), 20)
}

// sha1Repo makes the fake-git Repo see a SHA-1 repository.
func sha1Repo(r *Repo) *Repo {
	r.EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	return r
}

func TestIndexChecksum(t *testing.T) {
	r := sha1Repo(&Repo{})
	path := filepath.Join(t.TempDir(), "index")
	want := writeIndex(t, path, "entries", 0xab)
	if got := r.IndexChecksum(path); got != want {
		t.Fatalf("IndexChecksum = %q, want %q", got, want)
	}
	if got := r.IndexChecksum(path + ".missing"); got != "" {
		t.Fatalf("missing file: %q", got)
	}
	writeIndex(t, path, "entries", 0)
	if got := r.IndexChecksum(path); got != "" {
		t.Fatalf("zero hash (index.skipHash): %q", got)
	}
	if err := os.WriteFile(path, make([]byte, indexHeaderSize+19), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.IndexChecksum(path); got != "" {
		t.Fatalf("file shorter than a header and a hash: %q", got)
	}
	if got := (&Repo{}).IndexChecksum(path); got != "" {
		t.Fatalf("repository without an object format: %q", got)
	}

	sha256 := &Repo{EmptyTree: strings.Repeat("0", 64)}
	data := append([]byte("DIRC\x00\x00\x00\x02\x00\x00\x00\x00"), bytes.Repeat([]byte{0x5c}, 32)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := sha256.IndexChecksum(path); got != strings.Repeat("5c", 32) {
		t.Fatalf("sha256 checksum: %q", got)
	}
}

func TestWriteTreeRemembersByChecksum(t *testing.T) {
	r, calls := countingGit(t, treeA+"\n")
	sha1Repo(r)
	path := filepath.Join(t.TempDir(), "index")
	writeIndex(t, path, "one", 1)
	for range 3 {
		tree, err := r.WriteTree(path)
		if err != nil || tree != treeA {
			t.Fatalf("WriteTree = %q, %v", tree, err)
		}
	}
	if got := calls(); len(got) != 1 {
		t.Fatalf("git calls %v, want one write-tree for unchanged bytes", got)
	}
	writeIndex(t, path, "two", 2)
	if _, err := r.WriteTree(path); err != nil {
		t.Fatal(err)
	}
	if got := calls(); len(got) != 2 {
		t.Fatalf("git calls %v, want a second write-tree for changed bytes", got)
	}
	writeIndex(t, path, "one", 1)
	if _, err := r.WriteTree(path); err != nil {
		t.Fatal(err)
	}
	if got := calls(); len(got) != 2 {
		t.Fatalf("git calls %v, want the first bytes remembered", got)
	}
}

func TestWriteTreeRemembersNothingWithoutChecksum(t *testing.T) {
	r, calls := countingGit(t, treeA+"\n")
	sha1Repo(r)
	path := filepath.Join(t.TempDir(), "index")
	writeIndex(t, path, "one", 0)
	for range 2 {
		if _, err := r.WriteTree(path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.WriteTree(path + ".missing"); err != nil {
		t.Fatal(err)
	}
	if got := calls(); len(got) != 3 {
		t.Fatalf("git calls %v, want one for each call", got)
	}
	if len(r.indexTrees.trees) != 0 {
		t.Fatalf("remembered %v", r.indexTrees.trees)
	}
}

func TestWriteTreeDoesNotRememberAFailure(t *testing.T) {
	r := sha1Repo(fakeGit(t, "exit 1\n"))
	path := filepath.Join(t.TempDir(), "index")
	writeIndex(t, path, "one", 1)
	for range 2 {
		if _, err := r.WriteTree(path); err == nil {
			t.Fatal("WriteTree of a failing git succeeded")
		}
	}
	if len(r.indexTrees.trees) != 0 {
		t.Fatalf("remembered %v", r.indexTrees.trees)
	}
}

// TestWriteTreeRemembersTheRewrittenIndex checks the case of a write-tree
// that rewrites the index, as it does when it updates the cache-tree
// extension: both the bytes before and the bytes after name the tree.
func TestWriteTreeRemembersTheRewrittenIndex(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	after := filepath.Join(dir, "after")
	writeIndex(t, after, "with cache-tree", 2)
	r := sha1Repo(fakeGit(t, `echo "$*" >> '`+log+`'
cp '`+after+`' "$GIT_INDEX_FILE"
echo `+treeA+`
`))
	path := filepath.Join(dir, "index")
	writeIndex(t, path, "plain", 1)
	if tree, err := r.WriteTree(path); err != nil || tree != treeA {
		t.Fatalf("WriteTree = %q, %v", tree, err)
	}
	if tree := r.KnownIndexTree(path); tree != treeA {
		t.Fatalf("KnownIndexTree of the rewritten index = %q", tree)
	}
	writeIndex(t, path, "plain", 1)
	if tree, err := r.WriteTree(path); err != nil || tree != treeA {
		t.Fatalf("WriteTree of the bytes from before = %q, %v", tree, err)
	}
	data, _ := os.ReadFile(log)
	if n := strings.Count(string(data), "write-tree"); n != 1 {
		t.Fatalf("git ran write-tree %d times, want 1", n)
	}
}

func TestRememberAndKnownIndexTree(t *testing.T) {
	r, calls := countingGit(t, treeB+"\n")
	sha1Repo(r)
	path := filepath.Join(t.TempDir(), "index")
	writeIndex(t, path, "one", 1)
	if tree := r.KnownIndexTree(path); tree != "" {
		t.Fatalf("KnownIndexTree before = %q", tree)
	}
	r.RememberIndexTree(path, treeA)
	if tree := r.KnownIndexTree(path); tree != treeA {
		t.Fatalf("KnownIndexTree = %q", tree)
	}
	if tree, err := r.WriteTree(path); err != nil || tree != treeA {
		t.Fatalf("WriteTree = %q, %v", tree, err)
	}
	r.RememberIndexTree(path, "short")
	if tree := r.KnownIndexTree(path); tree != treeA {
		t.Fatalf("a tree of the wrong length replaced the entry: %q", tree)
	}
	writeIndex(t, path, "two", 2)
	if tree := r.KnownIndexTree(path); tree != "" {
		t.Fatalf("KnownIndexTree of other bytes = %q", tree)
	}
	if got := calls(); len(got) != 0 {
		t.Fatalf("git calls %v, want none", got)
	}
}

func TestIndexTreesFileServesANewProcess(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "multistage", "index-trees")
	path := filepath.Join(dir, "index")
	sum := writeIndex(t, path, "one", 1)

	first, _ := countingGit(t, treeA+"\n")
	sha1Repo(first).TreeCache = cache
	if _, err := first.WriteTree(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cache)
	if err != nil || string(data) != sum+" "+treeA+"\n" {
		t.Fatalf("cache file %q, %v", data, err)
	}

	second, calls := countingGit(t, treeB+"\n")
	sha1Repo(second).TreeCache = cache
	if tree, err := second.WriteTree(path); err != nil || tree != treeA {
		t.Fatalf("WriteTree of a new Repo = %q, %v", tree, err)
	}
	if got := calls(); len(got) != 0 {
		t.Fatalf("git calls %v, want none", got)
	}
	if entries, _ := os.ReadDir(filepath.Dir(cache)); len(entries) != 1 {
		t.Fatalf("directory holds %d files, want only the cache", len(entries))
	}
}

func TestIndexTreesFileDamaged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index")
	sum := writeIndex(t, path, "one", 0xab)
	good := strings.Repeat("22", 20) + " " + treeB
	for name, content := range map[string]string{
		"garbage":        "\x00\xff not a cache\n",
		"truncated line": sum + " " + treeA[:10],
		"one field":      sum + "\n",
		"upper case":     strings.ToUpper(sum) + " " + treeA + "\n",
		"extra field":    sum + " " + treeA + " x\n",
		"wrong length":   sum[:30] + " " + treeA + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			cache := filepath.Join(t.TempDir(), "index-trees")
			if err := os.WriteFile(cache, []byte(content+"\n"+good+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			r, calls := countingGit(t, treeA+"\n")
			sha1Repo(r).TreeCache = cache
			if tree, err := r.WriteTree(path); err != nil || tree != treeA {
				t.Fatalf("WriteTree = %q, %v", tree, err)
			}
			if got := calls(); len(got) != 1 {
				t.Fatalf("git calls %v, want one write-tree", got)
			}
			if tree := r.knownTree(strings.Repeat("22", 20)); tree != treeB {
				t.Fatalf("the good line was dropped: %q", tree)
			}
		})
	}
}

func TestIndexTreesFileUnreadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index")
	writeIndex(t, path, "one", 1)
	cache := filepath.Join(dir, "a-directory")
	if err := os.Mkdir(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	r, calls := countingGit(t, treeA+"\n")
	sha1Repo(r).TreeCache = cache
	for range 2 {
		if tree, err := r.WriteTree(path); err != nil || tree != treeA {
			t.Fatalf("WriteTree = %q, %v", tree, err)
		}
	}
	if got := calls(); len(got) != 1 {
		t.Fatalf("git calls %v, want one write-tree", got)
	}
}

func TestIndexTreesFileKeepsTheNewest64(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "index-trees")
	r := sha1Repo(&Repo{TreeCache: cache})
	sum := func(i int) string {
		return strings.Repeat(string("0123456789abcdef"[i>>4])+string("0123456789abcdef"[i&15]), 20)
	}
	for i := 1; i <= 70; i++ {
		r.rememberTree(sum(i), treeA)
	}
	r.rememberTree(sum(10), treeB) // an entry that changes moves to the end
	data, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != maxIndexTrees {
		t.Fatalf("%d lines, want %d", len(lines), maxIndexTrees)
	}
	if lines[0] != sum(7)+" "+treeA || lines[len(lines)-1] != sum(10)+" "+treeB {
		t.Fatalf("first line %q, last line %q", lines[0], lines[len(lines)-1])
	}
	if r.knownTree(sum(6)) != "" || r.knownTree(sum(7)) != treeA {
		t.Fatal("the oldest entries were not dropped")
	}
	fresh := sha1Repo(&Repo{TreeCache: cache})
	if fresh.knownTree(sum(70)) != treeA || fresh.knownTree(sum(6)) != "" {
		t.Fatal("a new process did not read the 64 entries")
	}
}
