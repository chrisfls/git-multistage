//go:build !testhooks

package ms

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestReleaseBuildIgnoresLeaseTimeoutVariable checks that a build without
// the testhooks tag waits for a held lease although
// GIT_MULTISTAGE_LEASE_TIMEOUT_MS asks for no wait.
func TestReleaseBuildIgnoresLeaseTimeoutVariable(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.CommandContext(context.Background(), "git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	newHook := func(stage string) *editHook {
		t.Helper()
		h, err := newEditHook(hookInput{}, stage, filepath.Join(dir, "f.txt"))
		if err != nil || h == nil {
			t.Fatalf("newEditHook(%s) = %v, %v", stage, h, err)
		}
		return h
	}
	holder, waiter := newHook("b"), newHook("a")
	held, _, _, err := holder.takeLease()
	if err != nil || held == "" {
		t.Fatalf("stage b takes the lease: %q, %v", held, err)
	}
	t.Setenv("GIT_MULTISTAGE_LEASE_TIMEOUT_MS", "0")
	type result struct {
		nonce    string
		tookOver bool
		err      error
	}
	done := make(chan result, 1)
	go func() {
		nonce, _, tookOver, err := waiter.takeLease()
		done <- result{nonce, tookOver, err}
	}()
	select {
	case res := <-done:
		t.Fatalf("takeLease returned while the lease was held: %+v", res)
	case <-time.After(500 * time.Millisecond):
	}
	holder.releaseLease(held)
	res := <-done
	if res.err != nil || res.nonce == "" || res.tookOver {
		t.Fatalf("takeLease after the release = %+v", res)
	}
}
