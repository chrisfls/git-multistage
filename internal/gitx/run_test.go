//go:build !windows

package gitx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeGit puts a shell script named git first on PATH and returns a Repo
// whose calls run it.
func fakeGit(t *testing.T, script string) *Repo {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &Repo{Top: t.TempDir(), baseEnv: internalEnv()}
}

// TestRunUntranslated checks that a call whose output git-multistage reads
// runs with LC_ALL=C, and that calls for the user keep the user's locale.
func TestRunUntranslated(t *testing.T) {
	t.Setenv("LC_ALL", "de_DE.UTF-8")
	r := fakeGit(t, `printf %s "$LC_ALL"`)
	out, err := r.Run(Opt{}, "status")
	if err != nil || string(out) != "C" {
		t.Fatalf("captured call: LC_ALL=%q, %v", out, err)
	}
	out, err = r.Run(Opt{User: true}, "status")
	if err != nil || string(out) != "de_DE.UTF-8" {
		t.Fatalf("user call: LC_ALL=%q, %v", out, err)
	}
	var streamed bytes.Buffer
	if _, err := r.Run(Opt{Stdout: &streamed}, "status"); err != nil || streamed.String() != "de_DE.UTF-8" {
		t.Fatalf("streamed call: LC_ALL=%q, %v", streamed.String(), err)
	}
}

// lostWriteGit is a fake git that counts its runs in $COUNT, reads all of
// its stdin, and fails with a lost loose-object write until run $OK_AT.
// On success it prints the stdin of that run.
const lostWriteGit = `n=$(($(cat "$COUNT" 2>/dev/null || echo 0) + 1))
echo "$n" > "$COUNT"
in=$(cat)
if [ "$n" -lt "$OK_AT" ]; then
	echo "error: unable to write file .git/objects/e6/9de29bb2d1d6434b8b29ae775ad8c2e48c5391: Permission denied" >&2
	exit 128
fi
printf %s "$in"
`

// lostWriteRepo returns a Repo whose git is lostWriteGit with renames on,
// and a function that reads the number of runs.
func lostWriteRepo(t *testing.T, okAt int) (*Repo, func() int) {
	t.Helper()
	count := filepath.Join(t.TempDir(), "count")
	t.Setenv("COUNT", count)
	t.Setenv("OK_AT", strconv.Itoa(okAt))
	saved := renamesObjects
	renamesObjects = true
	t.Cleanup(func() { renamesObjects = saved })
	return fakeGit(t, lostWriteGit), func() int {
		data, err := os.ReadFile(count)
		if err != nil {
			t.Fatal(err)
		}
		n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		return n
	}
}

// TestRunRerunsALostObjectWrite reruns a command that lost a loose-object
// write, and gives the rerun the same stdin from where the first run began.
func TestRunRerunsALostObjectWrite(t *testing.T) {
	r, runs := lostWriteRepo(t, 3)
	in := strings.NewReader("skip:payload")
	if _, err := in.Seek(5, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	out, err := r.Run(Opt{Stdin: in}, "hash-object", "-w", "--stdin")
	if err != nil || string(out) != "payload" {
		t.Fatalf("Run = %q, %v; want the rewound stdin", out, err)
	}
	if n := runs(); n != 3 {
		t.Fatalf("%d runs, want 3", n)
	}
}

// TestRunStopsRerunningAfterTheCap gives up after objectWriteRetries reruns.
func TestRunStopsRerunningAfterTheCap(t *testing.T) {
	r, runs := lostWriteRepo(t, 1000)
	_, err := r.Run(Opt{}, "hash-object", "-w", "--stdin")
	if !lostObjectWrite(errorStderr(err)) {
		t.Fatalf("Run = %v; want the lost write", err)
	}
	if n := runs(); n != 1+objectWriteRetries {
		t.Fatalf("%d runs, want %d", n, 1+objectWriteRetries)
	}
}

// TestRunDoesNotRerun runs once a call that cannot be repeated safely: a
// user command, streamed output, and stdin that cannot be rewound.
func TestRunDoesNotRerun(t *testing.T) {
	for _, tc := range []struct {
		name string
		opt  func() Opt
	}{
		{"user", func() Opt { return Opt{User: true} }},
		{"streamed stdout", func() Opt { return Opt{Stdout: io.Discard} }},
		{"streamed stderr", func() Opt { return Opt{Stderr: io.Discard} }},
		{"stdin that cannot be rewound", func() Opt { return Opt{Stdin: io.MultiReader(strings.NewReader("x"))} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, runs := lostWriteRepo(t, 2)
			if _, err := r.Run(tc.opt(), "hash-object", "-w", "--stdin"); !lostObjectWrite(errorStderr(err)) {
				t.Fatalf("Run = %v; want the lost write", err)
			}
			if n := runs(); n != 1 {
				t.Fatalf("%d runs, want 1", n)
			}
		})
	}
}

// errorStderr returns the stderr of a Git error, or "".
func errorStderr(err error) string {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Stderr
	}
	return ""
}

// TestRunPicksTheBinary runs internal calls with the Repo's own Git, and
// user calls with the git on PATH.
func TestRunPicksTheBinary(t *testing.T) {
	r := fakeGit(t, `printf path`)
	r.git = filepath.Join(t.TempDir(), "own-git")
	if err := os.WriteFile(r.git, []byte("#!/bin/sh\nprintf own"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := r.Run(Opt{}, "status"); err != nil || string(out) != "own" {
		t.Fatalf("internal call ran %q, %v; want the own Git", out, err)
	}
	if out, err := r.Run(Opt{User: true}, "status"); err != nil || string(out) != "path" {
		t.Fatalf("user call ran %q, %v; want the git on PATH", out, err)
	}
}

// TestOwnGit replaces the macOS Xcode shim with the Git that it runs, and
// keeps every other git on PATH.
func TestOwnGit(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if got := ownGit(); got != "" {
			t.Fatalf("ownGit() = %q on %s, want the git on PATH", got, runtime.GOOS)
		}
		return
	}
	shimVersion, err := exec.CommandContext(context.Background(), xcodeShim, "version").Output()
	if err != nil {
		t.Skipf("the Xcode shim does not run: %v", err)
	}
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("GIT_EXEC_PATH", "")
	own := ownGit()
	if own == "" || own == xcodeShim {
		t.Fatalf("ownGit() = %q, want the Git that the shim runs", own)
	}
	ownVersion, err := exec.CommandContext(context.Background(), own, "version").Output()
	if err != nil || string(ownVersion) != string(shimVersion) {
		t.Fatalf("%s version = %q, %v; the shim reports %q", own, ownVersion, err, shimVersion)
	}
	r, err := Open(initRepo(t, "sha1"))
	if err != nil || r.git != own {
		t.Fatalf("Open runs %q, %v; want %q", r.git, err, own)
	}

	t.Setenv("GIT_EXEC_PATH", filepath.Dir(own))
	if got := ownGit(); got != "" {
		t.Fatalf("with GIT_EXEC_PATH, ownGit() = %q, want the git on PATH", got)
	}
	t.Setenv("GIT_EXEC_PATH", "")
	fakeGit(t, "")
	if got := ownGit(); got != "" {
		t.Fatalf("with another git on PATH, ownGit() = %q, want the git on PATH", got)
	}
}
