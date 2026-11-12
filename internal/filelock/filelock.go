// Package filelock holds exclusive operating-system locks on files.
// The operating system releases a lock when its process exits, so a crashed
// process never leaves a lock behind.
package filelock

import (
	"errors"
	"os"
	"time"
)

// ErrBusy means that another process holds the lock.
var ErrBusy = errors.New("lock is held by another process")

// Lock is a held lock.
type Lock struct{ f *os.File }

// Acquire locks path, creating it when needed, and waits up to timeout.
func Acquire(path string, timeout time.Duration) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	delay := 5 * time.Millisecond
	for {
		err := tryLock(f)
		if err == nil {
			return &Lock{f: f}, nil
		}
		if !errors.Is(err, ErrBusy) {
			_ = f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, ErrBusy
		}
		time.Sleep(delay)
		if delay < 100*time.Millisecond {
			delay *= 2
		}
	}
}

// Release unlocks and closes the lock file.
func (l *Lock) Release() {
	if l == nil || l.f == nil {
		return
	}
	unlock(l.f)
	_ = l.f.Close()
	l.f = nil
}
