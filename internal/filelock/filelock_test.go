package filelock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExclusive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock")
	a, err := Acquire(p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(p, 50*time.Millisecond); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Acquire = %v, want ErrBusy", err)
	}
	a.Release()
	b, err := Acquire(p, time.Second)
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	b.Release()
}

// TestLockFollowsThePath has a waiter whose lock file is renamed away while
// it waits, as dropping a stage renames the stage directory. The waiter must
// end up holding the file at the lock path, not the renamed file, so that a
// later process that locks the path waits for it.
func TestLockFollowsThePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "lock")
	a, err := Acquire(p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	var b *Lock
	go func() {
		var err error
		b, err = Acquire(p, 5*time.Second)
		got <- err
	}()
	time.Sleep(100 * time.Millisecond)
	if err := os.Rename(p, filepath.Join(dir, "renamed")); err != nil {
		t.Fatal(err)
	}
	a.Release()
	if err := <-got; err != nil {
		t.Fatalf("waiter: %v", err)
	}
	defer b.Release()
	if _, err := Acquire(p, 50*time.Millisecond); !errors.Is(err, ErrBusy) {
		t.Fatalf("a third Acquire = %v, want ErrBusy", err)
	}
}
