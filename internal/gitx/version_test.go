package gitx

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestVersionCheckRemembersTheBinary checks that a passed version check is
// remembered for the binary's path, size, and modification time, and that
// any change of them, a damaged cache file, and an empty cache name run
// `git version` again.
func TestVersionCheckRemembersTheBinary(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	script := "echo x >>" + log + "\necho 'git version 2.54.0'\n"
	r := fakeGit(t, script)
	bin, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(t.TempDir(), "ms", "git-version")
	check := func(cache string, want int, why string) {
		t.Helper()
		if err := r.runVersionCheck(cache); err != nil {
			t.Fatalf("%s: %v", why, err)
		}
		data, _ := os.ReadFile(log)
		if got := strings.Count(string(data), "x"); got != want {
			t.Fatalf("%s: git version ran %d times in all, want %d", why, got, want)
		}
	}
	check(cache, 1, "first check")
	check(cache, 1, "same binary")
	check("", 2, "no cache name")
	if err := os.Chtimes(bin, time.Now(), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	check(cache, 3, "new modification time")
	check(cache, 3, "same binary again")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n# a new build\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	check(cache, 4, "new size")
	if err := os.WriteFile(cache, []byte("damaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	check(cache, 5, "damaged cache file")
	r = fakeGit(t, script)
	check(cache, 6, "another path")
	check(cache, 6, "the other path again")
}

// TestVersionCheckRefusesAnOldGit checks that an old Git fails the check and
// is not remembered.
func TestVersionCheckRefusesAnOldGit(t *testing.T) {
	r := fakeGit(t, "echo 'git version 2.44.1'\n")
	cache := filepath.Join(t.TempDir(), "git-version")
	if err := r.runVersionCheck(cache); err == nil || !strings.Contains(err.Error(), "needs Git 2.45") {
		t.Fatalf("runVersionCheck = %v, want the version error", err)
	}
	if _, err := os.Stat(cache); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an old Git was remembered: %v", err)
	}
}

// resetVersionCheck forgets the version check of this process. The tests
// that call it do not run in parallel, so no other Open runs meanwhile.
func resetVersionCheck(t *testing.T) {
	t.Helper()
	reset := func() {
		versionChecked.once = sync.Once{}
		versionChecked.err = nil
	}
	reset()
	t.Cleanup(reset)
}

// TestOpenChecksTheVersion checks that Open writes the cache file under the
// cache directory name, writes none without one, and reports an old Git that
// fails rev-parse by its version.
func TestOpenChecksTheVersion(t *testing.T) {
	dir := initRepo(t, "sha1")
	resetVersionCheck(t)
	if _, err := Open(dir, ""); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, ".git", "ms", "git-version")
	if _, err := os.Stat(cache); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open without a cache name wrote %s: %v", cache, err)
	}
	resetVersionCheck(t)
	if _, err := Open(dir, "ms"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(cache); err != nil || !strings.HasSuffix(strings.TrimSpace(string(data)), "git") {
		t.Fatalf("cache file %q, %v; want the binary's key", data, err)
	}

	resetVersionCheck(t)
	fakeGit(t, `case "$*" in *version*) echo 'git version 2.30.0' ;; *) echo 'error: unknown option' >&2; exit 129 ;; esac`+"\n")
	if _, err := Open(dir, "ms"); err == nil || !strings.Contains(err.Error(), "needs Git 2.45") {
		t.Fatalf("Open with an old Git = %v, want the version error", err)
	}
}
