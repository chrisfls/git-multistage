package filelock

import (
	"errors"
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
