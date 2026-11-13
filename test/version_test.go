package test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestVersionStamp builds the command the way a release does and checks that
// `version` reports the stamped value, and that a plain build reports dev.
func TestVersionStamp(t *testing.T) {
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	build := func(name string, args ...string) string {
		out := filepath.Join(t.TempDir(), name+exe)
		cmd := exec.CommandContext(context.Background(), "go", append(append([]string{"build"}, args...), "-o", out, "../cmd/git-multistage")...)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, b)
		}
		return out
	}
	version := func(path string) string {
		b, err := exec.CommandContext(context.Background(), path, "version").Output()
		if err != nil {
			t.Fatalf("version: %v", err)
		}
		return strings.TrimSpace(string(b))
	}
	stamped := build("stamped", "-ldflags", "-X github.com/chrisfls/git-multistage/internal/ms.Version=1.2.3")
	if got := version(stamped); got != "git-multistage 1.2.3" {
		t.Fatalf("stamped build reports %q", got)
	}
	if got := version(build("plain")); got != "git-multistage dev" {
		t.Fatalf("plain build reports %q", got)
	}
}
