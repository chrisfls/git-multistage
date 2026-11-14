//go:build darwin

package ms

import (
	"os"
	"syscall"
)

// syncFile flushes f with fsync(2), as Git's default core.fsyncMethod,
// writeout-only, does on macOS. os.File.Sync uses F_FULLFSYNC, which also
// flushes the drive's cache and takes about 5 ms.
func syncFile(f *os.File) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var syncErr error
	if err := rc.Control(func(fd uintptr) { syncErr = syscall.Fsync(int(fd)) }); err != nil {
		return err
	}
	return syncErr
}
