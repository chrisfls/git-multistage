// Package filelock holds exclusive operating-system locks on files.
// The operating system releases a lock when its process exits, so a crashed
// process never leaves a lock behind.
package filelock

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// ErrBusy means that another process holds the lock.
var ErrBusy = errors.New("lock is held by another process")

// Lock is a held lock.
type Lock struct{ f *os.File }

// Acquire locks path, creating it when needed, and waits up to timeout.
// A lock file that another process renames or deletes while this process
// waits is not the lock of path any more, so Acquire then locks the file at
// path instead. When the directory of path is gone, it returns an error that
// wraps fs.ErrNotExist.
func Acquire(path string, timeout time.Duration) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	delay := time.Millisecond
	for {
		err := tryLock(f)
		if err == nil {
			same, err := isFileAt(f, path)
			if err != nil {
				unlock(f)
				_ = f.Close()
				return nil, err
			}
			if same {
				return &Lock{f: f}, nil
			}
			unlock(f)
			_ = f.Close()
			if f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o666); err != nil {
				return nil, err
			}
			continue
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
		delay = min(2*delay, maxPoll)
	}
}

// maxPoll is the longest wait between two checks of a held lock. A check is
// one system call, and a longer wait keeps a waiter asleep after the lock is
// free.
const maxPoll = 10 * time.Millisecond

// isFileAt reports whether the open file f is still the file at path.
func isFileAt(f *os.File, path string) (bool, error) {
	held, err := f.Stat()
	if err != nil {
		return false, err
	}
	cur, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(held, cur), nil
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
