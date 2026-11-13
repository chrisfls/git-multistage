//go:build !windows

package gitx

import (
	"bytes"
	"os"
	"path/filepath"
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
