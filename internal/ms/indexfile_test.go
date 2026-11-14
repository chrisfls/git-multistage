package ms

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// TestReadIndexFileMatchesLsFiles compares the index file reader with
// ls-files for pathspecs that name a file, a directory, a missing path, a
// conflicted path, and a path below a file, and checks the paths that only
// ls-files reads.
func TestReadIndexFileMatchesLsFiles(t *testing.T) {
	t.Parallel()
	r := batchRepo(t)
	blob, err := r.Line(gitx.Opt{Stdin: strings.NewReader("x\n")}, "hash-object", "-w", "--stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(gitx.Opt{Stdin: strings.NewReader(fmt.Sprintf("100644 %s 1\tc\n100644 %s 3\tc\n100644 %s 0\td/e/f\n", blob, blob, blob))},
		"update-index", "--index-info"); err != nil {
		t.Fatal(err)
	}
	paths := []string{"a", "d", "c", "nope", "a/b", "d/e"}
	cur, conf, ok := r.readIndexFile("", paths)
	if !ok {
		t.Fatal("readIndexFile handed the index to Git")
	}
	wantCur, wantConf, err := r.lsFilesEntries("", paths)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(cur) != fmt.Sprint(wantCur) || fmt.Sprint(conf) != fmt.Sprint(wantConf) {
		t.Fatalf("readIndexFile = %v, %v; ls-files reads %v, %v", cur, conf, wantCur, wantConf)
	}
	if len(cur) != 3 || !conf["c"] {
		t.Fatalf("readIndexFile = %v, %v; want a, d/b, d/e/f and the conflicted c", cur, conf)
	}
	for _, p := range []string{"", "d/", "./a", "d//b", "d/../a", `d\b`} {
		if _, _, ok := r.readIndexFile("", []string{"a", p}); ok {
			t.Errorf("readIndexFile read the pathspec %q, which only ls-files reads", p)
		}
	}
}

// TestICaseSpellingMatchesLsFiles compares the index spelling of a path
// with the name that the callers choose from `ls-files` with
// GIT_ICASE_PATHSPECS: the hook's pathspec is the path, and add's is the
// path's directory. The pathspec ignores ASCII case only.
func TestICaseSpellingMatchesLsFiles(t *testing.T) {
	t.Parallel()
	r := batchRepo(t)
	for _, f := range []string{"Readme.TXT", "Ä.txt", "Ä/x", "Sub/Deep/File.go"} {
		cmd := exec.CommandContext(context.Background(), "git", "update-index", "--add", "--cacheinfo", "100644,"+r.EmptyBlob+","+f)
		cmd.Dir = r.Top
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git update-index: %v\n%s", err, out)
		}
	}
	lsFiles := func(spec, p string) string {
		t.Helper()
		out, err := r.Run(gitx.Opt{Env: []string{envLiteralPathspecs, "GIT_ICASE_PATHSPECS=1"}}, "ls-files", "-z", "--", spec)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range strings.Split(string(out), "\x00") {
			if strings.EqualFold(name, p) {
				return name
			}
		}
		return p
	}
	for _, p := range []string{"README.txt", "readme.txt", "D/B", "ä.txt", "Ä.txt", "ä/x", "sub/deep/file.GO", "missing", "sub/missing"} {
		for _, spec := range []string{p, path.Dir(p)} {
			name, found, ok := r.icaseSpelling(r.IndexPath, spec, p)
			if !ok {
				t.Fatalf("icaseSpelling(%q, %q) handed the index to Git", spec, p)
			}
			if !found {
				name = p
			}
			if want := lsFiles(spec, p); name != want {
				t.Errorf("icaseSpelling(%q, %q) = %q; ls-files gives %q", spec, p, name, want)
			}
		}
		if got, want := r.indexSpelling(p), lsFiles(p, p); got != want {
			t.Errorf("indexSpelling(%q) = %q; ls-files gives %q", p, got, want)
		}
	}
}

// TestICaseSpellingLeavesGlobsToGit hands a pathspec with glob characters or
// magic to ls-files.
func TestICaseSpellingLeavesGlobsToGit(t *testing.T) {
	t.Parallel()
	r := batchRepo(t)
	for _, spec := range []string{"", "a*", "a?", "a[b]", `a\b`, ":(top)a"} {
		if _, _, ok := r.icaseSpelling(r.IndexPath, spec, "a"); ok {
			t.Errorf("icaseSpelling read the pathspec %q, which only ls-files reads", spec)
		}
	}
}
