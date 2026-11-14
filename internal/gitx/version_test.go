//go:build !windows

package gitx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// buildFakeGit compiles a program at dir/git that appends a line to log and
// prints "git version <version>". A compiled program, unlike a script, is a
// Git binary that the version check may remember.
func buildFakeGit(t *testing.T, dir, log, version string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "main.go")
	prog := "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nvar log, version string\n\n" +
		"func main() {\n\tf, _ := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)\n" +
		"\tfmt.Fprintln(f, \"x\")\n\t_ = f.Close()\n\tfmt.Println(\"git version \" + version)\n}\n"
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "git")
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", bin,
		"-ldflags", "-X main.log="+log+" -X main.version="+version, src)
	cmd.Env = append(os.Environ(), "GOFLAGS=", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// TestVersionCheckRemembersTheBinary checks that a passed version check is
// remembered for the binary's path, size, and modification time, and that
// any change of them, a damaged cache file, and an empty cache name run
// `git version` again.
func TestVersionCheckRemembersTheBinary(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	dir := t.TempDir()
	bin := buildFakeGit(t, dir, log, "2.54.0")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := &Repo{Top: t.TempDir(), baseEnv: internalEnv()}
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
	buildFakeGit(t, dir, log, "2.54.1000000")
	check(cache, 4, "new size")
	if err := os.WriteFile(cache, []byte("damaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	check(cache, 5, "damaged cache file")
	other := t.TempDir()
	buildFakeGit(t, other, log, "2.54.0")
	t.Setenv("PATH", other+string(os.PathListSeparator)+os.Getenv("PATH"))
	check(cache, 6, "another path")
	check(cache, 6, "the other path again")
	data, err := os.ReadFile(cache)
	if err != nil || !strings.HasPrefix(string(data), fmt.Sprintf("%d.%d ", MinVersion[0], MinVersion[1])) {
		t.Fatalf("cache file %q, %v; want the minimum version first", data, err)
	}
}

// TestVersionCheckRemembersNoShim checks that a git that runs another
// Git is never remembered: a script, and a binary of another name, such as a
// version manager's shim behind a symbolic link.
func TestVersionCheckRemembersNoShim(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	cache := filepath.Join(t.TempDir(), "git-version")
	calls := func() int {
		data, _ := os.ReadFile(log)
		return strings.Count(string(data), "x")
	}
	r := fakeGit(t, "echo x >>"+log+"\necho 'git version 2.54.0'\n")
	for range 2 {
		if err := r.runVersionCheck(cache); err != nil {
			t.Fatal(err)
		}
	}
	if n := calls(); n != 2 {
		t.Fatalf("a script git ran git version %d times in two checks, want 2", n)
	}

	shimDir, pathDir := t.TempDir(), t.TempDir()
	shim := buildFakeGit(t, shimDir, log, "2.54.0")
	if err := os.Rename(shim, filepath.Join(shimDir, "shim")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(shimDir, "shim"), filepath.Join(pathDir, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pathDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for range 2 {
		if err := r.runVersionCheck(cache); err != nil {
			t.Fatal(err)
		}
	}
	if n := calls(); n != 4 {
		t.Fatalf("a shim of another name ran git version %d times in two checks, want 2", n-2)
	}
	if _, err := os.Stat(cache); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a shim was remembered: %v", err)
	}
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat(xcodeShim); err == nil {
			t.Setenv("PATH", filepath.Dir(xcodeShim))
			if key := r.binaryKey(); key != "" {
				t.Fatalf("the Xcode shim has the key %q, want none", key)
			}
		}
	}
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
