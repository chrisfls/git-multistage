//go:build linux

package ms

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

// renameat2Calls are the numbers of the renameat2 system call, which the
// syscall package does not name on every architecture.
var renameat2Calls = map[string]uintptr{
	"386": 353, "amd64": 316, "arm": 382, "arm64": 276, "loong64": 276, "mips": 4351, "mipsle": 4351,
	"mips64": 5311, "mips64le": 5311, "ppc64": 357, "ppc64le": 357, "riscv64": 276, "s390x": 347,
}

const (
	atFDCWD         = -100 // AT_FDCWD
	renameNoreplace = 1    // RENAME_NOREPLACE
)

// renameNoReplace renames oldpath to newpath, and fails with an error that
// matches fs.ErrExist when newpath exists. It uses renameat2 with
// RENAME_NOREPLACE, and link then unlink where that is not supported.
func renameNoReplace(oldpath, newpath string) error {
	nr, ok := renameat2Calls[runtime.GOARCH]
	if !ok {
		return linkRename(oldpath, newpath)
	}
	err := renameat2(nr, oldpath, newpath)
	if errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
		return linkRename(oldpath, newpath)
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	return nil
}

func renameat2(nr uintptr, oldpath, newpath string) error {
	from, err := syscall.BytePtrFromString(oldpath)
	if err != nil {
		return err
	}
	to, err := syscall.BytePtrFromString(newpath)
	if err != nil {
		return err
	}
	cwd := atFDCWD
	//nolint:gosec // G103: the system call reads the two path strings, which stay alive during the call
	_, _, errno := syscall.Syscall6(nr, uintptr(cwd), uintptr(unsafe.Pointer(from)), uintptr(cwd), uintptr(unsafe.Pointer(to)), renameNoreplace, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// caseRenameInOneCall reports whether renameNoReplace renames between two
// spellings of one path on a case-insensitive file system.
const caseRenameInOneCall = false
